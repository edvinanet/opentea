// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/oej/opentea/internal/idgen"
	"github.com/oej/opentea/pkg/tea"
)

// ArtifactFormatInput carries the fields needed to create one format of a new artifact.
type ArtifactFormatInput struct {
	MediaType   string
	Description string
}

// ArtifactInput carries the fields needed to create a new artifact.
type ArtifactInput struct {
	Name            string
	Type            string
	CreatedDate     *time.Time
	DistributionIDs []string
	Formats         []ArtifactFormatInput
}

// CreateArtifact creates a new artifact at version 1 (the version sequence
// for a given uuid is only advanced by future "create new revision" flows,
// which Phase 1 doesn't need).
func (r *Repo) CreateArtifact(ctx context.Context, in ArtifactInput) (tea.Artifact, error) {
	uuid := idgen.New()
	const version = 1

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return tea.Artifact{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO artifact (uuid, version, name, type, created_date) VALUES (?, ?, ?, ?, ?)`,
		uuid, version, in.Name, in.Type, formatTimePtr(in.CreatedDate),
	); err != nil {
		return tea.Artifact{}, err
	}

	for _, distID := range in.DistributionIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO artifact_distribution (artifact_uuid, artifact_version, distribution_id) VALUES (?, ?, ?)`,
			uuid, version, distID,
		); err != nil {
			return tea.Artifact{}, err
		}
	}

	for _, f := range in.Formats {
		formatID := idgen.New()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO artifact_format (id, artifact_uuid, artifact_version, media_type, description) VALUES (?, ?, ?, ?, ?)`,
			formatID, uuid, version, f.MediaType, f.Description,
		); err != nil {
			return tea.Artifact{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return tea.Artifact{}, err
	}
	return r.GetArtifactByVersion(ctx, uuid, version)
}

// ImportArtifactFormatInput mirrors ArtifactFormatInput but adds URL/
// checksums, since a bundle already has the file data (unlike the normal
// create-then-upload flow).
type ImportArtifactFormatInput struct {
	MediaType    string
	Description  string
	URL          string
	SignatureURL string
	Checksums    []tea.Checksum
}

// ImportArtifactInput mirrors ArtifactInput but carries the explicit
// (uuid, version) identity a bundle's artifacts are keyed by.
type ImportArtifactInput struct {
	UUID            string
	Version         int
	Name            string
	Type            string
	CreatedDate     *time.Time
	DistributionIDs []string
	Formats         []ImportArtifactFormatInput
}

// ImportArtifact creates the artifact revision at the explicit (uuid,
// version) if it doesn't already exist, or leaves an existing one untouched
// if its content matches -- see ImportProduct for the identity/idempotency
// rationale. Artifacts are versioned, so re-importing the same (uuid,
// version) is a no-op, but a bundle can still introduce a new version of an
// artifact the target already has some revisions of. Returns
// ErrImportIdentityConflict if (uuid, version) already exists with
// different content.
func (r *Repo) ImportArtifact(ctx context.Context, in ImportArtifactInput) (created bool, err error) {
	return runInTx(ctx, r, func(tx dbtx) (bool, error) {
		existing, err := getArtifactByVersionTx(ctx, tx, in.UUID, in.Version)
		if err == nil {
			if artifactConflicts(existing, in) {
				return false, fmt.Errorf("%w: artifact %s version %d", ErrImportIdentityConflict, in.UUID, in.Version)
			}
			return false, nil
		} else if !errors.Is(err, ErrNotFound) {
			return false, err
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO artifact (uuid, version, name, type, created_date) VALUES (?, ?, ?, ?, ?)`,
			in.UUID, in.Version, in.Name, in.Type, formatTimePtr(in.CreatedDate),
		); err != nil {
			return false, err
		}

		for _, distID := range in.DistributionIDs {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO artifact_distribution (artifact_uuid, artifact_version, distribution_id) VALUES (?, ?, ?)`,
				in.UUID, in.Version, distID,
			); err != nil {
				return false, err
			}
		}

		for _, f := range in.Formats {
			formatID := idgen.New()
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO artifact_format (id, artifact_uuid, artifact_version, media_type, description, url, signature_url) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				formatID, in.UUID, in.Version, f.MediaType, f.Description, nullIfEmpty(f.URL), nullIfEmpty(f.SignatureURL),
			); err != nil {
				return false, err
			}
			if err := insertChecksums(ctx, tx, OwnerArtifactFormat, formatID, f.Checksums); err != nil {
				return false, err
			}
		}

		return true, nil
	})
}

// artifactConflicts reports whether existing (already stored) differs from
// in (being imported) in a way that means they're not the same artifact
// revision. Formats' URL/SignatureURL are excluded from the comparison --
// same rewritten-URL reasoning as distribution.go's ImportDistribution.
func artifactConflicts(existing tea.Artifact, in ImportArtifactInput) bool {
	if existing.Name != in.Name {
		return true
	}
	if existing.Type != in.Type {
		return true
	}
	if !timePtrEqual(existing.CreatedDate, in.CreatedDate) {
		return true
	}
	if !setEqual(existing.DistributionIDs, in.DistributionIDs) {
		return true
	}
	existingKeys := make([]string, len(existing.Formats))
	for i, f := range existing.Formats {
		existingKeys[i] = artifactFormatKey(f.MediaType, f.Description, f.Checksums)
	}
	inKeys := make([]string, len(in.Formats))
	for i, f := range in.Formats {
		inKeys[i] = artifactFormatKey(f.MediaType, f.Description, f.Checksums)
	}
	return !setEqual(existingKeys, inKeys)
}

// artifactFormatKey builds a canonical, order-independent string identity
// for one artifact format's content -- formats have a randomly-generated
// surrogate id, not a content-derived one, so there's no stable "same
// format" identity across two different bundles' artifact definitions to
// compare positionally; this key lets setEqual compare them as a set
// instead.
func artifactFormatKey(mediaType, description string, checksums []tea.Checksum) string {
	keys := make([]string, len(checksums))
	for i, c := range checksums {
		keys[i] = c.AlgType + "=" + c.AlgValue
	}
	sort.Strings(keys)
	return mediaType + "\x00" + description + "\x00" + strings.Join(keys, "\x01")
}

// GetArtifactLatest fetches the highest-versioned revision of artifact
// uuid. Returns ErrNotFound if uuid has no revisions.
func (r *Repo) GetArtifactLatest(ctx context.Context, uuid string) (tea.Artifact, error) {
	version, ok, err := r.LatestArtifactVersion(ctx, uuid)
	if err != nil {
		return tea.Artifact{}, err
	}
	if !ok {
		return tea.Artifact{}, ErrNotFound
	}
	return r.GetArtifactByVersion(ctx, uuid, version)
}

// LatestArtifactVersion fetches just the highest existing version number
// for artifact uuid -- for ETag construction on the "latest artifact"
// endpoint, so a conditional GET can check for a newly-published revision
// via one indexed MAX() lookup instead of the full artifact fetch. ok is
// false (not an error) if uuid has no revisions at all.
func (r *Repo) LatestArtifactVersion(ctx context.Context, uuid string) (version int, ok bool, err error) {
	var v sql.NullInt64
	if err := r.conn().QueryRowContext(ctx, `SELECT MAX(version) FROM artifact WHERE uuid = ?`, uuid).Scan(&v); err != nil {
		return 0, false, err
	}
	if !v.Valid {
		return 0, false, nil
	}
	return int(v.Int64), true, nil
}

// GetArtifactByVersion fetches one specific revision of artifact uuid.
// Returns ErrNotFound if that (uuid, version) pair doesn't exist.
func (r *Repo) GetArtifactByVersion(ctx context.Context, uuid string, version int) (tea.Artifact, error) {
	return getArtifactByVersionTx(ctx, r.conn(), uuid, version)
}

// GetArtifactRevision fetches just the revision counter for one artifact
// version -- for ETag construction, so a conditional GET can check
// If-None-Match against a single indexed column instead of the full
// join-heavy fetch GetArtifactByVersion does. Returns ErrNotFound if that
// (uuid, version) pair doesn't exist.
func (r *Repo) GetArtifactRevision(ctx context.Context, uuid string, version int) (int64, error) {
	return getArtifactRevisionTx(ctx, r.conn(), uuid, version)
}

func getArtifactRevisionTx(ctx context.Context, q dbtx, uuid string, version int) (int64, error) {
	var revision int64
	err := q.QueryRowContext(ctx, `SELECT revision FROM artifact WHERE uuid = ? AND version = ?`, uuid, version).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return revision, nil
}

// GetArtifactType fetches just the type column for (uuid, version) -- for
// authz checks that need to evaluate an artifact-type-constrained
// capability rule before the full object is worth fetching (see
// internal/api/artifact.go). Returns ErrNotFound if the (uuid, version)
// pair doesn't exist.
func (r *Repo) GetArtifactType(ctx context.Context, uuid string, version int) (string, error) {
	var artifactType string
	err := r.conn().QueryRowContext(ctx, `SELECT type FROM artifact WHERE uuid = ? AND version = ?`, uuid, version).Scan(&artifactType)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return artifactType, err
}

// getArtifactByVersionTx is GetArtifactByVersion's logic parameterized
// over a dbtx -- see product.go's getProductTx doc comment for why this
// exists.
func getArtifactByVersionTx(ctx context.Context, q dbtx, uuid string, version int) (tea.Artifact, error) {
	var name sql.NullString
	var artifactType string
	var createdDate sql.NullString
	err := q.QueryRowContext(ctx,
		`SELECT name, type, created_date FROM artifact WHERE uuid = ? AND version = ?`, uuid, version,
	).Scan(&name, &artifactType, &createdDate)
	if errors.Is(err, sql.ErrNoRows) {
		return tea.Artifact{}, ErrNotFound
	}
	if err != nil {
		return tea.Artifact{}, err
	}

	created, err := parseNullTime(createdDate)
	if err != nil {
		return tea.Artifact{}, err
	}

	distIDs, err := listArtifactDistributionIDs(ctx, q, uuid, version)
	if err != nil {
		return tea.Artifact{}, err
	}
	formats, err := listArtifactFormats(ctx, q, uuid, version)
	if err != nil {
		return tea.Artifact{}, err
	}

	return tea.Artifact{
		UUID:            uuid,
		Version:         version,
		Name:            name.String,
		Type:            artifactType,
		CreatedDate:     created,
		DistributionIDs: distIDs,
		Formats:         formats,
	}, nil
}

func listArtifactDistributionIDs(ctx context.Context, q dbtx, uuid string, version int) ([]string, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT distribution_id FROM artifact_distribution WHERE artifact_uuid = ? AND artifact_version = ?`, uuid, version)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func listArtifactFormats(ctx context.Context, q dbtx, uuid string, version int) ([]tea.ArtifactFormat, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, media_type, description, url, signature_url FROM artifact_format WHERE artifact_uuid = ? AND artifact_version = ? ORDER BY rowid`, uuid, version)
	if err != nil {
		return nil, err
	}
	type row struct {
		id, mediaType         string
		description, url, sig sql.NullString
	}
	var rowsData []row
	for rows.Next() {
		var rr row
		if err := rows.Scan(&rr.id, &rr.mediaType, &rr.description, &rr.url, &rr.sig); err != nil {
			_ = rows.Close()
			return nil, err
		}
		rowsData = append(rowsData, rr)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	out := []tea.ArtifactFormat{}
	for _, rr := range rowsData {
		checksums, err := listChecksums(ctx, q, OwnerArtifactFormat, rr.id)
		if err != nil {
			return nil, err
		}
		out = append(out, tea.ArtifactFormat{
			MediaType:    rr.mediaType,
			Description:  rr.description.String,
			URL:          rr.url.String,
			SignatureURL: rr.sig.String,
			Checksums:    checksums,
		})
	}
	return out, nil
}

// ErrNoMatchingFormat is returned by GetArtifactFormatForDownload/
// GetArtifactFormatForSignatureDownload when the artifact revision exists
// but no format matches the requested mediaType (TEA 1.0, spec/openapi.yaml:
// 406 + NO_ACCEPTABLE_FORMAT) -- distinct from ErrNotFound, which means the
// artifact/version itself doesn't exist or is concealed (404 + OBJECT_UNKNOWN).
var ErrNoMatchingFormat = errors.New("repo: no artifact format matches the requested mediaType")

// ErrSignatureNotFound is returned by GetArtifactFormatForSignatureDownload
// when the matched format has neither a self-hosted nor an external
// signature published (TEA 1.0: 404 + SIGNATURE_NOT_FOUND, distinct from
// ErrNotFound/ErrNoMatchingFormat's OBJECT_UNKNOWN -- the spec defines this
// exact distinction for signatures specifically).
var ErrSignatureNotFound = errors.New("repo: no signature is published for this artifact format")

// GetArtifactFormatForDownload resolves (uuid, version, mediaType) to the
// matched format's content location for the download endpoints
// (internal/api/artifactdownload.go): sha256Hex is set when the content is
// self-hosted (retrieve via storage.Storage.Open), externalURL is set when
// it's hosted externally (redirect, TEA 1.0's "url" field) -- exactly one of
// the two is ever set on success. mediaType matching is case-insensitive
// for the whole value (TEA 1.0: "case-insensitive for the type and
// subtype"). Returns ErrNotFound if (uuid, version) doesn't exist, or if a
// format matches mediaType but has no content published yet (opentea's own
// two-step create-then-upload flow can leave a format in that state
// transiently; the spec has no distinct signal for it the way it does for
// signatures, so this collapses to the same "doesn't exist" response as a
// genuinely absent object rather than inventing one). Returns
// ErrNoMatchingFormat if the revision exists but no format matches
// mediaType at all.
func (r *Repo) GetArtifactFormatForDownload(ctx context.Context, uuid string, version int, mediaType string) (sha256Hex, externalURL string, err error) {
	if _, err := r.GetArtifactType(ctx, uuid, version); err != nil {
		return "", "", err
	}
	formats, err := listArtifactFormats(ctx, r.conn(), uuid, version)
	if err != nil {
		return "", "", err
	}
	for _, f := range formats {
		if !strings.EqualFold(f.MediaType, mediaType) {
			continue
		}
		if f.URL != "" {
			return "", f.URL, nil
		}
		for _, c := range f.Checksums {
			if c.AlgType == "SHA-256" {
				return c.AlgValue, "", nil
			}
		}
		return "", "", ErrNotFound
	}
	return "", "", ErrNoMatchingFormat
}

// GetArtifactFormatForSignatureDownload is GetArtifactFormatForDownload's
// counterpart for the signature download endpoints, resolving against
// signature_url/signature_sha256 instead of url/the content checksum.
// Returns ErrSignatureNotFound (not ErrNotFound) when a format matches
// mediaType but has neither -- the spec's own distinct signal for this
// case, unlike content's equivalent state (see GetArtifactFormatForDownload's
// own doc comment on that asymmetry).
func (r *Repo) GetArtifactFormatForSignatureDownload(ctx context.Context, uuid string, version int, mediaType string) (sha256Hex, externalURL string, err error) {
	if _, err := r.GetArtifactType(ctx, uuid, version); err != nil {
		return "", "", err
	}
	_, sig, err := getArtifactFormatSignatureTx(ctx, r.conn(), uuid, version, mediaType)
	if err != nil {
		return "", "", err
	}
	if sig.url != "" {
		return "", sig.url, nil
	}
	if sig.sha256 != "" {
		return sig.sha256, "", nil
	}
	return "", "", ErrSignatureNotFound
}

type artifactFormatSignature struct {
	url, sha256 string
}

// getArtifactFormatSignatureTx finds the format matching mediaType
// (case-insensitive) among (uuid, version)'s formats and returns its id
// plus its raw signature_url/signature_sha256 values (listArtifactFormats
// doesn't carry signature_sha256, only the public signatureUrl, so this
// queries directly rather than extending that shared helper's return shape
// for one internal-only field). Returns ErrNoMatchingFormat if no format
// matches mediaType.
func getArtifactFormatSignatureTx(ctx context.Context, q dbtx, uuid string, version int, mediaType string) (formatID string, sig artifactFormatSignature, err error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, media_type, signature_url, signature_sha256 FROM artifact_format WHERE artifact_uuid = ? AND artifact_version = ? ORDER BY rowid`,
		uuid, version,
	)
	if err != nil {
		return "", artifactFormatSignature{}, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id, rowMediaType string
		var url, sha256Hex sql.NullString
		if err := rows.Scan(&id, &rowMediaType, &url, &sha256Hex); err != nil {
			return "", artifactFormatSignature{}, err
		}
		if !strings.EqualFold(rowMediaType, mediaType) {
			continue
		}
		return id, artifactFormatSignature{url: url.String, sha256: sha256Hex.String}, nil
	}
	if err := rows.Err(); err != nil {
		return "", artifactFormatSignature{}, err
	}
	return "", artifactFormatSignature{}, ErrNoMatchingFormat
}

// SetArtifactFormatFile records an uploaded file for the formatIndex-th
// format (0-based, in creation order) of an artifact revision: adds a
// SHA-256 checksum row locating the content in blob storage (self-hosted
// content is retrieved via the download endpoints,
// internal/api/artifactdownload.go, resolved from this checksum -- TEA
// 1.0's url field is reserved for genuinely external locations, so this no
// longer sets it). Runs as one transaction (format lookup, checksum
// insert, and the revision bump below) so the revision that backs this
// artifact version's ETag can never observe a partial version of this
// change.
func (r *Repo) SetArtifactFormatFile(ctx context.Context, artifactUUID string, artifactVersion, formatIndex int, sha256Hex string) (tea.Artifact, error) {
	return runInTx(ctx, r, func(tx dbtx) (tea.Artifact, error) {
		rows, err := tx.QueryContext(ctx,
			`SELECT id FROM artifact_format WHERE artifact_uuid = ? AND artifact_version = ? ORDER BY rowid`, artifactUUID, artifactVersion)
		if err != nil {
			return tea.Artifact{}, err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return tea.Artifact{}, err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return tea.Artifact{}, err
		}
		_ = rows.Close()

		if formatIndex < 0 || formatIndex >= len(ids) {
			return tea.Artifact{}, ErrNotFound
		}
		formatID := ids[formatIndex]

		if err := insertChecksums(ctx, tx, OwnerArtifactFormat, formatID, []tea.Checksum{{AlgType: "SHA-256", AlgValue: sha256Hex}}); err != nil {
			return tea.Artifact{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE artifact SET revision = revision + 1 WHERE uuid = ? AND version = ?`, artifactUUID, artifactVersion); err != nil {
			return tea.Artifact{}, err
		}

		return getArtifactByVersionTx(ctx, tx, artifactUUID, artifactVersion)
	})
}

// SetArtifactFormatSignatureFile is SetArtifactFormatFile's counterpart
// for a self-hosted detached signature: records sha256Hex as the
// formatIndex-th format's signature_sha256 (its blob-storage locator for
// internal/api/artifactdownload.go's signature download endpoints).
// Deliberately does not touch signature_url -- every write path only ever
// sets one of the two (self-hosted or external), never both, so there's
// no conflict to resolve here, just nothing to clear.
func (r *Repo) SetArtifactFormatSignatureFile(ctx context.Context, artifactUUID string, artifactVersion, formatIndex int, sha256Hex string) (tea.Artifact, error) {
	return runInTx(ctx, r, func(tx dbtx) (tea.Artifact, error) {
		rows, err := tx.QueryContext(ctx,
			`SELECT id FROM artifact_format WHERE artifact_uuid = ? AND artifact_version = ? ORDER BY rowid`, artifactUUID, artifactVersion)
		if err != nil {
			return tea.Artifact{}, err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return tea.Artifact{}, err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return tea.Artifact{}, err
		}
		_ = rows.Close()

		if formatIndex < 0 || formatIndex >= len(ids) {
			return tea.Artifact{}, ErrNotFound
		}
		formatID := ids[formatIndex]

		if _, err := tx.ExecContext(ctx, `UPDATE artifact_format SET signature_sha256 = ? WHERE id = ?`, sha256Hex, formatID); err != nil {
			return tea.Artifact{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE artifact SET revision = revision + 1 WHERE uuid = ? AND version = ?`, artifactUUID, artifactVersion); err != nil {
			return tea.Artifact{}, err
		}

		return getArtifactByVersionTx(ctx, tx, artifactUUID, artifactVersion)
	})
}

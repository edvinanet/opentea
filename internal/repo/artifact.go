package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/oej/opentea/internal/idgen"
	"github.com/oej/opentea/pkg/tea"
)

type ArtifactFormatInput struct {
	MediaType   string
	Description string
}

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
	defer tx.Rollback()

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
// version) if it doesn't already exist -- see ImportProduct for the
// identity/idempotency rationale. Artifacts are versioned, so re-importing
// the same (uuid, version) is a no-op, but a bundle can still introduce a
// new version of an artifact the target already has some revisions of.
func (r *Repo) ImportArtifact(ctx context.Context, in ImportArtifactInput) (created bool, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM artifact WHERE uuid = ? AND version = ?`, in.UUID, in.Version).Scan(&exists); err == nil {
		return false, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
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

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repo) GetArtifactLatest(ctx context.Context, uuid string) (tea.Artifact, error) {
	var version sql.NullInt64
	if err := r.db.QueryRowContext(ctx, `SELECT MAX(version) FROM artifact WHERE uuid = ?`, uuid).Scan(&version); err != nil {
		return tea.Artifact{}, err
	}
	if !version.Valid {
		return tea.Artifact{}, ErrNotFound
	}
	return r.GetArtifactByVersion(ctx, uuid, int(version.Int64))
}

func (r *Repo) GetArtifactByVersion(ctx context.Context, uuid string, version int) (tea.Artifact, error) {
	var name sql.NullString
	var artifactType string
	var createdDate sql.NullString
	err := r.db.QueryRowContext(ctx,
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

	distIDs, err := listArtifactDistributionIDs(ctx, r.db, uuid, version)
	if err != nil {
		return tea.Artifact{}, err
	}
	formats, err := listArtifactFormats(ctx, r.db, uuid, version)
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
	defer rows.Close()

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
			rows.Close()
			return nil, err
		}
		rowsData = append(rowsData, rr)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

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

// SetArtifactFormatFile records an uploaded file for the formatIndex-th
// format (0-based, in creation order) of an artifact revision: sets its
// download URL and adds a SHA-256 checksum row.
func (r *Repo) SetArtifactFormatFile(ctx context.Context, artifactUUID string, artifactVersion, formatIndex int, url, sha256Hex string) (tea.Artifact, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id FROM artifact_format WHERE artifact_uuid = ? AND artifact_version = ? ORDER BY rowid`, artifactUUID, artifactVersion)
	if err != nil {
		return tea.Artifact{}, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return tea.Artifact{}, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return tea.Artifact{}, err
	}
	rows.Close()

	if formatIndex < 0 || formatIndex >= len(ids) {
		return tea.Artifact{}, ErrNotFound
	}
	formatID := ids[formatIndex]

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return tea.Artifact{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE artifact_format SET url = ? WHERE id = ?`, url, formatID); err != nil {
		return tea.Artifact{}, err
	}
	if err := insertChecksums(ctx, tx, OwnerArtifactFormat, formatID, []tea.Checksum{{AlgType: "SHA-256", AlgValue: sha256Hex}}); err != nil {
		return tea.Artifact{}, err
	}
	if err := tx.Commit(); err != nil {
		return tea.Artifact{}, err
	}
	return r.GetArtifactByVersion(ctx, artifactUUID, artifactVersion)
}

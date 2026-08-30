// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/oej/opentea/internal/pagination"
	"github.com/oej/opentea/pkg/tea"
)

// ArtifactRef identifies one artifact revision (by its versioned identity)
// to include in a collection.
type ArtifactRef struct {
	UUID    string
	Version int
}

// BelongsToProductRelease and BelongsToComponentRelease are the two valid
// values of collection.belongs_to (matching the column's own CHECK
// constraint, internal/db/migrations/0001_init.sql) -- named here so
// callers pass a real, type-checked constant instead of a bare string
// literal, particularly at the read-path call sites that use it to scope a
// lookup to one release type (see getCollectionByVersionTx).
const (
	BelongsToProductRelease   = "PRODUCT_RELEASE"
	BelongsToComponentRelease = "COMPONENT_RELEASE"
)

// CollectionInput carries the fields needed to publish a new collection version.
type CollectionInput struct {
	UpdateReason *tea.UpdateReason
	Artifacts    []ArtifactRef
}

// CreateCollectionForComponentRelease publishes a new collection version
// for componentReleaseUUID (version auto-incremented from any existing
// collection for it). Returns ErrNotFound if componentReleaseUUID doesn't
// exist. Composes into an outer WithTx when called through the Repo a
// WithTx callback receives (see createCollection).
func (r *Repo) CreateCollectionForComponentRelease(ctx context.Context, componentReleaseUUID string, in CollectionInput) (tea.Collection, error) {
	var exists int
	if err := r.conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM component_release WHERE uuid = ?`, componentReleaseUUID).Scan(&exists); err != nil {
		return tea.Collection{}, err
	}
	if exists == 0 {
		return tea.Collection{}, ErrNotFound
	}
	return r.createCollection(ctx, componentReleaseUUID, BelongsToComponentRelease, in)
}

// CreateCollectionForProductRelease publishes a new collection version for
// productReleaseUUID (version auto-incremented from any existing collection
// for it). Returns ErrNotFound if productReleaseUUID doesn't exist.
// Composes into an outer WithTx when called through the Repo a WithTx
// callback receives (see createCollection).
func (r *Repo) CreateCollectionForProductRelease(ctx context.Context, productReleaseUUID string, in CollectionInput) (tea.Collection, error) {
	var exists int
	if err := r.conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM product_release WHERE uuid = ?`, productReleaseUUID).Scan(&exists); err != nil {
		return tea.Collection{}, err
	}
	if exists == 0 {
		return tea.Collection{}, ErrNotFound
	}
	return r.createCollection(ctx, productReleaseUUID, BelongsToProductRelease, in)
}

// createCollection runs via runInTx (like CreateEvidenceBundle) rather than
// a private r.db.BeginTx, so a caller assembling an atomic multi-step
// operation -- internal/publisher's commitCollectionDraft, which must
// create the collection, create its evidence bundle, and delete the draft
// all-or-nothing -- can call CreateCollectionForProductRelease/
// ComponentRelease from inside its own repo.WithTx callback and have it
// compose into that one transaction instead of opening a second, separate
// one (design/publisher-service.md §8's explicit call-out of this pattern).
func (r *Repo) createCollection(ctx context.Context, ownerUUID, belongsTo string, in CollectionInput) (tea.Collection, error) {
	return runInTx(ctx, r, func(tx dbtx) (tea.Collection, error) {
		var maxVersion sql.NullInt64
		if err := tx.QueryRowContext(ctx, `SELECT MAX(version) FROM collection WHERE uuid = ?`, ownerUUID).Scan(&maxVersion); err != nil {
			return tea.Collection{}, err
		}
		version := 1
		if maxVersion.Valid {
			version = int(maxVersion.Int64) + 1
		}

		now := time.Now()
		var reasonType, reasonComment any
		if in.UpdateReason != nil {
			reasonType = in.UpdateReason.Type
			if in.UpdateReason.Comment != "" {
				reasonComment = in.UpdateReason.Comment
			}
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO collection (uuid, version, date, belongs_to, update_reason_type, update_reason_comment) VALUES (?, ?, ?, ?, ?, ?)`,
			ownerUUID, version, formatTime(now), belongsTo, reasonType, reasonComment,
		); err != nil {
			return tea.Collection{}, err
		}

		for _, a := range in.Artifacts {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO collection_artifact (collection_uuid, collection_version, artifact_uuid, artifact_version) VALUES (?, ?, ?, ?)`,
				ownerUUID, version, a.UUID, a.Version,
			); err != nil {
				return tea.Collection{}, err
			}
		}

		if err := bumpWatermarkTx(ctx, tx, WatermarkCollections); err != nil {
			return tea.Collection{}, err
		}

		return getCollectionByVersionTx(ctx, tx, ownerUUID, version, belongsTo)
	})
}

// ImportCollectionInput carries the explicit (uuid, version) identity a
// bundle's collections are keyed by.
type ImportCollectionInput struct {
	UUID         string
	Version      int
	Date         time.Time
	BelongsTo    string
	UpdateReason *tea.UpdateReason
	Artifacts    []ArtifactRef
}

// ImportCollection creates the collection version if it doesn't already
// exist, or leaves an existing one untouched if its content matches -- see
// ImportProduct for the identity/idempotency rationale. Like artifacts,
// collections are versioned, so a bundle can introduce a new version of a
// collection the target already has earlier versions of. Returns
// ErrImportIdentityConflict if (uuid, version) already exists with
// different content.
func (r *Repo) ImportCollection(ctx context.Context, in ImportCollectionInput) (created bool, err error) {
	return runInTx(ctx, r, func(tx dbtx) (bool, error) {
		existing, err := getCollectionByVersionUnfilteredTx(ctx, tx, in.UUID, in.Version)
		if err == nil {
			if collectionConflicts(existing, in) {
				return false, fmt.Errorf("%w: collection %s version %d", ErrImportIdentityConflict, in.UUID, in.Version)
			}
			return false, nil
		} else if !errors.Is(err, ErrNotFound) {
			return false, err
		}

		var reasonType, reasonComment any
		if in.UpdateReason != nil {
			reasonType = in.UpdateReason.Type
			if in.UpdateReason.Comment != "" {
				reasonComment = in.UpdateReason.Comment
			}
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO collection (uuid, version, date, belongs_to, update_reason_type, update_reason_comment) VALUES (?, ?, ?, ?, ?, ?)`,
			in.UUID, in.Version, formatTime(in.Date), in.BelongsTo, reasonType, reasonComment,
		); err != nil {
			return false, err
		}

		for _, a := range in.Artifacts {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO collection_artifact (collection_uuid, collection_version, artifact_uuid, artifact_version) VALUES (?, ?, ?, ?)`,
				in.UUID, in.Version, a.UUID, a.Version,
			); err != nil {
				return false, err
			}
		}

		if err := bumpWatermarkTx(ctx, tx, WatermarkCollections); err != nil {
			return false, err
		}
		return true, nil
	})
}

// collectionConflicts reports whether existing (already stored) differs
// from in (being imported) in a way that means they're not the same
// collection version. Artifacts is reduced to a set of {UUID,Version}
// pairs rather than compared by full artifact body: an artifact-content
// conflict is already caught by ImportArtifact itself, which runs before
// ImportCollection in internal/bundle/import.go's import order --
// comparing full bodies here would double-report the same conflict under
// a confusing "collection conflict" label.
func collectionConflicts(existing tea.Collection, in ImportCollectionInput) bool {
	if existing.BelongsTo != in.BelongsTo {
		return true
	}
	if !existing.Date.Equal(in.Date) {
		return true
	}
	if !ptrEqual(existing.UpdateReason, in.UpdateReason) {
		return true
	}
	existingRefs := make([]ArtifactRef, len(existing.Artifacts))
	for i, a := range existing.Artifacts {
		existingRefs[i] = ArtifactRef{UUID: a.UUID, Version: a.Version}
	}
	return !setEqual(existingRefs, in.Artifacts)
}

// GetLatestCollection fetches the highest-versioned collection for
// ownerUUID whose belongs_to matches belongsTo (BelongsToProductRelease or
// BelongsToComponentRelease). Returns ErrNotFound if ownerUUID has no
// collections of that type yet -- including when ownerUUID exists but only
// as the *other* release type's collection, so a product-release route
// can't be used to read a component-release's collection (or vice versa)
// by supplying its UUID.
func (r *Repo) GetLatestCollection(ctx context.Context, ownerUUID, belongsTo string) (tea.Collection, error) {
	version, ok, err := latestCollectionVersionTx(ctx, r.conn(), ownerUUID, belongsTo)
	if err != nil {
		return tea.Collection{}, err
	}
	if !ok {
		return tea.Collection{}, ErrNotFound
	}
	return r.GetCollectionByVersion(ctx, ownerUUID, version, belongsTo)
}

// LatestCollectionVersion fetches just the highest existing collection
// version for (ownerUUID, belongsTo) -- the public form of
// latestCollectionVersionTx, for ETag construction on endpoints that embed
// "the latest collection" without needing GetLatestCollection's full fetch
// (or its ErrNotFound-on-none semantics: ok is false, not an error, when
// ownerUUID has no collections of this type yet).
func (r *Repo) LatestCollectionVersion(ctx context.Context, ownerUUID, belongsTo string) (version int, ok bool, err error) {
	return latestCollectionVersionTx(ctx, r.conn(), ownerUUID, belongsTo)
}

// latestCollectionVersionTx fetches just the highest existing collection
// version for (ownerUUID, belongsTo), without fetching the collection
// itself -- for ETag construction on endpoints that embed "the latest
// collection" (GetLatestCollection itself, and getComponentReleaseWithCollection's
// aggregate ETag), so a conditional GET can check for a newly-published
// collection (or the first one ever) via one indexed MAX() lookup instead
// of the full collection+artifacts fetch. ok is false if ownerUUID has no
// collections of this type yet -- not an error, since "no collection yet"
// is a normal, valid state (distinguished from GetLatestCollection's own
// ErrNotFound, which callers there want as an error).
func latestCollectionVersionTx(ctx context.Context, q dbtx, ownerUUID, belongsTo string) (version int, ok bool, err error) {
	var v sql.NullInt64
	if err := q.QueryRowContext(ctx,
		`SELECT MAX(version) FROM collection WHERE uuid = ? AND belongs_to = ?`, ownerUUID, belongsTo,
	).Scan(&v); err != nil {
		return 0, false, err
	}
	if !v.Valid {
		return 0, false, nil
	}
	return int(v.Int64), true, nil
}

// GetCollectionByVersion fetches one specific collection version for
// ownerUUID, including its artifacts. Returns ErrNotFound if that
// (ownerUUID, version) pair doesn't exist under belongsTo -- see
// GetLatestCollection's doc comment for why the type is part of the lookup.
func (r *Repo) GetCollectionByVersion(ctx context.Context, ownerUUID string, version int, belongsTo string) (tea.Collection, error) {
	return getCollectionByVersionTx(ctx, r.conn(), ownerUUID, version, belongsTo)
}

// ExistsCollectionVersion reports whether (ownerUUID, version, belongsTo)
// exists, without fetching the rest of the row -- for ETag construction: a
// specific collection version has no update path in this codebase
// (insert-only), so its representation is provably immutable once created
// and its ETag needs no revision counter, just this one cheap existence
// check (still required, not skippable -- a collection version cascade-
// deletes with its owning release, so a client's cached ETag for a
// since-deleted release's collection must not keep matching forever).
// Returns ErrNotFound if it doesn't exist.
func (r *Repo) ExistsCollectionVersion(ctx context.Context, ownerUUID string, version int, belongsTo string) error {
	return existsCollectionVersionTx(ctx, r.conn(), ownerUUID, version, belongsTo)
}

func existsCollectionVersionTx(ctx context.Context, q dbtx, ownerUUID string, version int, belongsTo string) error {
	var exists int
	err := q.QueryRowContext(ctx,
		`SELECT 1 FROM collection WHERE uuid = ? AND version = ? AND belongs_to = ?`, ownerUUID, version, belongsTo,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// getCollectionByVersionTx is GetCollectionByVersion's logic parameterized
// over a dbtx -- see product.go's getProductTx doc comment for why this
// exists.
func getCollectionByVersionTx(ctx context.Context, q dbtx, ownerUUID string, version int, belongsTo string) (tea.Collection, error) {
	var (
		date                  string
		gotBelongsTo          string
		reasonType, reasonCmt sql.NullString
	)
	err := q.QueryRowContext(ctx,
		`SELECT date, belongs_to, update_reason_type, update_reason_comment FROM collection WHERE uuid = ? AND version = ? AND belongs_to = ?`,
		ownerUUID, version, belongsTo,
	).Scan(&date, &gotBelongsTo, &reasonType, &reasonCmt)
	if errors.Is(err, sql.ErrNoRows) {
		return tea.Collection{}, ErrNotFound
	}
	if err != nil {
		return tea.Collection{}, err
	}

	d, err := parseTime(date)
	if err != nil {
		return tea.Collection{}, err
	}

	artifacts, err := listCollectionArtifacts(ctx, q, ownerUUID, version)
	if err != nil {
		return tea.Collection{}, err
	}

	c := tea.Collection{
		UUID:      ownerUUID,
		Version:   version,
		Date:      d,
		BelongsTo: gotBelongsTo,
		Artifacts: artifacts,
	}
	if reasonType.Valid {
		c.UpdateReason = &tea.UpdateReason{Type: reasonType.String, Comment: reasonCmt.String}
	}
	return c, nil
}

// getCollectionByVersionUnfilteredTx is getCollectionByVersionTx without
// the belongs_to filter -- used ONLY by ImportCollection's own conflict
// check, which already separately compares BelongsTo in collectionConflicts
// and returns the friendlier ErrImportIdentityConflict on a type mismatch,
// instead of this query simply reporting ErrNotFound and letting the
// subsequent INSERT hit the raw collection(uuid, version) primary-key
// constraint. Do not reach for this from any other caller -- every other
// caller should go through getCollectionByVersionTx with a real
// BelongsToProductRelease/BelongsToComponentRelease value, since that's
// what keeps a product-release collection from being readable through a
// component-release route (or vice versa) when their UUIDs happen to
// collide (see GetLatestCollection's doc comment).
func getCollectionByVersionUnfilteredTx(ctx context.Context, q dbtx, ownerUUID string, version int) (tea.Collection, error) {
	var (
		date                  string
		belongsTo             string
		reasonType, reasonCmt sql.NullString
	)
	err := q.QueryRowContext(ctx,
		`SELECT date, belongs_to, update_reason_type, update_reason_comment FROM collection WHERE uuid = ? AND version = ?`, ownerUUID, version,
	).Scan(&date, &belongsTo, &reasonType, &reasonCmt)
	if errors.Is(err, sql.ErrNoRows) {
		return tea.Collection{}, ErrNotFound
	}
	if err != nil {
		return tea.Collection{}, err
	}

	d, err := parseTime(date)
	if err != nil {
		return tea.Collection{}, err
	}

	artifacts, err := listCollectionArtifacts(ctx, q, ownerUUID, version)
	if err != nil {
		return tea.Collection{}, err
	}

	c := tea.Collection{
		UUID:      ownerUUID,
		Version:   version,
		Date:      d,
		BelongsTo: belongsTo,
		Artifacts: artifacts,
	}
	if reasonType.Valid {
		c.UpdateReason = &tea.UpdateReason{Type: reasonType.String, Comment: reasonCmt.String}
	}
	return c, nil
}

func listCollectionArtifacts(ctx context.Context, q dbtx, ownerUUID string, version int) ([]tea.Artifact, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT artifact_uuid, artifact_version FROM collection_artifact WHERE collection_uuid = ? AND collection_version = ? ORDER BY rowid`,
		ownerUUID, version)
	if err != nil {
		return nil, err
	}
	type ref struct {
		uuid    string
		version int
	}
	var refs []ref
	for rows.Next() {
		var rf ref
		if err := rows.Scan(&rf.uuid, &rf.version); err != nil {
			_ = rows.Close()
			return nil, err
		}
		refs = append(refs, rf)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	out := []tea.Artifact{}
	for _, rf := range refs {
		a, err := getArtifactByVersionTx(ctx, q, rf.uuid, rf.version)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// ListCollections returns up to limit collection versions for ownerUUID
// whose belongs_to matches belongsTo -- see GetLatestCollection's doc
// comment for why. "version" is the only allowed sortField.
func (r *Repo) ListCollections(ctx context.Context, ownerUUID, sortOrder string, cursor *pagination.Cursor, limit int, belongsTo string) ([]tea.Collection, error) {
	query := `SELECT version FROM collection WHERE uuid = ? AND belongs_to = ?`
	args := []any{ownerUUID, belongsTo}

	pq := pageQuery{SortColumn: "version", SortOrder: sortOrder}
	if cursor != nil {
		pq.Cursor = &cursorValue{LastValue: cursor.LastValue, LastUUID: cursor.LastUUID}
	}
	where, whereArgs := pq.whereClause()
	query += where
	args = append(args, whereArgs...)
	query += pq.orderByClause() + " LIMIT ?" //nolint:gosec // orderByClause's SortColumn is the hardcoded literal "version" here, never user input
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	var versions []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			_ = rows.Close()
			return nil, err
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	out := make([]tea.Collection, 0, len(versions))
	for _, v := range versions {
		c, err := r.GetCollectionByVersion(ctx, ownerUUID, v, belongsTo)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

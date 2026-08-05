package repo

import (
	"context"
	"database/sql"
	"errors"
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

// CollectionInput carries the fields needed to publish a new collection version.
type CollectionInput struct {
	UpdateReason *tea.UpdateReason
	Artifacts    []ArtifactRef
}

// CreateCollectionForComponentRelease publishes a new collection version
// for componentReleaseUUID (version auto-incremented from any existing
// collection for it). Returns ErrNotFound if componentReleaseUUID doesn't exist.
func (r *Repo) CreateCollectionForComponentRelease(ctx context.Context, componentReleaseUUID string, in CollectionInput) (tea.Collection, error) {
	var exists int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM component_release WHERE uuid = ?`, componentReleaseUUID).Scan(&exists); err != nil {
		return tea.Collection{}, err
	}
	if exists == 0 {
		return tea.Collection{}, ErrNotFound
	}
	return r.createCollection(ctx, componentReleaseUUID, "COMPONENT_RELEASE", in)
}

// CreateCollectionForProductRelease publishes a new collection version for
// productReleaseUUID (version auto-incremented from any existing collection
// for it). Returns ErrNotFound if productReleaseUUID doesn't exist.
func (r *Repo) CreateCollectionForProductRelease(ctx context.Context, productReleaseUUID string, in CollectionInput) (tea.Collection, error) {
	var exists int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM product_release WHERE uuid = ?`, productReleaseUUID).Scan(&exists); err != nil {
		return tea.Collection{}, err
	}
	if exists == 0 {
		return tea.Collection{}, ErrNotFound
	}
	return r.createCollection(ctx, productReleaseUUID, "PRODUCT_RELEASE", in)
}

func (r *Repo) createCollection(ctx context.Context, ownerUUID, belongsTo string, in CollectionInput) (tea.Collection, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return tea.Collection{}, err
	}
	defer func() { _ = tx.Rollback() }()

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

	if err := tx.Commit(); err != nil {
		return tea.Collection{}, err
	}
	return r.GetCollectionByVersion(ctx, ownerUUID, version)
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
// exist -- see ImportProduct for the identity/idempotency rationale. Like
// artifacts, collections are versioned, so a bundle can introduce a new
// version of a collection the target already has earlier versions of.
func (r *Repo) ImportCollection(ctx context.Context, in ImportCollectionInput) (created bool, err error) {
	return runInTx(ctx, r, func(tx dbtx) (bool, error) {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM collection WHERE uuid = ? AND version = ?`, in.UUID, in.Version).Scan(&exists); err == nil {
			return false, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
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

		return true, nil
	})
}

// GetLatestCollection fetches the highest-versioned collection for
// ownerUUID. Returns ErrNotFound if ownerUUID has no collections yet.
func (r *Repo) GetLatestCollection(ctx context.Context, ownerUUID string) (tea.Collection, error) {
	var version sql.NullInt64
	if err := r.db.QueryRowContext(ctx, `SELECT MAX(version) FROM collection WHERE uuid = ?`, ownerUUID).Scan(&version); err != nil {
		return tea.Collection{}, err
	}
	if !version.Valid {
		return tea.Collection{}, ErrNotFound
	}
	return r.GetCollectionByVersion(ctx, ownerUUID, int(version.Int64))
}

// GetCollectionByVersion fetches one specific collection version for
// ownerUUID, including its artifacts. Returns ErrNotFound if that
// (ownerUUID, version) pair doesn't exist.
func (r *Repo) GetCollectionByVersion(ctx context.Context, ownerUUID string, version int) (tea.Collection, error) {
	var (
		date                  string
		belongsTo             string
		reasonType, reasonCmt sql.NullString
	)
	err := r.db.QueryRowContext(ctx,
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

	artifacts, err := r.listCollectionArtifacts(ctx, ownerUUID, version)
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

func (r *Repo) listCollectionArtifacts(ctx context.Context, ownerUUID string, version int) ([]tea.Artifact, error) {
	rows, err := r.db.QueryContext(ctx,
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
		a, err := r.GetArtifactByVersion(ctx, rf.uuid, rf.version)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// ListCollections returns up to limit collection versions for ownerUUID.
// "version" is the only allowed sortField.
func (r *Repo) ListCollections(ctx context.Context, ownerUUID, sortOrder string, cursor *pagination.Cursor, limit int) ([]tea.Collection, error) {
	query := `SELECT version FROM collection WHERE uuid = ?`
	args := []any{ownerUUID}

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
		c, err := r.GetCollectionByVersion(ctx, ownerUUID, v)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

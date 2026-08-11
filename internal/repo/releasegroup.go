package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/oej/opentea/internal/idgen"
	"github.com/oej/opentea/internal/model"
)

// CreateReleaseGroup creates a new, empty release group. Product-release-
// only in Phase 1 -- see internal/db/migrations/0005_authz.sql's header.
func (r *Repo) CreateReleaseGroup(ctx context.Context, name, description string) (model.ReleaseGroup, error) {
	uuid := idgen.New()
	if _, err := r.conn().ExecContext(ctx, `INSERT INTO release_group (uuid, name, description) VALUES (?, ?, ?)`, uuid, name, nullIfEmpty(description)); err != nil {
		return model.ReleaseGroup{}, err
	}
	return r.GetReleaseGroup(ctx, uuid)
}

// GetReleaseGroup fetches a release group by UUID. Returns ErrNotFound if
// uuid doesn't exist.
func (r *Repo) GetReleaseGroup(ctx context.Context, uuid string) (model.ReleaseGroup, error) {
	var g model.ReleaseGroup
	var description sql.NullString
	var createdAt string
	err := r.conn().QueryRowContext(ctx, `SELECT uuid, name, description, created_at FROM release_group WHERE uuid = ?`, uuid).
		Scan(&g.UUID, &g.Name, &description, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.ReleaseGroup{}, ErrNotFound
	}
	if err != nil {
		return model.ReleaseGroup{}, err
	}
	g.Description = description.String
	created, err := parseTime(createdAt)
	if err != nil {
		return model.ReleaseGroup{}, err
	}
	g.CreatedAt = created
	return g, nil
}

// ListReleaseGroups returns every release group, ordered by name.
func (r *Repo) ListReleaseGroups(ctx context.Context) ([]model.ReleaseGroup, error) {
	rows, err := r.conn().QueryContext(ctx, `SELECT uuid, name, description, created_at FROM release_group ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []model.ReleaseGroup{}
	for rows.Next() {
		var g model.ReleaseGroup
		var description sql.NullString
		var createdAt string
		if err := rows.Scan(&g.UUID, &g.Name, &description, &createdAt); err != nil {
			return nil, err
		}
		g.Description = description.String
		created, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		g.CreatedAt = created
		out = append(out, g)
	}
	return out, rows.Err()
}

// DeleteReleaseGroup deletes groupUUID and its membership rows (cascaded by
// the schema). Returns ErrNotFound if it doesn't exist.
func (r *Repo) DeleteReleaseGroup(ctx context.Context, uuid string) error {
	res, err := r.conn().ExecContext(ctx, `DELETE FROM release_group WHERE uuid = ?`, uuid)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// AddReleaseGroupMember adds productReleaseUUID to groupUUID. Idempotent:
// adding an already-present member is a no-op, not an error.
func (r *Repo) AddReleaseGroupMember(ctx context.Context, groupUUID, productReleaseUUID string) error {
	_, err := r.conn().ExecContext(ctx,
		`INSERT INTO release_group_member (release_group_uuid, product_release_uuid) VALUES (?, ?) ON CONFLICT DO NOTHING`,
		groupUUID, productReleaseUUID,
	)
	return err
}

// RemoveReleaseGroupMember removes productReleaseUUID from groupUUID.
// Returns ErrNotFound if that membership doesn't exist.
func (r *Repo) RemoveReleaseGroupMember(ctx context.Context, groupUUID, productReleaseUUID string) error {
	res, err := r.conn().ExecContext(ctx, `DELETE FROM release_group_member WHERE release_group_uuid = ? AND product_release_uuid = ?`, groupUUID, productReleaseUUID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListReleaseGroupMembers returns every product_release UUID belonging to
// groupUUID.
func (r *Repo) ListReleaseGroupMembers(ctx context.Context, groupUUID string) ([]string, error) {
	rows, err := r.conn().QueryContext(ctx, `SELECT product_release_uuid FROM release_group_member WHERE release_group_uuid = ? ORDER BY product_release_uuid`, groupUUID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []string{}
	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			return nil, err
		}
		out = append(out, uuid)
	}
	return out, rows.Err()
}

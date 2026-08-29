// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/oej/opentea/internal/idgen"
	"github.com/oej/opentea/internal/model"
)

// CreateProductGroup creates a new, empty product group.
func (r *Repo) CreateProductGroup(ctx context.Context, name, description string) (model.ProductGroup, error) {
	uuid := idgen.New()
	if _, err := r.conn().ExecContext(ctx, `INSERT INTO product_group (uuid, name, description) VALUES (?, ?, ?)`, uuid, name, nullIfEmpty(description)); err != nil {
		return model.ProductGroup{}, err
	}
	return r.GetProductGroup(ctx, uuid)
}

// GetProductGroup fetches a product group by UUID. Returns ErrNotFound if
// uuid doesn't exist.
func (r *Repo) GetProductGroup(ctx context.Context, uuid string) (model.ProductGroup, error) {
	var g model.ProductGroup
	var description sql.NullString
	var createdAt string
	err := r.conn().QueryRowContext(ctx, `SELECT uuid, name, description, created_at FROM product_group WHERE uuid = ?`, uuid).
		Scan(&g.UUID, &g.Name, &description, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.ProductGroup{}, ErrNotFound
	}
	if err != nil {
		return model.ProductGroup{}, err
	}
	g.Description = description.String
	created, err := parseTime(createdAt)
	if err != nil {
		return model.ProductGroup{}, err
	}
	g.CreatedAt = created
	return g, nil
}

// ListProductGroups returns every product group, ordered by name.
func (r *Repo) ListProductGroups(ctx context.Context) ([]model.ProductGroup, error) {
	rows, err := r.conn().QueryContext(ctx, `SELECT uuid, name, description, created_at FROM product_group ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []model.ProductGroup{}
	for rows.Next() {
		var g model.ProductGroup
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

// DeleteProductGroup deletes groupUUID and its membership rows (cascaded by
// the schema). Returns ErrNotFound if it doesn't exist. Entitlements
// scoped to this group are NOT deleted (entitlement.template_uuid's own FK
// governs templates, not this) -- resource_id isn't FK-enforced against
// product_group, so a dangling entitlement simply stops matching anything,
// same tradeoff as EntitlementInput's doc comment.
func (r *Repo) DeleteProductGroup(ctx context.Context, uuid string) error {
	res, err := r.conn().ExecContext(ctx, `DELETE FROM product_group WHERE uuid = ?`, uuid)
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

// AddProductGroupMember adds productUUID to groupUUID. Idempotent: adding
// an already-present member is a no-op, not an error.
func (r *Repo) AddProductGroupMember(ctx context.Context, groupUUID, productUUID string) error {
	_, err := r.conn().ExecContext(ctx,
		`INSERT INTO product_group_member (product_group_uuid, product_uuid) VALUES (?, ?) ON CONFLICT DO NOTHING`,
		groupUUID, productUUID,
	)
	return err
}

// RemoveProductGroupMember removes productUUID from groupUUID. Returns
// ErrNotFound if that membership doesn't exist.
func (r *Repo) RemoveProductGroupMember(ctx context.Context, groupUUID, productUUID string) error {
	res, err := r.conn().ExecContext(ctx, `DELETE FROM product_group_member WHERE product_group_uuid = ? AND product_uuid = ?`, groupUUID, productUUID)
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

// ListProductGroupMembers returns every product UUID belonging to
// groupUUID.
func (r *Repo) ListProductGroupMembers(ctx context.Context, groupUUID string) ([]string, error) {
	rows, err := r.conn().QueryContext(ctx, `SELECT product_uuid FROM product_group_member WHERE product_group_uuid = ? ORDER BY product_uuid`, groupUUID)
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

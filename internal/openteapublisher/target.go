// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/oej/opentea/internal/idgen"
)

// Target is one TEA server this deployment publishes to (Layer B,
// design/publisher-service.md §10.5, multi-target). BearerToken is stored
// as plaintext -- see db/migrations/0001_init.sql's own comment for why,
// and TODO.md for this v1 limitation. Plain CRUD only in this pass --
// nothing yet calls pkg/teapublisherclient against a stored Target.
type Target struct {
	UUID        string
	Label       string
	BaseURL     string
	BearerToken string
	CreatedAt   time.Time
}

// CreateTarget stores a new target.
func (r *Repo) CreateTarget(ctx context.Context, label, baseURL, bearerToken string) (Target, error) {
	uuid := idgen.New()
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO target (uuid, label, base_url, bearer_token) VALUES (?, ?, ?, ?)`,
		uuid, label, baseURL, bearerToken,
	); err != nil {
		return Target{}, err
	}
	return r.GetTarget(ctx, uuid)
}

// GetTarget fetches a target by UUID. Returns ErrNotFound if uuid doesn't exist.
func (r *Repo) GetTarget(ctx context.Context, uuid string) (Target, error) {
	var t Target
	var createdAt string
	err := r.db.QueryRowContext(ctx,
		`SELECT uuid, label, base_url, bearer_token, created_at FROM target WHERE uuid = ?`, uuid,
	).Scan(&t.UUID, &t.Label, &t.BaseURL, &t.BearerToken, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Target{}, ErrNotFound
	}
	if err != nil {
		return Target{}, err
	}
	created, err := parseTime(createdAt)
	if err != nil {
		return Target{}, err
	}
	t.CreatedAt = created
	return t, nil
}

// ListTargets returns every configured target, ordered by label.
// Unpaginated -- matches the current dashboard, which lists everyone at once.
func (r *Repo) ListTargets(ctx context.Context) ([]Target, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT uuid, label, base_url, bearer_token, created_at FROM target ORDER BY label`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []Target{}
	for rows.Next() {
		var t Target
		var createdAt string
		if err := rows.Scan(&t.UUID, &t.Label, &t.BaseURL, &t.BearerToken, &createdAt); err != nil {
			return nil, err
		}
		created, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		t.CreatedAt = created
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteTarget deletes the target identified by uuid. Returns ErrNotFound
// if uuid doesn't exist.
func (r *Repo) DeleteTarget(ctx context.Context, uuid string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM target WHERE uuid = ?`, uuid)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

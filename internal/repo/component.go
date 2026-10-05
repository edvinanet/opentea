// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/oej/opentea/internal/idgen"
	"github.com/oej/opentea/internal/pagination"
	"github.com/oej/opentea/pkg/tea"
)

// ErrComponentIdentifierConflict is returned by CreateComponent when one of
// the submitted identifiers already belongs to another component --
// server-side enforcement of the "search before create" convention
// /publisher/v1's findComponents/createComponent document, closing the
// race where two concurrent callers both search, both find nothing, and
// both create (found by external security review,
// docs/security-review-publisher-design-260828.md finding 12). Checked
// only when identifiers is non-empty -- a component with none has no
// server-checkable identity to deduplicate against.
var ErrComponentIdentifierConflict = errors.New("repo: an identifier in this request already belongs to another component")

// CreateComponent creates a new component with a fresh generated UUID.
// Returns ErrComponentIdentifierConflict if any of identifiers already
// belongs to another component -- the returned tea.Component is then that
// *existing* component (not the zero value), not one just created, so a
// caller can hand it straight back to its own caller (external security
// review, docs/security-review-publisher-design-260828.md finding 12:
// "return 409 with the existing component" -- a creator forced to
// separately search for what it just collided with is exactly the
// find-then-create race this whole mechanism exists to avoid repeating).
func (r *Repo) CreateComponent(ctx context.Context, name string, identifiers []tea.Identifier) (tea.Component, error) {
	uuid := idgen.New()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return tea.Component{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if conflictUUID, err := componentIdentifierConflictTx(ctx, tx, identifiers); err != nil {
		return tea.Component{}, err
	} else if conflictUUID != "" {
		existing, err := getComponentTx(ctx, tx, conflictUUID)
		if err != nil {
			return tea.Component{}, err
		}
		return existing, ErrComponentIdentifierConflict
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO component (uuid, name) VALUES (?, ?)`, uuid, name); err != nil {
		return tea.Component{}, err
	}
	if err := insertIdentifiers(ctx, tx, OwnerComponent, uuid, identifiers); err != nil {
		return tea.Component{}, err
	}
	if err := bumpWatermarkTx(ctx, tx, WatermarkComponents); err != nil {
		return tea.Component{}, err
	}
	if err := tx.Commit(); err != nil {
		return tea.Component{}, err
	}

	return tea.Component{UUID: uuid, Name: name, Identifiers: identifiers}, nil
}

// componentIdentifierConflictTx returns the UUID of an existing component
// that already owns one of identifiers, or "" if none does -- uses
// idx_identifier_lookup (owner_type, id_type, id_value), the same index
// name/identifier read paths already rely on.
func componentIdentifierConflictTx(ctx context.Context, q dbtx, identifiers []tea.Identifier) (string, error) {
	for _, id := range identifiers {
		var ownerUUID string
		err := q.QueryRowContext(ctx,
			`SELECT owner_uuid FROM identifier WHERE owner_type = ? AND id_type = ? AND id_value = ?`,
			OwnerComponent, id.IDType, id.IDValue,
		).Scan(&ownerUUID)
		if err == nil {
			return ownerUUID, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}
	return "", nil
}

// ImportComponent mirrors ImportProduct -- see there for the identity/
// idempotency/ErrImportIdentityConflict rationale.
func (r *Repo) ImportComponent(ctx context.Context, uuid, name string, identifiers []tea.Identifier) (created bool, err error) {
	return runInTx(ctx, r, func(tx dbtx) (bool, error) {
		existing, err := getComponentTx(ctx, tx, uuid)
		if err == nil {
			if existing.Name != name || !setEqual(existing.Identifiers, identifiers) {
				return false, fmt.Errorf("%w: component %s", ErrImportIdentityConflict, uuid)
			}
			return false, nil
		} else if !errors.Is(err, ErrNotFound) {
			return false, err
		}

		if _, err := tx.ExecContext(ctx, `INSERT INTO component (uuid, name) VALUES (?, ?)`, uuid, name); err != nil {
			return false, err
		}
		if err := insertIdentifiers(ctx, tx, OwnerComponent, uuid, identifiers); err != nil {
			return false, err
		}
		if err := bumpWatermarkTx(ctx, tx, WatermarkComponents); err != nil {
			return false, err
		}
		return true, nil
	})
}

// GetComponent fetches a component by UUID. Returns ErrNotFound if uuid doesn't exist.
func (r *Repo) GetComponent(ctx context.Context, uuid string) (tea.Component, error) {
	return getComponentTx(ctx, r.conn(), uuid)
}

// ExistsComponent reports whether uuid exists, without fetching the rest of
// the row -- see product.go's ExistsProduct doc comment for the rationale
// (component has the same no-update-path immutability, and the same
// still-required existence check given DeleteComponent is a real write path).
// Returns ErrNotFound if uuid doesn't exist.
func (r *Repo) ExistsComponent(ctx context.Context, uuid string) error {
	return existsComponentTx(ctx, r.conn(), uuid)
}

func existsComponentTx(ctx context.Context, q dbtx, uuid string) error {
	var exists int
	err := q.QueryRowContext(ctx, `SELECT 1 FROM component WHERE uuid = ?`, uuid).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// getComponentTx is GetComponent's logic parameterized over a dbtx --
// see product.go's getProductTx doc comment for why this exists.
func getComponentTx(ctx context.Context, q dbtx, uuid string) (tea.Component, error) {
	var name string
	err := q.QueryRowContext(ctx, `SELECT name FROM component WHERE uuid = ?`, uuid).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return tea.Component{}, ErrNotFound
	}
	if err != nil {
		return tea.Component{}, err
	}

	ids, err := listIdentifiers(ctx, q, OwnerComponent, uuid)
	if err != nil {
		return tea.Component{}, err
	}
	return tea.Component{UUID: uuid, Name: name, Identifiers: ids}, nil
}

// QueryComponents mirrors QueryProducts (see there for the pagination/filter
// design notes). Only "name" is a valid sortField for components.
func (r *Repo) QueryComponents(ctx context.Context, idType, idValue, sortField, sortOrder string, cursor *pagination.Cursor, limit int) ([]tea.Component, error) {
	switch sortField {
	case "name", "":
	default:
		return nil, errors.New("repo: unsupported sortField for component: " + sortField)
	}
	sortColumn := "name"

	query := `SELECT uuid, name FROM component WHERE 1=1`
	var args []any
	appendIdentifierFilter(&query, &args, OwnerComponent, "component.uuid", idType, idValue)

	pq := pageQuery{SortColumn: sortColumn, SortOrder: sortOrder}
	if cursor != nil {
		pq.Cursor = &cursorValue{LastValue: cursor.LastValue, LastUUID: cursor.LastUUID}
	}
	where, whereArgs := pq.whereClause()
	query += where
	args = append(args, whereArgs...)
	query += pq.orderByClause() + " LIMIT ?" //nolint:gosec // orderByClause's SortColumn always comes from a fixed, pre-validated allowlist, never raw input
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []tea.Component{}
	for rows.Next() {
		var c tea.Component
		if err := rows.Scan(&c.UUID, &c.Name); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range out {
		ids, err := listIdentifiers(ctx, r.db, OwnerComponent, out[i].UUID)
		if err != nil {
			return nil, err
		}
		out[i].Identifiers = ids
	}
	return out, nil
}

// SearchComponents returns up to limit components whose name contains q
// (case-insensitive substring match), for /publisher/v1's findComponents
// find-before-create workflow (design/publisher-openapi.yaml). Simplest
// useful implementation of the OpenAPI's free-text q search --
// QueryComponents' idType/idValue is an exact-match identifier filter, not
// this (and the actual integrity mechanism findComponents's own doc comment
// points callers at: CreateComponent's server-side uniqueness check, not
// this search -- see its own doc comment). An empty q matches everything
// (still capped by limit). hasNext reports whether more than limit
// components actually matched -- external security review,
// docs/security-review-publisher-design-260828.md finding 12 ("paginate and
// constrain component search"): this was previously silently truncated at
// the caller's limit with no way to tell. Not full cursor-based pagination
// (internal/pagination's heavier machinery, built for /tea/v1's read API) --
// this is a best-effort search aid, not an authoritative paginated list, so
// a plain "there were more, narrow your query" signal is proportionate.
func (r *Repo) SearchComponents(ctx context.Context, q string, limit int) (components []tea.Component, hasNext bool, err error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT uuid, name FROM component WHERE instr(lower(name), lower(?)) > 0 OR ? = '' ORDER BY name LIMIT ?`,
		q, q, limit+1,
	)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = rows.Close() }()

	out := []tea.Component{}
	for rows.Next() {
		var c tea.Component
		if err := rows.Scan(&c.UUID, &c.Name); err != nil {
			return nil, false, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}

	if len(out) > limit {
		out = out[:limit]
		hasNext = true
	}

	for i := range out {
		ids, err := listIdentifiers(ctx, r.db, OwnerComponent, out[i].UUID)
		if err != nil {
			return nil, false, err
		}
		out[i].Identifiers = ids
	}
	return out, hasNext, nil
}

// DeleteComponent deletes the component identified by uuid, cascading to
// its releases. Returns ErrNotFound if uuid doesn't exist.
func (r *Repo) DeleteComponent(ctx context.Context, uuid string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `DELETE FROM component WHERE uuid = ?`, uuid)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	if err := deleteOwnerScoped(ctx, tx, OwnerComponent, uuid); err != nil {
		return err
	}
	if err := bumpWatermarkTx(ctx, tx, WatermarkComponents); err != nil {
		return err
	}
	return tx.Commit()
}

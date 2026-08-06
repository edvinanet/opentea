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

// CreateComponent creates a new component with a fresh generated UUID.
func (r *Repo) CreateComponent(ctx context.Context, name string, identifiers []tea.Identifier) (tea.Component, error) {
	uuid := idgen.New()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return tea.Component{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `INSERT INTO component (uuid, name) VALUES (?, ?)`, uuid, name); err != nil {
		return tea.Component{}, err
	}
	if err := insertIdentifiers(ctx, tx, OwnerComponent, uuid, identifiers); err != nil {
		return tea.Component{}, err
	}
	if err := tx.Commit(); err != nil {
		return tea.Component{}, err
	}

	return tea.Component{UUID: uuid, Name: name, Identifiers: identifiers}, nil
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
		return true, nil
	})
}

// GetComponent fetches a component by UUID. Returns ErrNotFound if uuid doesn't exist.
func (r *Repo) GetComponent(ctx context.Context, uuid string) (tea.Component, error) {
	return getComponentTx(ctx, r.conn(), uuid)
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
	return tx.Commit()
}

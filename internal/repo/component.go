package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/oej/opentea/internal/idgen"
	"github.com/oej/opentea/internal/pagination"
	"github.com/oej/opentea/pkg/tea"
)

func (r *Repo) CreateComponent(ctx context.Context, name string, identifiers []tea.Identifier) (tea.Component, error) {
	uuid := idgen.New()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return tea.Component{}, err
	}
	defer tx.Rollback()

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
// idempotency rationale.
func (r *Repo) ImportComponent(ctx context.Context, uuid, name string, identifiers []tea.Identifier) (created bool, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM component WHERE uuid = ?`, uuid).Scan(&exists); err == nil {
		return false, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO component (uuid, name) VALUES (?, ?)`, uuid, name); err != nil {
		return false, err
	}
	if err := insertIdentifiers(ctx, tx, OwnerComponent, uuid, identifiers); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repo) GetComponent(ctx context.Context, uuid string) (tea.Component, error) {
	var name string
	err := r.db.QueryRowContext(ctx, `SELECT name FROM component WHERE uuid = ?`, uuid).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return tea.Component{}, ErrNotFound
	}
	if err != nil {
		return tea.Component{}, err
	}

	ids, err := listIdentifiers(ctx, r.db, OwnerComponent, uuid)
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
	query += pq.orderByClause() + " LIMIT ?"
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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

func (r *Repo) DeleteComponent(ctx context.Context, uuid string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

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

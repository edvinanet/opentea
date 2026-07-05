package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/oej/opentea/internal/idgen"
	"github.com/oej/opentea/internal/pagination"
	"github.com/oej/opentea/pkg/tea"
)

func (r *Repo) CreateProduct(ctx context.Context, name string, identifiers []tea.Identifier) (tea.Product, error) {
	uuid := idgen.New()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return tea.Product{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `INSERT INTO product (uuid, name) VALUES (?, ?)`, uuid, name); err != nil {
		return tea.Product{}, err
	}
	if err := insertIdentifiers(ctx, tx, OwnerProduct, uuid, identifiers); err != nil {
		return tea.Product{}, err
	}
	if err := tx.Commit(); err != nil {
		return tea.Product{}, err
	}

	return tea.Product{UUID: uuid, Name: name, Identifiers: identifiers}, nil
}

// ImportProduct creates the product with an explicit (caller-supplied) UUID
// if it doesn't already exist, preserving the source's identity so
// cross-references in an imported bundle resolve correctly. If a product
// with this UUID already exists, it's left untouched (created=false) --
// import is idempotent, not a merge.
func (r *Repo) ImportProduct(ctx context.Context, uuid, name string, identifiers []tea.Identifier) (created bool, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM product WHERE uuid = ?`, uuid).Scan(&exists); err == nil {
		return false, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO product (uuid, name) VALUES (?, ?)`, uuid, name); err != nil {
		return false, err
	}
	if err := insertIdentifiers(ctx, tx, OwnerProduct, uuid, identifiers); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repo) GetProduct(ctx context.Context, uuid string) (tea.Product, error) {
	var name string
	err := r.db.QueryRowContext(ctx, `SELECT name FROM product WHERE uuid = ?`, uuid).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return tea.Product{}, ErrNotFound
	}
	if err != nil {
		return tea.Product{}, err
	}

	ids, err := listIdentifiers(ctx, r.db, OwnerProduct, uuid)
	if err != nil {
		return tea.Product{}, err
	}
	return tea.Product{UUID: uuid, Name: name, Identifiers: ids}, nil
}

// QueryProducts returns up to limit products (callers pass pageSize+1 to
// detect a following page), filtered by idType/idValue and paginated via
// cursor. sortField must already be validated upstream (the only allowed
// value for products is "name").
func (r *Repo) QueryProducts(ctx context.Context, idType, idValue, sortField, sortOrder string, cursor *pagination.Cursor, limit int) ([]tea.Product, error) {
	sortColumn := "name"
	switch sortField {
	case "name", "":
		sortColumn = "name"
	default:
		return nil, errors.New("repo: unsupported sortField for product: " + sortField)
	}

	query := `SELECT uuid, name FROM product WHERE 1=1`
	var args []any
	appendIdentifierFilter(&query, &args, OwnerProduct, "product.uuid", idType, idValue)

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

	out := []tea.Product{}
	for rows.Next() {
		var p tea.Product
		if err := rows.Scan(&p.UUID, &p.Name); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range out {
		ids, err := listIdentifiers(ctx, r.db, OwnerProduct, out[i].UUID)
		if err != nil {
			return nil, err
		}
		out[i].Identifiers = ids
	}
	return out, nil
}

func (r *Repo) DeleteProduct(ctx context.Context, uuid string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `DELETE FROM product WHERE uuid = ?`, uuid)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	if err := deleteOwnerScoped(ctx, tx, OwnerProduct, uuid); err != nil {
		return err
	}
	return tx.Commit()
}

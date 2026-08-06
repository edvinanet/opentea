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

// CreateProduct creates a new product with a fresh generated UUID.
func (r *Repo) CreateProduct(ctx context.Context, name string, identifiers []tea.Identifier) (tea.Product, error) {
	uuid := idgen.New()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return tea.Product{}, err
	}
	defer func() { _ = tx.Rollback() }()

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
// with this UUID already exists and its content matches, it's left
// untouched (created=false) -- import is idempotent, not a merge. If a
// product with this UUID exists with *different* content, returns
// ErrImportIdentityConflict: the UUID is only meaningful within its
// source server, so a same-UUID collision with different content means
// two unrelated products, not the same one re-imported -- see
// ErrImportIdentityConflict's doc comment.
func (r *Repo) ImportProduct(ctx context.Context, uuid, name string, identifiers []tea.Identifier) (created bool, err error) {
	return runInTx(ctx, r, func(tx dbtx) (bool, error) {
		existing, err := getProductTx(ctx, tx, uuid)
		if err == nil {
			if existing.Name != name || !setEqual(existing.Identifiers, identifiers) {
				return false, fmt.Errorf("%w: product %s", ErrImportIdentityConflict, uuid)
			}
			return false, nil
		} else if !errors.Is(err, ErrNotFound) {
			return false, err
		}

		if _, err := tx.ExecContext(ctx, `INSERT INTO product (uuid, name) VALUES (?, ?)`, uuid, name); err != nil {
			return false, err
		}
		if err := insertIdentifiers(ctx, tx, OwnerProduct, uuid, identifiers); err != nil {
			return false, err
		}
		return true, nil
	})
}

// GetProduct fetches a product by UUID. Returns ErrNotFound if uuid doesn't exist.
func (r *Repo) GetProduct(ctx context.Context, uuid string) (tea.Product, error) {
	return getProductTx(ctx, r.conn(), uuid)
}

// getProductTx is GetProduct's logic parameterized over a dbtx instead of
// going through r.conn() -- needed by ImportProduct's conflict check, which
// runs inside a runInTx closure holding a *sql.Tx that isn't necessarily
// r.tx (runInTx opens its own transaction when r.tx is nil). Calling the
// r.conn()-based GetProduct from in there would try to check out a second
// connection from the pool while the closure's own tx already holds the
// only one (internal/db/db.go pins the pool to a single connection) --
// a self-deadlock, not a stale read.
func getProductTx(ctx context.Context, q dbtx, uuid string) (tea.Product, error) {
	var name string
	err := q.QueryRowContext(ctx, `SELECT name FROM product WHERE uuid = ?`, uuid).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return tea.Product{}, ErrNotFound
	}
	if err != nil {
		return tea.Product{}, err
	}

	ids, err := listIdentifiers(ctx, q, OwnerProduct, uuid)
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
	switch sortField {
	case "name", "":
	default:
		return nil, errors.New("repo: unsupported sortField for product: " + sortField)
	}
	sortColumn := "name"

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
	query += pq.orderByClause() + " LIMIT ?" //nolint:gosec // orderByClause's SortColumn always comes from a fixed, pre-validated allowlist (see the switch above), never raw input
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

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

// DeleteProduct deletes the product identified by uuid, cascading to its
// releases. Returns ErrNotFound if uuid doesn't exist.
func (r *Repo) DeleteProduct(ctx context.Context, uuid string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

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

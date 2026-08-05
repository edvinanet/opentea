package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/oej/opentea/internal/idgen"
	"github.com/oej/opentea/internal/pagination"
	"github.com/oej/opentea/pkg/tea"
)

// ProductReleaseInput carries the fields needed to create a new product release.
type ProductReleaseInput struct {
	Version     string
	CreatedDate time.Time
	ReleaseDate *time.Time
	PreRelease  bool
	Identifiers []tea.Identifier
}

// CreateProductRelease creates a new release of productUUID with a fresh
// generated UUID. Returns ErrNotFound if productUUID doesn't exist.
func (r *Repo) CreateProductRelease(ctx context.Context, productUUID string, in ProductReleaseInput) (tea.ProductRelease, error) {
	var productName string
	if err := r.db.QueryRowContext(ctx, `SELECT name FROM product WHERE uuid = ?`, productUUID).Scan(&productName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tea.ProductRelease{}, ErrNotFound
		}
		return tea.ProductRelease{}, err
	}

	uuid := idgen.New()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return tea.ProductRelease{}, err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO product_release (uuid, product_uuid, product_name, version, created_date, release_date, pre_release) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid, productUUID, productName, in.Version, formatTime(in.CreatedDate), formatTimePtr(in.ReleaseDate), boolToInt(in.PreRelease),
	)
	if err != nil {
		return tea.ProductRelease{}, err
	}
	if err := insertIdentifiers(ctx, tx, OwnerProductRelease, uuid, in.Identifiers); err != nil {
		return tea.ProductRelease{}, err
	}
	if err := tx.Commit(); err != nil {
		return tea.ProductRelease{}, err
	}

	return r.GetProductRelease(ctx, uuid)
}

// ImportProductReleaseInput mirrors ProductReleaseInput but carries an
// explicit UUID (preserving the source's identity, required for import) and
// the source's denormalized ProductName (rather than re-deriving it from the
// possibly since-renamed local product row).
type ImportProductReleaseInput struct {
	UUID        string
	ProductUUID string
	ProductName string
	Version     string
	CreatedDate time.Time
	ReleaseDate *time.Time
	PreRelease  bool
	Identifiers []tea.Identifier
}

// ImportProductRelease creates the release with an explicit UUID if it
// doesn't already exist; a second import of the same UUID is a no-op
// (created=false), not a merge. Component links are handled separately via
// the existing idempotent LinkComponent.
func (r *Repo) ImportProductRelease(ctx context.Context, in ImportProductReleaseInput) (created bool, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM product_release WHERE uuid = ?`, in.UUID).Scan(&exists); err == nil {
		return false, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO product_release (uuid, product_uuid, product_name, version, created_date, release_date, pre_release) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		in.UUID, in.ProductUUID, in.ProductName, in.Version, formatTime(in.CreatedDate), formatTimePtr(in.ReleaseDate), boolToInt(in.PreRelease),
	)
	if err != nil {
		return false, err
	}
	if err := insertIdentifiers(ctx, tx, OwnerProductRelease, in.UUID, in.Identifiers); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// GetProductRelease fetches a product release by UUID, including its
// identifiers and linked components. Returns ErrNotFound if uuid doesn't exist.
func (r *Repo) GetProductRelease(ctx context.Context, uuid string) (tea.ProductRelease, error) {
	var (
		productUUID sql.NullString
		productName sql.NullString
		version     string
		createdDate string
		releaseDate sql.NullString
		preRelease  int
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT product_uuid, product_name, version, created_date, release_date, pre_release FROM product_release WHERE uuid = ?`, uuid,
	).Scan(&productUUID, &productName, &version, &createdDate, &releaseDate, &preRelease)
	if errors.Is(err, sql.ErrNoRows) {
		return tea.ProductRelease{}, ErrNotFound
	}
	if err != nil {
		return tea.ProductRelease{}, err
	}

	created, err := parseTime(createdDate)
	if err != nil {
		return tea.ProductRelease{}, err
	}
	released, err := parseNullTime(releaseDate)
	if err != nil {
		return tea.ProductRelease{}, err
	}

	ids, err := listIdentifiers(ctx, r.db, OwnerProductRelease, uuid)
	if err != nil {
		return tea.ProductRelease{}, err
	}
	components, err := listProductReleaseComponents(ctx, r.db, uuid)
	if err != nil {
		return tea.ProductRelease{}, err
	}

	pre := preRelease != 0
	pr := tea.ProductRelease{
		UUID:        uuid,
		Version:     version,
		CreatedDate: created,
		ReleaseDate: released,
		PreRelease:  &pre,
		Identifiers: ids,
		Components:  components,
	}
	if productUUID.Valid {
		pr.Product = &productUUID.String
	}
	if productName.Valid {
		pr.ProductName = productName.String
	}
	return pr, nil
}

func listProductReleaseComponents(ctx context.Context, q dbtx, productReleaseUUID string) ([]tea.ComponentRef, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT component_uuid, component_release_uuid FROM product_release_component WHERE product_release_uuid = ?`, productReleaseUUID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []tea.ComponentRef{}
	for rows.Next() {
		var ref tea.ComponentRef
		var releaseUUID sql.NullString
		if err := rows.Scan(&ref.UUID, &releaseUUID); err != nil {
			return nil, err
		}
		if releaseUUID.Valid {
			ref.Release = &releaseUUID.String
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

// LinkComponent adds (or replaces the pin for) a component reference on a
// product release -- the admin-API stand-in for productRelease.components[].
func (r *Repo) LinkComponent(ctx context.Context, productReleaseUUID string, ref tea.ComponentRef) (tea.ProductRelease, error) {
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO product_release_component (product_release_uuid, component_uuid, component_release_uuid) VALUES (?, ?, ?)
		 ON CONFLICT (product_release_uuid, component_uuid) DO UPDATE SET component_release_uuid = excluded.component_release_uuid`,
		productReleaseUUID, ref.UUID, ref.Release,
	); err != nil {
		return tea.ProductRelease{}, err
	}
	return r.GetProductRelease(ctx, productReleaseUUID)
}

func productReleaseSortColumn(sortField string) (string, error) {
	switch sortField {
	case "createdDate", "":
		return "created_date", nil
	case "releaseDate":
		return "release_date", nil
	case "version":
		return "version", nil
	default:
		return "", errors.New("repo: unsupported sortField for productRelease: " + sortField)
	}
}

// ListProductReleasesByProduct returns up to limit releases of productUUID.
func (r *Repo) ListProductReleasesByProduct(ctx context.Context, productUUID, sortField, sortOrder string, cursor *pagination.Cursor, limit int) ([]tea.ProductRelease, error) {
	sortColumn, err := productReleaseSortColumn(sortField)
	if err != nil {
		return nil, err
	}
	query := `SELECT uuid FROM product_release WHERE product_uuid = ?`
	args := []any{productUUID}
	return r.queryProductReleaseUUIDs(ctx, query, args, sortColumn, sortOrder, cursor, limit)
}

// QueryProductReleases returns up to limit releases across all products,
// filtered by idType/idValue.
func (r *Repo) QueryProductReleases(ctx context.Context, idType, idValue, sortField, sortOrder string, cursor *pagination.Cursor, limit int) ([]tea.ProductRelease, error) {
	sortColumn, err := productReleaseSortColumn(sortField)
	if err != nil {
		return nil, err
	}
	query := `SELECT uuid FROM product_release WHERE 1=1`
	var args []any
	appendIdentifierFilter(&query, &args, OwnerProductRelease, "product_release.uuid", idType, idValue)
	return r.queryProductReleaseUUIDs(ctx, query, args, sortColumn, sortOrder, cursor, limit)
}

func (r *Repo) queryProductReleaseUUIDs(ctx context.Context, query string, args []any, sortColumn, sortOrder string, cursor *pagination.Cursor, limit int) ([]tea.ProductRelease, error) {
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
	var uuids []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			_ = rows.Close()
			return nil, err
		}
		uuids = append(uuids, u)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	out := make([]tea.ProductRelease, 0, len(uuids))
	for _, u := range uuids {
		pr, err := r.GetProductRelease(ctx, u)
		if err != nil {
			return nil, err
		}
		out = append(out, pr)
	}
	return out, nil
}

// DeleteProductRelease deletes the release identified by uuid, cascading to
// its component links and collections. Returns ErrNotFound if uuid doesn't exist.
func (r *Repo) DeleteProductRelease(ctx context.Context, uuid string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `DELETE FROM product_release WHERE uuid = ?`, uuid)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	if err := deleteOwnerScoped(ctx, tx, OwnerProductRelease, uuid); err != nil {
		return err
	}
	return tx.Commit()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

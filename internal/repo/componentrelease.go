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

// ComponentReleaseInput carries the fields needed to create a new component release.
type ComponentReleaseInput struct {
	Version     string
	CreatedDate time.Time
	ReleaseDate *time.Time
	PreRelease  bool
	Identifiers []tea.Identifier
}

// CreateComponentRelease creates a new release of componentUUID with a
// fresh generated UUID. Returns ErrNotFound if componentUUID doesn't exist.
func (r *Repo) CreateComponentRelease(ctx context.Context, componentUUID string, in ComponentReleaseInput) (tea.ComponentRelease, error) {
	var componentName string
	if err := r.db.QueryRowContext(ctx, `SELECT name FROM component WHERE uuid = ?`, componentUUID).Scan(&componentName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tea.ComponentRelease{}, ErrNotFound
		}
		return tea.ComponentRelease{}, err
	}

	uuid := idgen.New()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return tea.ComponentRelease{}, err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO component_release (uuid, component_uuid, component_name, version, created_date, release_date, pre_release) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid, componentUUID, componentName, in.Version, formatTime(in.CreatedDate), formatTimePtr(in.ReleaseDate), boolToInt(in.PreRelease),
	)
	if err != nil {
		return tea.ComponentRelease{}, err
	}
	if err := insertIdentifiers(ctx, tx, OwnerComponentRelease, uuid, in.Identifiers); err != nil {
		return tea.ComponentRelease{}, err
	}
	if err := tx.Commit(); err != nil {
		return tea.ComponentRelease{}, err
	}

	return r.GetComponentRelease(ctx, uuid)
}

// ImportComponentReleaseInput mirrors ImportProductReleaseInput -- see there
// for the identity/idempotency rationale.
type ImportComponentReleaseInput struct {
	UUID          string
	ComponentUUID string
	ComponentName string
	Version       string
	CreatedDate   time.Time
	ReleaseDate   *time.Time
	PreRelease    bool
	Identifiers   []tea.Identifier
}

// ImportComponentRelease creates a component release preserving its source
// identity (in.UUID), or leaves an existing one with that UUID untouched --
// see ImportComponentReleaseInput and internal/bundle for the idempotency
// rationale.
func (r *Repo) ImportComponentRelease(ctx context.Context, in ImportComponentReleaseInput) (created bool, err error) {
	return runInTx(ctx, r, func(tx dbtx) (bool, error) {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM component_release WHERE uuid = ?`, in.UUID).Scan(&exists); err == nil {
			return false, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}

		_, err := tx.ExecContext(ctx,
			`INSERT INTO component_release (uuid, component_uuid, component_name, version, created_date, release_date, pre_release) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			in.UUID, in.ComponentUUID, in.ComponentName, in.Version, formatTime(in.CreatedDate), formatTimePtr(in.ReleaseDate), boolToInt(in.PreRelease),
		)
		if err != nil {
			return false, err
		}
		if err := insertIdentifiers(ctx, tx, OwnerComponentRelease, in.UUID, in.Identifiers); err != nil {
			return false, err
		}
		return true, nil
	})
}

// GetComponentRelease fetches a component release by UUID, including its
// identifiers and distributions. Returns ErrNotFound if uuid doesn't exist.
func (r *Repo) GetComponentRelease(ctx context.Context, uuid string) (tea.ComponentRelease, error) {
	var (
		componentUUID sql.NullString
		componentName sql.NullString
		version       string
		createdDate   string
		releaseDate   sql.NullString
		preRelease    int
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT component_uuid, component_name, version, created_date, release_date, pre_release FROM component_release WHERE uuid = ?`, uuid,
	).Scan(&componentUUID, &componentName, &version, &createdDate, &releaseDate, &preRelease)
	if errors.Is(err, sql.ErrNoRows) {
		return tea.ComponentRelease{}, ErrNotFound
	}
	if err != nil {
		return tea.ComponentRelease{}, err
	}

	created, err := parseTime(createdDate)
	if err != nil {
		return tea.ComponentRelease{}, err
	}
	released, err := parseNullTime(releaseDate)
	if err != nil {
		return tea.ComponentRelease{}, err
	}

	ids, err := listIdentifiers(ctx, r.db, OwnerComponentRelease, uuid)
	if err != nil {
		return tea.ComponentRelease{}, err
	}
	distributions, err := listDistributionsForRelease(ctx, r.db, uuid)
	if err != nil {
		return tea.ComponentRelease{}, err
	}

	pre := preRelease != 0
	cr := tea.ComponentRelease{
		UUID:          uuid,
		ComponentName: componentName.String,
		Version:       version,
		CreatedDate:   created,
		ReleaseDate:   released,
		PreRelease:    &pre,
		Identifiers:   ids,
		Distributions: distributions,
	}
	if componentUUID.Valid {
		cr.Component = componentUUID.String
	}
	return cr, nil
}

func componentReleaseSortColumn(sortField string) (string, error) {
	switch sortField {
	case "createdDate", "":
		return "created_date", nil
	case "releaseDate":
		return "release_date", nil
	case "version":
		return "version", nil
	default:
		return "", errors.New("repo: unsupported sortField for componentRelease: " + sortField)
	}
}

// ListComponentReleasesByComponent returns up to limit releases of componentUUID.
func (r *Repo) ListComponentReleasesByComponent(ctx context.Context, componentUUID, sortField, sortOrder string, cursor *pagination.Cursor, limit int) ([]tea.ComponentRelease, error) {
	sortColumn, err := componentReleaseSortColumn(sortField)
	if err != nil {
		return nil, err
	}
	query := `SELECT uuid FROM component_release WHERE component_uuid = ?`
	args := []any{componentUUID}
	return r.queryComponentReleaseUUIDs(ctx, query, args, sortColumn, sortOrder, cursor, limit)
}

// QueryComponentReleases returns up to limit releases across all
// components, filtered by idType/idValue.
func (r *Repo) QueryComponentReleases(ctx context.Context, idType, idValue, sortField, sortOrder string, cursor *pagination.Cursor, limit int) ([]tea.ComponentRelease, error) {
	sortColumn, err := componentReleaseSortColumn(sortField)
	if err != nil {
		return nil, err
	}
	query := `SELECT uuid FROM component_release WHERE 1=1`
	var args []any
	appendIdentifierFilter(&query, &args, OwnerComponentRelease, "component_release.uuid", idType, idValue)
	return r.queryComponentReleaseUUIDs(ctx, query, args, sortColumn, sortOrder, cursor, limit)
}

func (r *Repo) queryComponentReleaseUUIDs(ctx context.Context, query string, args []any, sortColumn, sortOrder string, cursor *pagination.Cursor, limit int) ([]tea.ComponentRelease, error) {
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

	out := make([]tea.ComponentRelease, 0, len(uuids))
	for _, u := range uuids {
		cr, err := r.GetComponentRelease(ctx, u)
		if err != nil {
			return nil, err
		}
		out = append(out, cr)
	}
	return out, nil
}

// DeleteComponentRelease deletes the release identified by uuid, cascading
// to its distributions and collections. Returns ErrNotFound if uuid doesn't exist.
func (r *Repo) DeleteComponentRelease(ctx context.Context, uuid string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `DELETE FROM component_release WHERE uuid = ?`, uuid)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	if err := deleteOwnerScoped(ctx, tx, OwnerComponentRelease, uuid); err != nil {
		return err
	}
	return tx.Commit()
}

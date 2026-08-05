// Package repo is the SQLite data-access layer. Each aggregate (product,
// component, productrelease, componentrelease, distribution, artifact,
// collection, cle) has its own file with Create/Get/Query/Delete methods on
// *Repo returning internal/model structs directly -- there is no separate
// DTO layer since there's no business logic beyond CRUD + pagination.
package repo

import (
	"context"
	"database/sql"
)

// Repo wraps the SQLite connection pool.
type Repo struct {
	db *sql.DB
}

// New wraps db as a Repo.
func New(db *sql.DB) *Repo {
	return &Repo{db: db}
}

// dbtx is satisfied by both *sql.DB and *sql.Tx, letting shared helpers run
// either standalone or inside a caller's transaction.
type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Owner type discriminators used by the polymorphic identifier, checksum,
// and CLE tables.
const (
	OwnerProduct          = "PRODUCT"
	OwnerProductRelease   = "PRODUCT_RELEASE"
	OwnerComponent        = "COMPONENT"
	OwnerComponentRelease = "COMPONENT_RELEASE"
	OwnerDistribution     = "DISTRIBUTION"
	OwnerArtifactFormat   = "ARTIFACT_FORMAT"
)

// deleteOwnerScoped removes the polymorphic identifier and CLE rows owned by
// (ownerType, ownerUUID). Structural children (releases, distributions,
// etc.) are cleaned up by the schema's ON DELETE CASCADE foreign keys;
// identifier/CLE rows are not FK-enforced (they're polymorphic across owner
// types) so they need explicit cleanup here.
func deleteOwnerScoped(ctx context.Context, q dbtx, ownerType, ownerUUID string) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM identifier WHERE owner_type = ? AND owner_uuid = ?`, ownerType, ownerUUID); err != nil {
		return err
	}
	if _, err := q.ExecContext(ctx, `DELETE FROM cle_event WHERE owner_type = ? AND owner_uuid = ?`, ownerType, ownerUUID); err != nil {
		return err
	}
	if _, err := q.ExecContext(ctx, `DELETE FROM cle_support_definition WHERE owner_type = ? AND owner_uuid = ?`, ownerType, ownerUUID); err != nil {
		return err
	}
	return nil
}

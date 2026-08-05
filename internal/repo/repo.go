// Package repo is the SQLite data-access layer. Each aggregate (product,
// component, productrelease, componentrelease, distribution, artifact,
// collection, cle) has its own file with Create/Get/Query/Delete methods on
// *Repo returning internal/model structs directly -- there is no separate
// DTO layer since there's no business logic beyond CRUD + pagination.
package repo

import (
	"context"
	"database/sql"
	"errors"
)

// Repo wraps the SQLite connection pool. tx is non-nil only for a Repo
// handed to a WithTx callback, scoping every call made through it to that
// one transaction -- see WithTx.
type Repo struct {
	db *sql.DB
	tx *sql.Tx
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

// conn returns the connection this Repo should use for a single-statement
// operation that doesn't need its own transaction (e.g. an upsert): the
// active transaction if this Repo is scoped inside WithTx, otherwise the
// raw pool.
func (r *Repo) conn() dbtx {
	if r.tx != nil {
		return r.tx
	}
	return r.db
}

// WithTx runs fn against a Repo scoped to a single transaction spanning
// every call fn makes through it -- committed if fn returns nil, rolled
// back otherwise (including on panic, which is re-panicked after
// rolling back). Every Import* method (and LinkComponent, UpsertBlob)
// participates in that one transaction when called through the Repo fn
// receives, instead of each opening/committing its own -- use this when a
// caller needs several such calls to succeed or fail as a unit (see
// internal/bundle.Import).
//
// Not reentrant: fn must not call WithTx again on the Repo it receives --
// database/sql doesn't support nested transactions, and this project has
// no need for savepoints.
func (r *Repo) WithTx(ctx context.Context, fn func(*Repo) error) (err error) {
	if r.tx != nil {
		return errors.New("repo: WithTx called on a Repo that's already inside a transaction")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(&Repo{db: r.db, tx: tx}); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// runInTx runs fn against a transaction, returning whatever fn returns:
// reuses r.tx if this Repo is already scoped inside WithTx (so the call
// composes into that one outer transaction rather than starting its own),
// otherwise begins and manages a new transaction just for fn.
func runInTx[T any](ctx context.Context, r *Repo, fn func(dbtx) (T, error)) (T, error) {
	if r.tx != nil {
		return fn(r.tx)
	}
	var zero T
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return zero, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := fn(tx)
	if err != nil {
		return zero, err
	}
	if err := tx.Commit(); err != nil {
		return zero, err
	}
	return result, nil
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

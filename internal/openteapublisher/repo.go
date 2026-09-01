// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Package openteapublisher implements opentea-publisher: the GUI publisher
// platform (design/publisher-service.md §4, §17) -- a separate service
// (own binary cmd/openteapublisher, own database, own deployment) that
// signs locally (internal/trust) and calls a target TEA server's
// /publisher/v1 as a credentialed client (pkg/teapublisherclient), never
// the other way around. Its source lives in this repository (v0.21) for
// code reuse and integrated testing, not because it's part of opentea
// itself -- see §3's non-goal.
//
// One package, not opentea's repo/admin/webadmin three-way split --
// appropriately scoped for this app's current size. File names carry the
// separation instead: staff.go/session.go/target.go hold the data layer
// (this file's Repo type, its own *sql.DB via internal/db.OpenWithMigrations),
// everything else the HTTP/GUI layer. Revisit the split if/when the
// internal multi-team business-approval workflow (§17.3, not designed yet)
// adds real complexity.
package openteapublisher

import (
	"context"
	"database/sql"
	"embed"
	"errors"

	"github.com/oej/opentea/internal/db"
)

//go:embed db/migrations/*.sql
var migrationsFS embed.FS

// Open opens (creating if needed) opentea-publisher's own SQLite database
// at path and applies its migrations -- entirely separate from opentea's
// own (internal/db.Open), via the shared runner (internal/db.OpenWithMigrations).
func Open(path string) (*sql.DB, error) {
	return db.OpenWithMigrations(path, migrationsFS, "db/migrations")
}

// Repo wraps the SQLite connection pool for opentea-publisher's own
// database (staff accounts, sessions, target credentials -- never
// opentea's own tables, a genuinely separate database). tx is non-nil only
// for a Repo handed to a WithTx callback, scoping every call made through
// it to that one transaction -- see WithTx. Mirrors internal/repo.Repo's
// own shape exactly (same reasoning: an audit record must commit
// atomically with the mutation it describes, §18.12).
type Repo struct {
	db *sql.DB
	tx *sql.Tx
}

// New wraps sqlDB (from Open) as a Repo.
func New(sqlDB *sql.DB) *Repo {
	return &Repo{db: sqlDB}
}

// dbtx is satisfied by both *sql.DB and *sql.Tx, letting shared code run
// either standalone or inside a caller's transaction.
type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// conn returns the connection this Repo should use for a single-statement
// operation: the active transaction if this Repo is scoped inside WithTx,
// otherwise the raw pool.
func (r *Repo) conn() dbtx {
	if r.tx != nil {
		return r.tx
	}
	return r.db
}

// WithTx runs fn against a Repo scoped to a single transaction spanning
// every call fn makes through it -- committed if fn returns nil, rolled
// back otherwise (including on panic, which is re-panicked after rolling
// back). Use this whenever a mutation must commit atomically with its own
// audit record (RecordAudit) -- see targets.go for the call sites.
//
// Not reentrant: fn must not call WithTx again on the Repo it receives.
func (r *Repo) WithTx(ctx context.Context, fn func(*Repo) error) (err error) {
	if r.tx != nil {
		return errors.New("openteapublisher: WithTx called on a Repo that's already inside a transaction")
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

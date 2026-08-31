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
	"database/sql"
	"embed"

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
// opentea's own tables, a genuinely separate database).
type Repo struct {
	db *sql.DB
}

// New wraps sqlDB (from Open) as a Repo.
func New(sqlDB *sql.DB) *Repo {
	return &Repo{db: sqlDB}
}

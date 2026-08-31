// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Package db opens the SQLite database and applies migrations.
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Open opens (creating if needed) opentea's own SQLite database at path,
// applies its migrations (this package's own embedded migrations/*.sql),
// and returns a connection pool pinned to a single connection (SQLite only
// supports one writer; this also makes the CLE per-owner id-assignment
// race-free without extra application locking). A thin wrapper over
// OpenWithMigrations for opentea's own callers.
func Open(path string) (*sql.DB, error) {
	return OpenWithMigrations(path, migrationsFS, "migrations")
}

// OpenWithMigrations is Open generalized to a caller-supplied migrations
// source -- for a second, independently-migrated database (e.g.
// internal/openteapublisher's own) that needs the same open/migrate
// mechanics through this one runner instead of a duplicate. migrations is
// typically a package-level `//go:embed <dir>/*.sql` var in the caller's
// own package (embed directives only see files under the declaring
// package's own directory tree, so each database's migrations must be
// embedded where they live); dir is the directory within migrations that
// holds the *.sql files, e.g. "migrations".
func OpenWithMigrations(path string, migrations fs.FS, dir string) (*sql.DB, error) {
	if d := filepath.Dir(path); d != "." {
		if err := os.MkdirAll(d, 0o750); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	dsn := "file:" + path + "?" + url.Values{
		"_pragma": []string{
			"foreign_keys(1)",
			"busy_timeout(5000)",
			"journal_mode(WAL)",
		},
	}.Encode()

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)

	if err := migrate(sqlDB, migrations, dir); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return sqlDB, nil
}

func migrate(sqlDB *sql.DB, migrations fs.FS, dir string) error {
	if _, err := sqlDB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')))`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrations, dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		var already int
		if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, name).Scan(&already); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if already > 0 {
			continue
		}

		contents, err := fs.ReadFile(migrations, dir+"/"+name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if err := applyMigration(sqlDB, name, string(contents)); err != nil {
			return err
		}
	}
	return nil
}

// applyMigration runs contents and records name as applied in one
// transaction, so a crash between the two can't leave the schema change
// committed but unrecorded (which would otherwise make the next startup
// retry a non-idempotent CREATE TABLE and fail). SQLite's DDL is
// transactional, so this is safe even though contents typically contains
// CREATE TABLE statements.
func applyMigration(sqlDB *sql.DB, name, contents string) error {
	tx, err := sqlDB.Begin()
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", name, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(contents); err != nil {
		return fmt.Errorf("apply migration %s: %w", name, err)
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations (name) VALUES (?)`, name); err != nil {
		return fmt.Errorf("record migration %s: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", name, err)
	}
	return nil
}

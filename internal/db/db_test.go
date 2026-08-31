// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package db

import (
	"embed"
	"path/filepath"
	"testing"
)

//go:embed testdata/otherdb/*.sql
var otherMigrationsFS embed.FS

// TestOpenWithMigrationsIndependentDatabase proves OpenWithMigrations
// serves a second, independently-migrated database through a caller-
// supplied embed.FS -- the prerequisite internal/openteapublisher's own
// database needs, so Open's own embedded migrations/*.sql don't leak into
// (or get required by) an unrelated database.
func TestOpenWithMigrationsIndependentDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "other.db")

	sqlDB, err := OpenWithMigrations(path, otherMigrationsFS, "testdata/otherdb")
	if err != nil {
		t.Fatalf("OpenWithMigrations: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	var widgetCount int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='widget'`).Scan(&widgetCount); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if widgetCount != 1 {
		t.Fatalf("expected widget table to exist, got count=%d", widgetCount)
	}
	// Confirms this database is genuinely independent of opentea's own --
	// it must NOT have opentea's product table (or any of its migrations).
	var productCount int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='product'`).Scan(&productCount); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if productCount != 0 {
		t.Fatalf("expected no product table in the independent database, got count=%d", productCount)
	}
}

func TestOpenAppliesMigrationsIdempotently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	sqlDB, err := Open(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	var tableCount int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='product'`).Scan(&tableCount); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if tableCount != 1 {
		t.Fatalf("expected product table to exist, got count=%d", tableCount)
	}
	_ = sqlDB.Close()

	// Re-open against the same file: migrations must be idempotent.
	sqlDB2, err := Open(path)
	if err != nil {
		t.Fatalf("second open (idempotency): %v", err)
	}
	defer func() { _ = sqlDB2.Close() }()
}

// TestApplyMigrationRollsBackOnFailure is the regression test for the
// finding that a migration's schema change and its schema_migrations
// bookkeeping row weren't one transaction: a crash (or, as exercised here,
// any failure) between the two could leave the schema applied but
// unrecorded, so the next startup would retry a non-idempotent CREATE TABLE
// and fail. A migration whose second statement fails must leave neither the
// first statement's effect nor a schema_migrations row behind.
func TestApplyMigrationRollsBackOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	sqlDB, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	const badMigration = `
		CREATE TABLE test_atomic_migration (id INTEGER PRIMARY KEY);
		THIS IS NOT VALID SQL;
	`
	if err := applyMigration(sqlDB, "9999_bad.sql", badMigration); err == nil {
		t.Fatal("expected applyMigration to fail on invalid SQL")
	}

	var tableCount int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='test_atomic_migration'`).Scan(&tableCount); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if tableCount != 0 {
		t.Fatal("expected the failed migration's CREATE TABLE to have been rolled back, not left applied")
	}

	var recorded int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, "9999_bad.sql").Scan(&recorded); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if recorded != 0 {
		t.Fatal("expected the failed migration to not be recorded as applied")
	}
}

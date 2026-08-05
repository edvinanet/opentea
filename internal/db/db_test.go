package db

import (
	"path/filepath"
	"testing"
)

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

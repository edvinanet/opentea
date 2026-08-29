// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oej/opentea/internal/bundle"
	"github.com/oej/opentea/internal/db"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/internal/storage"
)

// writeValidBundle builds a minimal real product via the repo and exports
// it to a temp zip file, returning the path.
func writeValidBundle(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()

	sqlDB, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	r := repo.New(sqlDB)

	store, err := storage.NewFSStorage(filepath.Join(dir, "blobs"))
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}

	product, err := r.CreateProduct(ctx, "Test Product", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if _, err := r.CreateProductRelease(ctx, product.UUID, repo.ProductReleaseInput{
		Version: "1.0.0", CreatedDate: time.Now().UTC().Truncate(time.Second),
	}); err != nil {
		t.Fatalf("CreateProductRelease: %v", err)
	}

	zipPath := filepath.Join(dir, "product.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip file: %v", err)
	}
	defer func() { _ = f.Close() }()

	if err := bundle.Export(ctx, r, store, product.UUID, f); err != nil {
		t.Fatalf("Export: %v", err)
	}
	return zipPath
}

func TestRunValidBundle(t *testing.T) {
	path := writeValidBundle(t)
	if code := run([]string{path}); code != 0 {
		t.Fatalf("run() = %d, want 0 for a valid bundle", code)
	}
}

func TestRunMissingFile(t *testing.T) {
	if code := run([]string{filepath.Join(t.TempDir(), "does-not-exist.zip")}); code != 1 {
		t.Fatalf("run() = %d, want 1 for a nonexistent file", code)
	}
}

func TestRunInvalidBundle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-a-zip.zip")
	if err := os.WriteFile(path, []byte("this is not a zip file"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if code := run([]string{path}); code != 1 {
		t.Fatalf("run() = %d, want 1 for a non-zip file", code)
	}
}

func TestRunNoArgs(t *testing.T) {
	if code := run(nil); code != 1 {
		t.Fatalf("run() = %d, want 1 with no arguments", code)
	}
}

// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFSStoragePutOpenDelete(t *testing.T) {
	s, err := NewFSStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	ctx := context.Background()

	content := []byte("hello, tea")
	sum, size, err := s.Put(ctx, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if size != int64(len(content)) {
		t.Fatalf("size = %d, want %d", size, len(content))
	}
	const wantSum = "d2dcfdfce3296e56b200d5a342c5dfa30dcc9e1ef8eacfd3cd1af5bca77bb586"
	if sum != wantSum {
		t.Fatalf("sha256 = %s, want %s", sum, wantSum)
	}

	rc, err := s.Open(ctx, sum)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("got %q, want %q", got, content)
	}

	if err := s.Delete(ctx, sum); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Open(ctx, sum); err == nil {
		t.Fatal("expected error opening deleted blob")
	}

	// Deleting a non-existent blob is not an error.
	if err := s.Delete(ctx, sum); err != nil {
		t.Fatalf("Delete (already gone): %v", err)
	}
}

// TestOpenAndDeleteRejectPathTraversal is the regression test for a
// security-review finding: Open/Delete built their on-disk path directly
// from the caller-supplied sha256 with no format check. internal/bundle's
// Export reads SHA-256 checksum values straight out of the DB, and those
// can originate from a bundle import's manifest -- attacker-controlled if
// importing from an untrusted TEA server, and never validated to actually
// correspond to an uploaded blob. A crafted checksum value shaped like a
// path-traversal payload must be rejected here, not passed through to
// filepath.Join/os.Open.
func TestOpenAndDeleteRejectPathTraversal(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFSStorage(dir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	ctx := context.Background()

	// A real file outside the blob directory that a traversal payload
	// would target if it weren't rejected.
	secret := filepath.Join(filepath.Dir(dir), "secret-outside-blob-dir.txt")
	if err := os.WriteFile(secret, []byte("should never be readable via Open"), 0o600); err != nil {
		t.Fatalf("write secret file: %v", err)
	}
	defer func() { _ = os.Remove(secret) }()

	payloads := []string{
		"../secret-outside-blob-dir.txt",
		"../../../../etc/passwd",
		"not-hex-and-not-64-chars",
		strings.Repeat("a", 63), // one char short of a real sha256
	}
	for _, p := range payloads {
		t.Run(p, func(t *testing.T) {
			if _, err := s.Open(ctx, p); err == nil {
				t.Errorf("Open(%q): expected an error, got none", p)
			}
			if err := s.Delete(ctx, p); err == nil {
				t.Errorf("Delete(%q): expected an error, got none", p)
			}
			if _, err := s.PutUnchecked(ctx, p, strings.NewReader("x")); err == nil {
				t.Errorf("PutUnchecked(%q): expected an error, got none", p)
			}
		})
	}
}

// TestPutUncheckedStoresUnderClaimedHashRegardlessOfContent is
// PutUnchecked's whole reason to exist: internal/bundle's force-import
// path needs to store content under a caller-claimed sha256 even when
// that claim is wrong, to deliberately create the exact corruption
// docs/consumer-api-conformance-test-rig.md's client-side suite proves it
// detects. Put, by contrast, must never allow this -- it always computes
// and returns the real hash, confirmed separately by
// TestFSStoragePutOpenDelete.
func TestPutUncheckedStoresUnderClaimedHashRegardlessOfContent(t *testing.T) {
	s, err := NewFSStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	ctx := context.Background()

	content := []byte("this is not the content that hashes to the claimed name below")
	const claimedSHA256 = "d2dcfdfce3296e56b200d5a342c5dfa30dcc9e1ef8eacfd3cd1af5bca77bb586" // the real sha256 of "hello, tea", not of content above

	size, err := s.PutUnchecked(ctx, claimedSHA256, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PutUnchecked: %v", err)
	}
	if size != int64(len(content)) {
		t.Fatalf("size = %d, want %d", size, len(content))
	}

	rc, err := s.Open(ctx, claimedSHA256)
	if err != nil {
		t.Fatalf("Open(%s): %v", claimedSHA256, err)
	}
	got, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("got %q, want %q -- PutUnchecked must store exactly what it was given, not what it was told", got, content)
	}
}

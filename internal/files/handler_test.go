// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package files

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oej/opentea/internal/db"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/internal/storage"
)

func newTestHandler(t *testing.T) (http.Handler, *repo.Repo, storage.Storage) {
	t.Helper()
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	r := repo.New(sqlDB)

	store, err := storage.NewFSStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	return NewHandler(r, store), r, store
}

func TestServeUnknownBlob(t *testing.T) {
	h, _, _ := newTestHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/files/"+strings.Repeat("a", 64), nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// TestServeRejectsMalformedSHA256 is the regression test for the finding
// that the sha256 path parameter was used to build a filesystem path (and,
// once Content-Disposition was added below, an HTTP header value) with no
// format check -- not exploitable in practice only because of incidental
// net/http path-cleaning behavior, not anything this package asserted
// itself. Anything that isn't exactly 64 lowercase hex characters must be
// rejected before it reaches either the storage layer or a response header.
func TestServeRejectsMalformedSHA256(t *testing.T) {
	h, _, _ := newTestHandler(t)

	cases := []string{
		"",
		"not-hex-at-all-and-way-too-short",
		strings.Repeat("A", 64),        // uppercase, not the canonical lowercase form
		strings.Repeat("a", 63),        // one short
		strings.Repeat("a", 65),        // one long
		strings.Repeat("a", 62) + `"a`, // header-injection-shaped: an embedded quote
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/files/"+c, nil)
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Errorf("sha256=%q: status = %d, want 404", c, rec.Code)
			}
		})
	}
}

// putOwnedBlob stores content and links it to a fresh artifact's first
// format, mirroring the real create-artifact-then-upload flow -- needed so
// authorized (added for the "/files/{sha256} bypasses authz" finding) has
// an owning artifact to check the default, migration-seeded permissive
// entitlement against. Without this, every blob in this package's tests
// would be an orphan and denied by default regardless of what's being
// tested.
func putOwnedBlob(t *testing.T, ctx context.Context, r *repo.Repo, store storage.Storage, content, mediaType string) string {
	t.Helper()
	sha256Hex, size, err := store.Put(ctx, strings.NewReader(content))
	if err != nil {
		t.Fatalf("store.Put: %v", err)
	}
	if err := r.UpsertBlob(ctx, sha256Hex, size, mediaType); err != nil {
		t.Fatalf("UpsertBlob: %v", err)
	}
	artifact, err := r.CreateArtifact(ctx, repo.ArtifactInput{
		Type:    "OTHER",
		Formats: []repo.ArtifactFormatInput{{MediaType: mediaType}},
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	if _, err := r.SetArtifactFormatFile(ctx, artifact.UUID, 1, 0, sha256Hex); err != nil {
		t.Fatalf("SetArtifactFormatFile: %v", err)
	}
	return sha256Hex
}

// TestServeDeniesBlobWithNoOwningArtifact is the regression test for the
// finding that /files/{sha256} served any stored blob to anyone who knew
// its hash, with no authorization check at all -- checksums exposed
// through otherwise-authorized metadata responses made these URLs
// discoverable. A blob that no artifact (or signature) references at all
// has nothing to authorize against and must be denied by default, not
// served.
func TestServeDeniesBlobWithNoOwningArtifact(t *testing.T) {
	ctx := context.Background()
	h, r, store := newTestHandler(t)

	sha256Hex, size, err := store.Put(ctx, strings.NewReader("orphaned content"))
	if err != nil {
		t.Fatalf("store.Put: %v", err)
	}
	if err := r.UpsertBlob(ctx, sha256Hex, size, "text/plain"); err != nil {
		t.Fatalf("UpsertBlob: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/files/"+sha256Hex, nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (anonymous caller, no owning artifact to authorize against)", rec.Code)
	}
}

// TestServeSetsProtectiveHeaders is the regression test for the "stored
// blob Content-Type" finding: even when a blob's recorded media type is
// something a browser would render/execute (text/html here, standing in
// for whatever an upload's Content-Type header or an imported bundle's
// manifest declared), the response must force a download rather than let
// it render in this origin (shared with /admin/ui and /admin/v1's session
// cookie).
func TestServeSetsProtectiveHeaders(t *testing.T) {
	ctx := context.Background()
	h, r, store := newTestHandler(t)

	sha256Hex := putOwnedBlob(t, ctx, r, store, "<script>alert(1)</script>", "text/html")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/files/"+sha256Hex, nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html" {
		t.Errorf("Content-Type = %q, want %q (unchanged -- disposition is what protects us, not rewriting this)", got, "text/html")
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want %q", got, "nosniff")
	}
	disposition := rec.Header().Get("Content-Disposition")
	if !strings.HasPrefix(disposition, "attachment") {
		t.Errorf("Content-Disposition = %q, want it to start with %q", disposition, "attachment")
	}
	if body := rec.Body.String(); body != "<script>alert(1)</script>" {
		t.Errorf("body = %q, want the stored content unchanged (disposition, not content mangling, is the protection)", body)
	}
}

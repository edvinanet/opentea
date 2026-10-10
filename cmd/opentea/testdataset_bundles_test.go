// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/oej/opentea/internal/bundle"
	"github.com/oej/opentea/internal/testdataset"
)

// writeBundleBytes writes m/files to a temp file via testdataset.WriteBundle
// and reads it back, giving an in-memory zip without duplicating that
// package's own zip-construction logic here.
func writeBundleBytes(t *testing.T, name string, m bundle.Manifest, files map[string][]byte) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := testdataset.WriteBundle(path, m, files); err != nil {
		t.Fatalf("WriteBundle: %v", err)
	}
	raw, err := os.ReadFile(path) //nolint:gosec // path is this test's own t.TempDir() file
	if err != nil {
		t.Fatalf("read back %s: %v", path, err)
	}
	return raw
}

// TestTestdatasetGoodBundlesImportIdempotently imports each of the three
// good reference datasets (docs/bundle-import-export-test-rig.md) into a
// real server via the real /admin/v1/products/import HTTP endpoint, and
// confirms a second identical import is a safe no-op -- the same guarantee
// cmd/testbundlegen's regenerate instructions promise.
func TestTestdatasetGoodBundlesImportIdempotently(t *testing.T) {
	simple := testdataset.Simple()
	complexManifest, complexFiles := testdataset.Complex()
	lifecycle, lifecycleFiles := testdataset.Lifecycle()

	datasets := []struct {
		name  string
		m     bundle.Manifest
		files map[string][]byte
	}{
		{"simple.zip", simple, nil},
		{"complex.zip", complexManifest, complexFiles},
		{"lifecycle-compliance.zip", lifecycle, lifecycleFiles},
	}

	for _, ds := range datasets {
		t.Run(ds.name, func(t *testing.T) {
			srv := newTestServer(t)
			zipBytes := writeBundleBytes(t, ds.name, ds.m, ds.files)

			status, raw := uploadBundle(t, srv, "/admin/v1/products/import", zipBytes)
			if status != http.StatusOK {
				t.Fatalf("first import: status=%d body=%s", status, raw)
			}
			var first struct {
				ProductCreated bool
				Created        map[string]int
			}
			decodeInto(t, raw, &first)
			if !first.ProductCreated {
				t.Fatalf("first import: ProductCreated = false, want true, result=%+v", first)
			}

			status, raw = uploadBundle(t, srv, "/admin/v1/products/import", zipBytes)
			if status != http.StatusOK {
				t.Fatalf("second import: status=%d body=%s", status, raw)
			}
			var second struct {
				ProductCreated bool
				Created        map[string]int
				AlreadyExisted map[string]int
			}
			decodeInto(t, raw, &second)
			if second.ProductCreated {
				t.Fatalf("second import: ProductCreated = true, want false (idempotent), result=%+v", second)
			}
			for entity, n := range second.Created {
				if n != 0 {
					t.Fatalf("second import: Created[%q] = %d, want 0 (idempotent), result=%+v", entity, n, second)
				}
			}
		})
	}
}

// TestTestdatasetBadBundlesRejectedOnImport feeds each of the five
// deliberately-invalid bundles (docs/bundle-import-export-test-rig.md's
// "Bad bundles" table) to the real /admin/v1/products/import endpoint with
// no special parameter, confirming each is rejected rather than partially
// applied.
func TestTestdatasetBadBundlesRejectedOnImport(t *testing.T) {
	type bad struct {
		name string
		m    bundle.Manifest
		f    map[string][]byte
	}
	mk := func(name string, m bundle.Manifest, f map[string][]byte) bad { return bad{name, m, f} }

	m1, f1 := testdataset.BadChecksumMismatch()
	m2, f2 := testdataset.BadMissingFile()
	m3, f3 := testdataset.BadFormatVersion()
	m4, f4 := testdataset.BadDanglingComponentRelease()
	m5, f5 := testdataset.BadDanglingComponent()

	bads := []bad{
		mk("bad-checksum-mismatch.zip", m1, f1),
		mk("bad-missing-file.zip", m2, f2),
		mk("bad-format-version.zip", m3, f3),
		mk("bad-dangling-component-release.zip", m4, f4),
		mk("bad-dangling-component.zip", m5, f5),
	}

	for _, b := range bads {
		t.Run(b.name, func(t *testing.T) {
			srv := newTestServer(t)
			zipBytes := writeBundleBytes(t, b.name, b.m, b.f)

			status, raw := uploadBundle(t, srv, "/admin/v1/products/import", zipBytes)
			if status == http.StatusOK {
				t.Fatalf("import of %s succeeded, want rejection; body=%s", b.name, raw)
			}
			var errBody struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(raw, &errBody); err == nil && errBody.Message == "" {
				t.Fatalf("import of %s rejected with status %d but no error message: %s", b.name, status, raw)
			}
		})
	}
}

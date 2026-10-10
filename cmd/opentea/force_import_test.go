// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"net/http"
	"testing"

	"github.com/oej/opentea/internal/config"
	"github.com/oej/opentea/internal/testdataset"
)

// TestForceImportIgnoredWhenFlagUnset proves docs/bundle-format.md's own
// requirement verbatim: "with the flag unset, force=true is rejected the
// same as if it weren't present at all." A bad bundle with ?force=true
// against an ordinary server (AllowUnsafeImport unset, the default) must
// fail exactly like a plain import would -- the query parameter alone
// must never be enough.
func TestForceImportIgnoredWhenFlagUnset(t *testing.T) {
	srv := newTestServer(t)
	m, files := testdataset.BadChecksumMismatch()
	zipBytes := writeBundleBytes(t, "bad-checksum-mismatch.zip", m, files)

	status, raw := uploadBundle(t, srv, "/admin/v1/products/import?force=true", zipBytes)
	if status == http.StatusOK {
		t.Fatalf("import with force=true succeeded despite TEA_ALLOW_UNSAFE_IMPORT being unset; body=%s", raw)
	}
}

// TestForceImportLandsBadDataWhenFlagSet proves the other half: with
// AllowUnsafeImport set on a disposable test server, both of dataset 4's
// force-import bundles (bad-checksum-mismatch.zip, bad-missing-file.zip)
// import successfully despite their defects -- exactly
// docs/bundle-format.md's documented contract.
func TestForceImportLandsBadDataWhenFlagSet(t *testing.T) {
	srv := newTestServerWithConfig(t, "/tea/v1", func(cfg *config.Config) {
		cfg.AllowUnsafeImport = true
	})

	checksumMismatch, checksumMismatchFiles := testdataset.BadChecksumMismatch()
	status, raw := uploadBundle(t, srv, "/admin/v1/products/import?force=true",
		writeBundleBytes(t, "bad-checksum-mismatch.zip", checksumMismatch, checksumMismatchFiles))
	if status != http.StatusOK {
		t.Fatalf("force-import bad-checksum-mismatch.zip: status=%d body=%s", status, raw)
	}

	missingFile, missingFileFiles := testdataset.BadMissingFile()
	status, raw = uploadBundle(t, srv, "/admin/v1/products/import?force=true",
		writeBundleBytes(t, "bad-missing-file.zip", missingFile, missingFileFiles))
	if status != http.StatusOK {
		t.Fatalf("force-import bad-missing-file.zip: status=%d body=%s", status, raw)
	}
}

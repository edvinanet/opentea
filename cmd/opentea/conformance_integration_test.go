// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/oej/opentea/internal/conformance"
	"github.com/oej/opentea/internal/testdataset"
)

// TestConformanceSuiteAgainstRealServer is Phase 3's own end-to-end proof:
// load the three good reference datasets into a real server via the real
// /admin/v1/products/import endpoint (opentea-specific; see
// docs/bundle-import-export-test-rig.md), then run the generic,
// implementation-agnostic conformance suite (internal/conformance)
// against that server's /tea/v1 exactly as any client of any TEA server
// would, and confirm every data-correctness assertion passes.
func TestConformanceSuiteAgainstRealServer(t *testing.T) {
	srv := newTestServer(t)

	simple := testdataset.Simple()
	complexManifest, complexFiles := testdataset.Complex()
	lifecycle, lifecycleFiles := testdataset.Lifecycle()

	importDataset := func(name string, zipBytes []byte) {
		status, raw := uploadBundle(t, srv, "/admin/v1/products/import", zipBytes)
		if status != http.StatusOK {
			t.Fatalf("import %s: status=%d body=%s", name, status, raw)
		}
	}
	importDataset("simple.zip", writeBundleBytes(t, "simple.zip", simple, nil))
	importDataset("complex.zip", writeBundleBytes(t, "complex.zip", complexManifest, complexFiles))
	importDataset("lifecycle-compliance.zip", writeBundleBytes(t, "lifecycle-compliance.zip", lifecycle, lifecycleFiles))

	baseURL := srv.URL + "/tea/v1"
	report, err := conformance.Run(context.Background(), baseURL, conformance.Options{})
	if err != nil {
		t.Fatalf("conformance.Run: %v", err)
	}

	t.Logf("endpoint+method coverage: %d/%d (%.1f%%)", report.Coverage.Exercised, report.Coverage.Total, report.Coverage.Percent())
	for _, key := range report.Coverage.NeverCalled {
		t.Logf("  never called: %s", key)
	}
	t.Logf("data correctness: %d/%d (%.1f%%)", report.Correctness.Passed, report.Correctness.Total, report.Correctness.Percent())

	if len(report.Correctness.Failed) > 0 {
		for _, label := range report.Correctness.Failed {
			t.Errorf("failed correctness check: %s", label)
		}
	}
	if report.Correctness.Total == 0 {
		t.Fatal("report.Correctness.Total = 0, want a real, non-empty run")
	}
}

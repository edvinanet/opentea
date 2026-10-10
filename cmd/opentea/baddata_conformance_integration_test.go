// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/oej/opentea/internal/config"
	"github.com/oej/opentea/internal/conformance"
	"github.com/oej/opentea/internal/testdataset"
	"github.com/oej/opentea/pkg/teaclient"
)

// TestConformanceSuiteDetectsForceImportedBadData is Phase 4's own payoff,
// end to end: force-import both of dataset 4 half B's bundles onto a
// disposable test server (AllowUnsafeImport set), then run
// internal/conformance.CheckBadData against it exactly as a real client
// would, confirming it actually detects both kinds of corruption rather
// than silently accepting them. See
// docs/consumer-api-conformance-test-rig.md's "Deliberately invalid data"
// section -- this is that section's own test, not a good-dataset check.
func TestConformanceSuiteDetectsForceImportedBadData(t *testing.T) {
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

	baseURL := srv.URL + "/tea/v1"
	tracker, err := conformance.NewCoverageTracker(baseURL)
	if err != nil {
		t.Fatalf("NewCoverageTracker: %v", err)
	}
	client := teaclient.NewClient(baseURL, teaclient.WithHTTPClient(&http.Client{Transport: tracker.Transport(nil)}))

	cor := conformance.NewCorrectnessTracker()
	conformance.CheckBadData(context.Background(), client, cor)

	report := cor.Report()
	t.Logf("bad-data detection: %d/%d (%.1f%%)", report.Passed, report.Total, report.Percent())
	if len(report.Failed) > 0 {
		for _, label := range report.Failed {
			t.Errorf("failed to detect bad data: %s", label)
		}
	}
	if report.Total == 0 {
		t.Fatal("report.Total = 0, want a real, non-empty run")
	}
}

// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package testdataset

import (
	"strings"
	"testing"
)

func TestBadChecksumMismatch(t *testing.T) {
	m, files := BadChecksumMismatch()
	report := checkManifest(t, m, files)
	if report.Valid {
		t.Fatalf("BadChecksumMismatch() validated cleanly, want invalid")
	}
	if len(report.HashMismatches) != 1 {
		t.Fatalf("HashMismatches = %d, want 1 (report: %+v)", len(report.HashMismatches), report)
	}
	if len(report.SchemaErrors) != 0 || len(report.MissingFiles) != 0 || len(report.DanglingReferences) != 0 {
		t.Fatalf("unexpected additional errors: %+v", report)
	}
}

func TestBadMissingFile(t *testing.T) {
	m, files := BadMissingFile()
	report := checkManifest(t, m, files)
	if report.Valid {
		t.Fatalf("BadMissingFile() validated cleanly, want invalid")
	}
	if len(report.MissingFiles) != 1 {
		t.Fatalf("MissingFiles = %d, want 1 (report: %+v)", len(report.MissingFiles), report)
	}
	if len(report.SchemaErrors) != 0 || len(report.HashMismatches) != 0 || len(report.DanglingReferences) != 0 {
		t.Fatalf("unexpected additional errors: %+v", report)
	}
}

func TestBadFormatVersion(t *testing.T) {
	m, files := BadFormatVersion()
	report := checkManifest(t, m, files)
	if report.Valid {
		t.Fatalf("BadFormatVersion() validated cleanly, want invalid")
	}
	if len(report.SchemaErrors) == 0 {
		t.Fatalf("SchemaErrors is empty, want at least one (report: %+v)", report)
	}
}

func TestBadDanglingComponentRelease(t *testing.T) {
	m, files := BadDanglingComponentRelease()
	report := checkManifest(t, m, files)
	if report.Valid {
		t.Fatalf("BadDanglingComponentRelease() validated cleanly, want invalid")
	}
	if len(report.DanglingReferences) != 1 {
		t.Fatalf("DanglingReferences = %d, want 1 (report: %+v)", len(report.DanglingReferences), report)
	}
	if !strings.Contains(report.DanglingReferences[0], "component release") {
		t.Fatalf("DanglingReferences[0] = %q, want it to mention the dangling component release", report.DanglingReferences[0])
	}
	if len(report.SchemaErrors) != 0 || len(report.HashMismatches) != 0 || len(report.MissingFiles) != 0 {
		t.Fatalf("unexpected additional errors: %+v", report)
	}
}

func TestBadDanglingComponent(t *testing.T) {
	m, files := BadDanglingComponent()
	report := checkManifest(t, m, files)
	if report.Valid {
		t.Fatalf("BadDanglingComponent() validated cleanly, want invalid")
	}
	if len(report.DanglingReferences) != 1 {
		t.Fatalf("DanglingReferences = %d, want 1 (report: %+v)", len(report.DanglingReferences), report)
	}
	if !strings.Contains(report.DanglingReferences[0], "component ") || strings.Contains(report.DanglingReferences[0], "component release") {
		t.Fatalf("DanglingReferences[0] = %q, want it to mention the dangling component (not a component release)", report.DanglingReferences[0])
	}
	if len(report.SchemaErrors) != 0 || len(report.HashMismatches) != 0 || len(report.MissingFiles) != 0 {
		t.Fatalf("unexpected additional errors: %+v", report)
	}
}

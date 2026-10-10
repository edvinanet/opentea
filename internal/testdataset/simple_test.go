// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package testdataset

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/oej/opentea/internal/bundle"
)

// checkManifest marshals m through WriteBundle into an in-memory zip and
// runs bundle.Check against it -- the same validation a real import or
// bundlecheck run would apply, confirming a generated manifest round-trips
// cleanly through the real schema, not just "the Go struct compiled."
func checkManifest(t *testing.T, m bundle.Manifest, files map[string][]byte) *bundle.CheckReport {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mw, err := zw.Create("manifest.json")
	if err != nil {
		t.Fatalf("create manifest.json entry: %v", err)
	}
	if err := writeManifestJSON(mw, m); err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	for sha256Hex, content := range files {
		fw, err := zw.Create("files/" + sha256Hex)
		if err != nil {
			t.Fatalf("create files/%s entry: %v", sha256Hex, err)
		}
		if _, err := fw.Write(content); err != nil {
			t.Fatalf("write files/%s: %v", sha256Hex, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("read back zip: %v", err)
	}
	report, err := bundle.Check(zr)
	if err != nil {
		t.Fatalf("bundle.Check: %v", err)
	}
	return report
}

func TestSimpleValidatesAgainstSchema(t *testing.T) {
	m := Simple()
	report := checkManifest(t, m, nil)
	if !report.Valid {
		t.Fatalf("Simple() produced an invalid bundle: %+v", report)
	}
	if len(m.Components) != 2 {
		t.Fatalf("Components = %d, want 2", len(m.Components))
	}
	if len(m.ComponentReleases) != 4 {
		t.Fatalf("ComponentReleases = %d, want 4", len(m.ComponentReleases))
	}
	if len(m.Collections) != 10 {
		t.Fatalf("Collections = %d, want 10 (5 releases x 2 versions)", len(m.Collections))
	}
	for _, c := range m.Collections {
		if len(c.Artifacts) != 1 {
			t.Fatalf("collection %s v%d has %d artifacts, want 1", c.UUID, c.Version, len(c.Artifacts))
		}
	}
}

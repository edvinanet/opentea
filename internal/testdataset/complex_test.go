// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package testdataset

import (
	"testing"
)

func TestComplexValidatesAgainstSchema(t *testing.T) {
	m, files := Complex()
	report := checkManifest(t, m, files)
	if !report.Valid {
		t.Fatalf("Complex() produced an invalid bundle: %+v", report)
	}

	if len(m.Components) != 10 {
		t.Fatalf("Components = %d, want 10", len(m.Components))
	}
	if len(m.ComponentReleases) != 100 {
		t.Fatalf("ComponentReleases = %d, want 100 (10 components x 10 releases)", len(m.ComponentReleases))
	}
	if len(m.ProductReleases) != 3 {
		t.Fatalf("ProductReleases = %d, want 3", len(m.ProductReleases))
	}
	if len(files) != 2 {
		t.Fatalf("embedded files = %d, want 2", len(files))
	}

	// Every collection has >= 2 artifacts.
	for _, c := range m.Collections {
		if len(c.Artifacts) < 2 {
			t.Fatalf("collection %s v%d has %d artifacts, want >= 2", c.UUID, c.Version, len(c.Artifacts))
		}
	}

	// The shared license artifact's UUID appears in every collection.
	sharedLicenseUUID := deterministicUUID("complex", "artifact", "shared-license")
	for _, c := range m.Collections {
		found := false
		for _, a := range c.Artifacts {
			if a.UUID == sharedLicenseUUID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("collection %s v%d (belongsTo=%s) is missing the shared license artifact", c.UUID, c.Version, c.BelongsTo)
		}
	}

	// Component releases: 100 component releases + 3 product releases'
	// collections (1+1+4 = 6) = 106 collections total.
	if want := 100 + 6; len(m.Collections) != want {
		t.Fatalf("Collections = %d, want %d", len(m.Collections), want)
	}

	// Product release 1.0.0's collection has 4 versions.
	pr1UUID := deterministicUUID("complex", "productrelease", "1.0.0")
	var pr1Versions []int
	for _, c := range m.Collections {
		if c.UUID == pr1UUID {
			pr1Versions = append(pr1Versions, c.Version)
		}
	}
	if len(pr1Versions) != 4 {
		t.Fatalf("product release 1.0.0 has %d collection versions, want 4 (got %v)", len(pr1Versions), pr1Versions)
	}

	// Component 3's release 1 (index 0) is pinned, unchanged, by all three
	// product releases -- confirm the same componentRelease UUID is
	// referenced from all three.
	comp3Release1UUID := deterministicUUID("complex", "componentrelease", "3", "1.0.0")
	pinnedByCount := 0
	for _, pr := range m.ProductReleases {
		for _, ref := range pr.Components {
			if ref.UUID == deterministicUUID("complex", "component", "3") && ref.Release != nil && *ref.Release == comp3Release1UUID {
				pinnedByCount++
			}
		}
	}
	if pinnedByCount != 3 {
		t.Fatalf("component 3 release 1 pinned by %d product releases, want 3", pinnedByCount)
	}

	// The multi-format certificate artifact has exactly two formats.
	certUUID := deterministicUUID("complex", "artifact", "compliance-certificate")
	var certFormats int
	for _, c := range m.Collections {
		for _, a := range c.Artifacts {
			if a.UUID == certUUID {
				certFormats = len(a.Formats)
			}
		}
	}
	if certFormats != 2 {
		t.Fatalf("compliance certificate has %d formats, want 2", certFormats)
	}

	// Markers extract correctly from the product's own name.
	if marker, ok := ExtractMarker(m.Product.Name); !ok || marker != m.Product.UUID {
		t.Fatalf("ExtractMarker(%q) = %q, %v; want %q, true", m.Product.Name, marker, ok, m.Product.UUID)
	}
}

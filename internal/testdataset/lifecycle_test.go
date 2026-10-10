// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package testdataset

import (
	"testing"

	"github.com/oej/opentea/pkg/tea"
)

func TestLifecycleValidatesAgainstSchema(t *testing.T) {
	m, files := Lifecycle()
	report := checkManifest(t, m, files)
	if !report.Valid {
		t.Fatalf("Lifecycle() produced an invalid bundle: %+v", report)
	}

	if len(files) != 1 {
		t.Fatalf("embedded files = %d, want 1", len(files))
	}
	if len(m.Components) != 2 {
		t.Fatalf("Components = %d, want 2", len(m.Components))
	}
	if len(m.ComponentReleases) != 4 {
		t.Fatalf("ComponentReleases = %d, want 4", len(m.ComponentReleases))
	}
	if len(m.ProductReleases) != 2 {
		t.Fatalf("ProductReleases = %d, want 2", len(m.ProductReleases))
	}

	// All nine CLEEventType values appear exactly once, somewhere across
	// product/productRelease/component/componentRelease CLE.
	wantTypes := []string{
		tea.CLEEventTypeReleased, tea.CLEEventTypeEndOfDevelopment, tea.CLEEventTypeEndOfSupport,
		tea.CLEEventTypeEndOfLife, tea.CLEEventTypeEndOfDistribution, tea.CLEEventTypeEndOfMarketing,
		tea.CLEEventTypeSupersededBy, tea.CLEEventTypeComponentRenamed, tea.CLEEventTypeWithdrawn,
	}
	seen := map[string]int{}
	var allEvents []tea.CLEEvent
	if m.Product.CLE != nil {
		allEvents = append(allEvents, m.Product.CLE.Events...)
	}
	for _, pr := range m.ProductReleases {
		if pr.CLE != nil {
			allEvents = append(allEvents, pr.CLE.Events...)
		}
	}
	for _, c := range m.Components {
		if c.CLE != nil {
			allEvents = append(allEvents, c.CLE.Events...)
		}
	}
	for _, cr := range m.ComponentReleases {
		if cr.CLE != nil {
			allEvents = append(allEvents, cr.CLE.Events...)
		}
	}
	for _, e := range allEvents {
		seen[e.Type]++
	}
	if len(allEvents) != 9 {
		t.Fatalf("total CLE events = %d, want 9 (got %v)", len(allEvents), seen)
	}
	for _, wt := range wantTypes {
		if seen[wt] != 1 {
			t.Fatalf("CLEEventType %q appears %d times, want exactly 1 (all: %v)", wt, seen[wt], seen)
		}
	}

	// At least one cle.definitions.support[] entry exists.
	foundSupportDef := false
	for _, pr := range m.ProductReleases {
		if pr.CLE != nil && pr.CLE.Definitions != nil && len(pr.CLE.Definitions.Support) > 0 {
			foundSupportDef = true
		}
	}
	if !foundSupportDef {
		t.Fatalf("no cle.definitions.support[] entry found")
	}

	// COMPLIANCE_DOCUMENT identifiers appear on exactly one component and
	// one component release (TEA 1.0 restricts this identifier type to
	// those two entity types).
	compWithCompliance := 0
	for _, c := range m.Components {
		for _, id := range c.Identifiers {
			if id.IDType == tea.IdentifierTypeComplianceDocument {
				compWithCompliance++
			}
		}
	}
	if compWithCompliance != 1 {
		t.Fatalf("components with a COMPLIANCE_DOCUMENT identifier = %d, want 1", compWithCompliance)
	}
	crWithCompliance := 0
	for _, cr := range m.ComponentReleases {
		for _, id := range cr.Identifiers {
			if id.IDType == tea.IdentifierTypeComplianceDocument {
				crWithCompliance++
			}
		}
	}
	if crWithCompliance != 1 {
		t.Fatalf("component releases with a COMPLIANCE_DOCUMENT identifier = %d, want 1", crWithCompliance)
	}

	// A real CERTIFICATION artifact with embedded content and a
	// signature URL on another artifact format exist somewhere in the
	// collections.
	foundCert := false
	foundSignature := false
	for _, c := range m.Collections {
		for _, a := range c.Artifacts {
			if a.Type == tea.ArtifactTypeCertification {
				foundCert = true
				for _, f := range a.Formats {
					for _, sum := range f.Checksums {
						if sum.AlgType == tea.ChecksumTypeSHA256 {
							if _, ok := files[sum.AlgValue]; !ok {
								t.Fatalf("certification artifact references sha256 %s not present in embedded files", sum.AlgValue)
							}
						}
					}
				}
			}
			for _, f := range a.Formats {
				if f.SignatureURL != "" {
					foundSignature = true
				}
			}
		}
	}
	if !foundCert {
		t.Fatalf("no CERTIFICATION artifact with embedded content found")
	}
	if !foundSignature {
		t.Fatalf("no artifact format with a signatureUrl found")
	}

	// Markers extract correctly from the product's own name.
	if marker, ok := ExtractMarker(m.Product.Name); !ok || marker != m.Product.UUID {
		t.Fatalf("ExtractMarker(%q) = %q, %v; want %q, true", m.Product.Name, marker, ok, m.Product.UUID)
	}
}

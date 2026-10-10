// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package testdataset

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/oej/opentea/internal/bundle"
	"github.com/oej/opentea/pkg/tea"
)

// Lifecycle is the breadth-not-scale reference dataset docs/bundle-import-export-test-rig.md
// describes: 1 product (2 releases), 2 components (2 releases each), one
// example of every CLE event type spread across all four entity levels the
// cle additive property exists on, a support-policy definition,
// COMPLIANCE_DOCUMENT identifiers on the two entity kinds TEA 1.0 allows
// them on, and one real compliance artifact with embedded content and a
// signature URL. Returns the manifest plus its one embedded file.
func Lifecycle() (bundle.Manifest, map[string][]byte) {
	createdAt := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	files := map[string][]byte{}

	productUUID := deterministicUUID("lifecycle", "product")
	product := tea.Product{
		UUID: productUUID,
		Name: withMarker("Acme Lifecycle Device", productUUID),
		Identifiers: []tea.Identifier{
			{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/acme-lifecycle-device"},
		},
	}
	productCLE := &tea.CLE{
		Events: []tea.CLEEvent{
			{ID: 1, Type: tea.CLEEventTypeReleased, Effective: createdAt, Published: createdAt, Version: "1.0.0",
				Description: withMarker("Initial release", productUUID+":cle:1")},
			{ID: 2, Type: tea.CLEEventTypeEndOfMarketing, Effective: createdAt.AddDate(3, 0, 0), Published: createdAt,
				Description: withMarker("Marketing ends three years after release", productUUID+":cle:2")},
		},
	}

	releaseUUID1 := deterministicUUID("lifecycle", "productrelease", "1.0.0")
	releaseUUID2 := deterministicUUID("lifecycle", "productrelease", "2.0.0")

	release1CLE := &tea.CLE{
		Events: []tea.CLEEvent{
			{ID: 1, Type: tea.CLEEventTypeEndOfDevelopment, Effective: createdAt.AddDate(1, 0, 0), Published: createdAt,
				Description: withMarker("Active development ends after one year", releaseUUID1+":cle:1")},
			{ID: 2, Type: tea.CLEEventTypeEndOfDistribution, Effective: createdAt.AddDate(2, 0, 0), Published: createdAt,
				Description: withMarker("Distribution ends after two years", releaseUUID1+":cle:2")},
		},
		Definitions: &tea.CLEDefinitions{
			Support: []tea.CLESupportDefinition{
				{ID: "standard", Description: "Standard support: best-effort patches for two years from release", URL: "https://example.com/support/standard"},
			},
		},
	}

	// Two components, each with two releases.
	comp1UUID := deterministicUUID("lifecycle", "component", "1")
	comp1 := tea.Component{
		UUID: comp1UUID,
		Name: withMarker("Acme Lifecycle Core Module", comp1UUID),
		Identifiers: []tea.Identifier{
			{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/acme-lifecycle-core-module"},
			{IDType: tea.IdentifierTypeComplianceDocument, IDValue: tea.ComplianceDocumentTypeISO27001},
		},
	}
	comp1CLE := &tea.CLE{
		Events: []tea.CLEEvent{
			{ID: 1, Type: tea.CLEEventTypeEndOfSupport, Effective: createdAt.AddDate(2, 0, 0), Published: createdAt,
				Description: withMarker("Support ends after two years", comp1UUID+":cle:1")},
			{ID: 2, Type: tea.CLEEventTypeComponentRenamed, Effective: createdAt.AddDate(0, 6, 0), Published: createdAt,
				Description: withMarker("Renamed from an internal codename to its public name", comp1UUID+":cle:2")},
		},
	}

	comp1Rel1UUID := deterministicUUID("lifecycle", "componentrelease", "1", "1.0.0")
	comp1Rel1CLE := &tea.CLE{
		Events: []tea.CLEEvent{
			{ID: 1, Type: tea.CLEEventTypeEndOfLife, Effective: createdAt.AddDate(3, 0, 0), Published: createdAt,
				Description: withMarker("End of life three years after this release", comp1Rel1UUID+":cle:1")},
			{ID: 2, Type: tea.CLEEventTypeSupersededBy, Effective: createdAt.AddDate(0, 8, 0), Published: createdAt,
				SupersededByVersion: "2.0.0", Description: withMarker("Superseded by release 2.0.0", comp1Rel1UUID+":cle:2")},
			{ID: 3, Type: tea.CLEEventTypeWithdrawn, Effective: createdAt.AddDate(0, 1, 0), Published: createdAt,
				EventID: intPtr(2), Reason: withMarker("Superseded-by date corrected after initial publication error", comp1Rel1UUID+":cle:3")},
		},
	}

	complianceCertUUID := deterministicUUID("lifecycle", "artifact", "compliance-cert")
	certContent := []byte("Acme Lifecycle Core Module -- ISO 27001 Certification (synthetic test content, not a real certificate)")
	certSum := sha256.Sum256(certContent)
	certHex := hex.EncodeToString(certSum[:])
	files[certHex] = certContent
	complianceCert := tea.Artifact{
		UUID: complianceCertUUID, Version: 1, Type: tea.ArtifactTypeCertification,
		Name:        withMarker("ISO 27001 Certification", complianceCertUUID),
		CreatedDate: &createdAt,
		Formats: []tea.ArtifactFormat{
			{
				MediaType: "application/pdf", Description: "ISO 27001 certification document",
				Checksums: []tea.Checksum{{AlgType: tea.ChecksumTypeSHA256, AlgValue: certHex}},
			},
		},
	}

	comp1Rel1SBOM := artifactSBOM("lifecycle", "component 1 release 1.0.0", createdAt)
	// Standard artifact-format signature field (TEA 1.0 spec, not the
	// Trust Architecture overlay) -- an external signature location, the
	// one artifact format in this dataset that carries one.
	comp1Rel1SBOM.Formats[0].SignatureURL = "https://example.com/acme/lifecycle/core-module/1.0.0/sbom.json.sig"

	comp1Release1 := tea.ComponentRelease{
		UUID: comp1Rel1UUID, Component: comp1UUID, Version: "1.0.0", CreatedDate: createdAt,
		Identifiers: []tea.Identifier{
			{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/acme-lifecycle-core-module@1.0.0"},
			{IDType: tea.IdentifierTypeComplianceDocument, IDValue: tea.ComplianceDocumentTypeSOC2TypeII},
		},
	}
	comp1Release2 := tea.ComponentRelease{
		UUID: deterministicUUID("lifecycle", "componentrelease", "1", "2.0.0"), Component: comp1UUID, Version: "2.0.0",
		CreatedDate: createdAt.AddDate(0, 8, 0),
		Identifiers: []tea.Identifier{
			{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/acme-lifecycle-core-module@2.0.0"},
		},
	}

	comp2UUID := deterministicUUID("lifecycle", "component", "2")
	comp2 := tea.Component{
		UUID: comp2UUID,
		Name: withMarker("Acme Lifecycle Sensor Module", comp2UUID),
		Identifiers: []tea.Identifier{
			{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/acme-lifecycle-sensor-module"},
		},
	}
	comp2Release1 := tea.ComponentRelease{
		UUID: deterministicUUID("lifecycle", "componentrelease", "2", "1.0.0"), Component: comp2UUID, Version: "1.0.0", CreatedDate: createdAt,
		Identifiers: []tea.Identifier{{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/acme-lifecycle-sensor-module@1.0.0"}},
	}
	comp2Release2 := tea.ComponentRelease{
		UUID: deterministicUUID("lifecycle", "componentrelease", "2", "2.0.0"), Component: comp2UUID, Version: "2.0.0", CreatedDate: createdAt.AddDate(0, 8, 0),
		Identifiers: []tea.Identifier{{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/acme-lifecycle-sensor-module@2.0.0"}},
	}

	comp1Rel1 := comp1Release1.UUID
	comp2Rel1 := comp2Release1.UUID
	productRelease1 := tea.ProductRelease{
		UUID: releaseUUID1, Product: productUUID, Version: "1.0.0", CreatedDate: createdAt,
		Identifiers: []tea.Identifier{{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/acme-lifecycle-device@1.0.0"}},
		Components: []tea.ComponentRef{
			{UUID: comp1UUID, Release: &comp1Rel1},
			{UUID: comp2UUID, Release: &comp2Rel1},
		},
	}
	comp1Rel2 := comp1Release2.UUID
	comp2Rel2 := comp2Release2.UUID
	productRelease2 := tea.ProductRelease{
		UUID: releaseUUID2, Product: productUUID, Version: "2.0.0", CreatedDate: createdAt.AddDate(0, 8, 0),
		Identifiers: []tea.Identifier{{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/acme-lifecycle-device@2.0.0"}},
		Components: []tea.ComponentRef{
			{UUID: comp1UUID, Release: &comp1Rel2},
			{UUID: comp2UUID, Release: &comp2Rel2},
		},
	}

	collections := []tea.Collection{
		simpleCollection(releaseUUID1, tea.CollectionBelongsToProductRelease, createdAt, "product release 1.0.0", artifactSBOM("lifecycle", "product release 1.0.0", createdAt)),
		simpleCollection(releaseUUID2, tea.CollectionBelongsToProductRelease, createdAt.AddDate(0, 8, 0), "product release 2.0.0", artifactSBOM("lifecycle", "product release 2.0.0", createdAt.AddDate(0, 8, 0))),
		{
			UUID: comp1Rel1UUID, Version: 1, CreatedDate: createdAt, BelongsTo: tea.CollectionBelongsToComponentRelease,
			UpdateReason: &tea.UpdateReason{Type: tea.CollectionUpdateReasonInitialRelease, Comment: withMarker("Initial collection for component 1 release 1.0.0", comp1Rel1UUID+":1")},
			Artifacts:    []tea.Artifact{comp1Rel1SBOM, complianceCert},
		},
		simpleCollection(comp1Release2.UUID, tea.CollectionBelongsToComponentRelease, createdAt.AddDate(0, 8, 0), "component 1 release 2.0.0", artifactSBOM("lifecycle", "component 1 release 2.0.0", createdAt.AddDate(0, 8, 0))),
		simpleCollection(comp2Release1.UUID, tea.CollectionBelongsToComponentRelease, createdAt, "component 2 release 1.0.0", artifactSBOM("lifecycle", "component 2 release 1.0.0", createdAt)),
		simpleCollection(comp2Release2.UUID, tea.CollectionBelongsToComponentRelease, createdAt.AddDate(0, 8, 0), "component 2 release 2.0.0", artifactSBOM("lifecycle", "component 2 release 2.0.0", createdAt.AddDate(0, 8, 0))),
	}

	return bundle.Manifest{
		FormatVersion: bundle.FormatVersion,
		ExportedAt:    createdAt,
		Product:       bundle.ProductEntry{Product: product, CLE: productCLE},
		ProductReleases: []bundle.ProductReleaseEntry{
			{ProductRelease: productRelease1, CLE: release1CLE},
			{ProductRelease: productRelease2},
		},
		Components: []bundle.ComponentEntry{
			{Component: comp1, CLE: comp1CLE},
			{Component: comp2},
		},
		ComponentReleases: []bundle.ComponentReleaseEntry{
			{ComponentRelease: comp1Release1, CLE: comp1Rel1CLE},
			{ComponentRelease: comp1Release2},
			{ComponentRelease: comp2Release1},
			{ComponentRelease: comp2Release2},
		},
		Collections: collections,
	}, files
}

func intPtr(i int) *int { return &i }

// artifactSBOM builds a single url-only SBOM artifact for label, scoped
// into namespace so its UUID stays deterministic and distinct across
// datasets and call sites.
func artifactSBOM(namespace, label string, createdDate time.Time) tea.Artifact {
	uuid := deterministicUUID(namespace, "artifact", "sbom", label)
	return tea.Artifact{
		UUID: uuid, Version: 1, Type: tea.ArtifactTypeBOM,
		Name:        withMarker("SBOM for "+label, uuid),
		CreatedDate: &createdDate,
		Formats: []tea.ArtifactFormat{
			{MediaType: "application/vnd.cyclonedx+json", Description: "CycloneDX SBOM (JSON)", URL: "https://example.com/" + namespace + "/sbom.json"},
		},
	}
}

func simpleCollection(ownerUUID, belongsTo string, createdDate time.Time, label string, artifact tea.Artifact) tea.Collection {
	return tea.Collection{
		UUID: ownerUUID, Version: 1, CreatedDate: createdDate, BelongsTo: belongsTo,
		UpdateReason: &tea.UpdateReason{Type: tea.CollectionUpdateReasonInitialRelease, Comment: withMarker("Initial collection for "+label, ownerUUID+":1")},
		Artifacts:    []tea.Artifact{artifact},
	}
}

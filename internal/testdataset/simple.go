// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package testdataset

import (
	"time"

	"github.com/oej/opentea/internal/bundle"
	"github.com/oej/opentea/pkg/tea"
)

// Simple is the small reference dataset docs/bundle-import-export-test-rig.md
// describes: 1 product, 1 release, 2 components with 2 releases each, every
// one of the resulting 5 releases carrying 2 collection versions (v1
// references an SBOM artifact's version 1, v2 that same artifact's version
// 2 -- an ARTIFACT_UPDATED story).
func Simple() bundle.Manifest {
	createdAt := time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)

	productUUID := deterministicUUID("simple", "product")
	product := tea.Product{
		UUID: productUUID,
		Name: withMarker("Acme Simple Gadget", productUUID),
		Identifiers: []tea.Identifier{
			{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/acme-simple-gadget"},
		},
	}

	releaseUUID := deterministicUUID("simple", "productrelease", "1.0.0")
	components := make([]tea.ComponentRef, 0, 2)
	var componentEntries []bundle.ComponentEntry
	var componentReleaseEntries []bundle.ComponentReleaseEntry
	var collections []tea.Collection

	for i := 1; i <= 2; i++ {
		compUUID := deterministicUUID("simple", "component", itoa(i))
		comp := tea.Component{
			UUID: compUUID,
			Name: withMarker(ordinalName("Acme Simple Component", i), compUUID),
			Identifiers: []tea.Identifier{
				{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/acme-simple-component-" + itoa(i)},
			},
		}
		componentEntries = append(componentEntries, bundle.ComponentEntry{Component: comp})

		var lastReleaseUUID string
		for v := 1; v <= 2; v++ {
			version := itoa(v) + ".0.0"
			compReleaseUUID := deterministicUUID("simple", "componentrelease", itoa(i), version)
			compRelease := tea.ComponentRelease{
				UUID:        compReleaseUUID,
				Component:   compUUID,
				Version:     version,
				CreatedDate: createdAt.AddDate(0, v-1, 0),
				Identifiers: []tea.Identifier{
					{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/acme-simple-component-" + itoa(i) + "@" + version},
				},
			}
			componentReleaseEntries = append(componentReleaseEntries, bundle.ComponentReleaseEntry{
				ComponentRelease: compRelease,
			})
			collections = append(collections, sbomVersionedCollectionPair(compReleaseUUID, tea.CollectionBelongsToComponentRelease, createdAt.AddDate(0, v-1, 0), "component "+itoa(i)+" release "+version)...)
			lastReleaseUUID = compReleaseUUID
		}
		components = append(components, tea.ComponentRef{UUID: compUUID, Release: &lastReleaseUUID})
	}

	productRelease := tea.ProductRelease{
		UUID:        releaseUUID,
		Product:     productUUID,
		Version:     "1.0.0",
		CreatedDate: createdAt,
		Identifiers: []tea.Identifier{
			{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/acme-simple-gadget@1.0.0"},
		},
		Components: components,
	}
	collections = append(collections, sbomVersionedCollectionPair(releaseUUID, tea.CollectionBelongsToProductRelease, createdAt, "product release 1.0.0")...)

	return bundle.Manifest{
		FormatVersion:     bundle.FormatVersion,
		ExportedAt:        createdAt,
		Product:           bundle.ProductEntry{Product: product},
		ProductReleases:   []bundle.ProductReleaseEntry{{ProductRelease: productRelease}},
		Components:        componentEntries,
		ComponentReleases: componentReleaseEntries,
		Collections:       collections,
	}
}

// sbomVersionedCollectionPair returns the two collection versions every
// release in the simple dataset gets: v1 referencing a fresh SBOM
// artifact's version 1, v2 referencing that same artifact's version 2 --
// deliberately the smallest shape that exercises both collection
// versioning and artifact versioning together.
func sbomVersionedCollectionPair(ownerUUID, belongsTo string, createdAt time.Time, label string) []tea.Collection {
	artifactUUID := deterministicUUID("simple", "artifact", ownerUUID)
	v1 := tea.Artifact{
		UUID: artifactUUID, Version: 1, Type: tea.ArtifactTypeBOM,
		Name:        withMarker("SBOM for "+label, artifactUUID),
		CreatedDate: &createdAt,
		Formats: []tea.ArtifactFormat{
			{MediaType: "application/vnd.cyclonedx+json", Description: "CycloneDX SBOM (JSON), initial", URL: "https://example.com/sboms/" + artifactUUID + "-v1.json"},
		},
	}
	updatedAt := createdAt.AddDate(0, 0, 14)
	v2 := tea.Artifact{
		UUID: artifactUUID, Version: 2, Type: tea.ArtifactTypeBOM,
		Name:        withMarker("SBOM for "+label, artifactUUID),
		CreatedDate: &updatedAt,
		Formats: []tea.ArtifactFormat{
			{MediaType: "application/vnd.cyclonedx+json", Description: "CycloneDX SBOM (JSON), corrected", URL: "https://example.com/sboms/" + artifactUUID + "-v2.json"},
		},
	}
	return []tea.Collection{
		{
			UUID: ownerUUID, Version: 1, CreatedDate: createdAt, BelongsTo: belongsTo,
			UpdateReason: &tea.UpdateReason{Type: tea.CollectionUpdateReasonInitialRelease, Comment: withMarker("Initial collection for "+label, ownerUUID+":1")},
			Artifacts:    []tea.Artifact{v1},
		},
		{
			UUID: ownerUUID, Version: 2, CreatedDate: updatedAt, BelongsTo: belongsTo,
			UpdateReason: &tea.UpdateReason{Type: tea.CollectionUpdateReasonArtifactUpdated, Comment: withMarker("Corrected SBOM for "+label, ownerUUID+":2")},
			Artifacts:    []tea.Artifact{v2},
		},
	}
}

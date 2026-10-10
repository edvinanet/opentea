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

// componentLadder is one component's 10 releases, built once and reused
// by Complex's product-release pinning logic below -- each entry is that
// release's own full data plus the collection already built for it.
type componentLadder struct {
	component          tea.Component
	releases           []tea.ComponentRelease // index 0 = release "1.0.0", etc.
	releaseCollections []tea.Collection       // one collection per release, same index
}

// Complex is the large reference dataset docs/bundle-import-export-test-rig.md
// describes: 1 product, 3 releases, 10 components x 10 releases each (100
// component releases), shared artifacts reachable from many collections, a
// 4-version collection history on one release, and a multi-format artifact.
// Returns the manifest plus the embedded file content (keyed by SHA-256
// hex) a handful of its artifacts actually carry.
func Complex() (bundle.Manifest, map[string][]byte) {
	createdAt := time.Date(2026, 2, 1, 9, 0, 0, 0, time.UTC)
	files := map[string][]byte{}

	productUUID := deterministicUUID("complex", "product")
	product := tea.Product{
		UUID: productUUID,
		Name: withMarker("Acme MegaSuite", productUUID),
		Identifiers: []tea.Identifier{
			{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/acme-megasuite"},
		},
	}

	sharedLicense := tea.Artifact{
		UUID: deterministicUUID("complex", "artifact", "shared-license"), Version: 1,
		Type: tea.ArtifactTypeLicense,
		Name: withMarker("Acme Group License Statement", deterministicUUID("complex", "artifact", "shared-license")),
		Formats: []tea.ArtifactFormat{
			{MediaType: "text/plain", Description: "Group-wide license statement", URL: "https://example.com/acme/license.txt"},
		},
	}
	sharedBaseImage := tea.Artifact{
		UUID: deterministicUUID("complex", "artifact", "shared-baseimage"), Version: 1,
		Type: tea.ArtifactTypeBOM,
		Name: withMarker("Acme Base Image SBOM", deterministicUUID("complex", "artifact", "shared-baseimage")),
		Formats: []tea.ArtifactFormat{
			{MediaType: "application/vnd.cyclonedx+json", Description: "Shared base image SBOM", URL: "https://example.com/acme/base-image-sbom.json"},
		},
	}

	// 10 components, each with 10 releases and one collection per release.
	ladders := make([]componentLadder, 10)
	for i := 0; i < 10; i++ {
		ladders[i] = buildComponentLadder(i+1, createdAt, sharedLicense, sharedBaseImage, files)
	}

	var componentEntries []bundle.ComponentEntry
	var componentReleaseEntries []bundle.ComponentReleaseEntry
	var collections []tea.Collection
	for _, l := range ladders {
		componentEntries = append(componentEntries, bundle.ComponentEntry{Component: l.component})
		for _, r := range l.releases {
			componentReleaseEntries = append(componentReleaseEntries, bundle.ComponentReleaseEntry{ComponentRelease: r})
		}
		collections = append(collections, l.releaseCollections...)
	}

	// Product-release pinning schedule: which release index (0-based) of
	// each component each product release pins. A component reused
	// unchanged between two product releases (comp 3 throughout; comp 7
	// between its two appearances) pins the *same* release index both
	// times -- the same componentReleaseUUID becomes reachable from more
	// than one product release's scope, a second, structural source of
	// cross-collection artifact reuse alongside the two shared artifacts
	// above.
	pins := map[string][3]int{ // component index (1-based, as string) -> release index pinned at v1/v2/v3 (-1 = not present)
		"1": {0, 1, 2}, "2": {0, 1, 2}, "3": {0, 0, 0} /* unchanged throughout */, "4": {0, 1, 2}, "5": {0, 1, 2}, "6": {0, 1, 2},
		"7": {-1, 0, 0} /* introduced v2, unchanged into v3 */, "8": {-1, 0, 1},
		"9": {-1, -1, 0}, "10": {-1, -1, 0},
	}

	productReleases := make([]bundle.ProductReleaseEntry, 0, 3)
	versions := []string{"1.0.0", "2.0.0", "3.0.0"}
	for prIdx, version := range versions {
		releaseUUID := deterministicUUID("complex", "productrelease", version)
		var refs []tea.ComponentRef
		for i := 1; i <= 10; i++ {
			schedule := pins[itoa(i)]
			relIdx := schedule[prIdx]
			if relIdx < 0 {
				continue
			}
			compUUID := ladders[i-1].component.UUID
			compReleaseUUID := ladders[i-1].releases[relIdx].UUID
			refs = append(refs, tea.ComponentRef{UUID: compUUID, Release: &compReleaseUUID})
		}

		productRelease := tea.ProductRelease{
			UUID: releaseUUID, Product: productUUID, Version: version,
			CreatedDate: createdAt.AddDate(0, prIdx*2, 0),
			Identifiers: []tea.Identifier{
				{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/acme-megasuite@" + version},
			},
			Components: refs,
		}
		productReleases = append(productReleases, bundle.ProductReleaseEntry{ProductRelease: productRelease})

		switch version {
		case "1.0.0":
			collections = append(collections, productReleaseHistory(releaseUUID, version, createdAt, sharedLicense)...)
		case "2.0.0":
			sbom := productSBOM(version, createdAt.AddDate(0, 2, 0))
			collections = append(collections, tea.Collection{
				UUID: releaseUUID, Version: 1, CreatedDate: createdAt.AddDate(0, 2, 0), BelongsTo: tea.CollectionBelongsToProductRelease,
				UpdateReason: &tea.UpdateReason{Type: tea.CollectionUpdateReasonInitialRelease, Comment: withMarker("Initial collection for product release "+version, releaseUUID+":1")},
				Artifacts:    []tea.Artifact{sbom, sharedLicense},
			})
		case "3.0.0":
			sbom := productSBOM(version, createdAt.AddDate(0, 4, 0))
			cert := multiFormatCertificate(files)
			collections = append(collections, tea.Collection{
				UUID: releaseUUID, Version: 1, CreatedDate: createdAt.AddDate(0, 4, 0), BelongsTo: tea.CollectionBelongsToProductRelease,
				UpdateReason: &tea.UpdateReason{Type: tea.CollectionUpdateReasonInitialRelease, Comment: withMarker("Initial collection for product release "+version, releaseUUID+":1")},
				Artifacts:    []tea.Artifact{sbom, sharedLicense, cert},
			})
		}
	}

	return bundle.Manifest{
		FormatVersion:     bundle.FormatVersion,
		ExportedAt:        createdAt,
		Product:           bundle.ProductEntry{Product: product},
		ProductReleases:   productReleases,
		Components:        componentEntries,
		ComponentReleases: componentReleaseEntries,
		Collections:       collections,
	}, files
}

// buildComponentLadder builds one component's 10 releases and one
// collection per release. Components 1-5 additionally carry the shared
// base-image artifact (modeling components that build on a common base
// image) -- components 6-10 don't, so "shared across collections" covers
// both the always-shared license and a partially-shared second artifact.
func buildComponentLadder(index int, createdAt time.Time, sharedLicense, sharedBaseImage tea.Artifact, files map[string][]byte) componentLadder {
	compUUID := deterministicUUID("complex", "component", itoa(index))
	comp := tea.Component{
		UUID: compUUID,
		Name: withMarker(ordinalName("Acme MegaSuite Component", index), compUUID),
		Identifiers: []tea.Identifier{
			{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/acme-megasuite-component-" + pad2(index)},
		},
	}

	releases := make([]tea.ComponentRelease, 10)
	collections := make([]tea.Collection, 10)
	for v := 1; v <= 10; v++ {
		version := itoa(v) + ".0.0"
		releaseUUID := deterministicUUID("complex", "componentrelease", itoa(index), version)
		createdDate := createdAt.AddDate(0, 0, (index-1)*10+v)
		releases[v-1] = tea.ComponentRelease{
			UUID: releaseUUID, Component: compUUID, Version: version, CreatedDate: createdDate,
			Identifiers: []tea.Identifier{
				{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/acme-megasuite-component-" + pad2(index) + "@" + version},
			},
		}

		sbomUUID := deterministicUUID("complex", "artifact", "sbom", itoa(index), version)
		sbom := tea.Artifact{
			UUID: sbomUUID, Version: 1, Type: tea.ArtifactTypeBOM,
			Name:        withMarker("SBOM for component "+pad2(index)+" release "+version, sbomUUID),
			CreatedDate: &createdDate,
			Formats: []tea.ArtifactFormat{
				{MediaType: "application/vnd.cyclonedx+json", Description: "CycloneDX SBOM (JSON)", URL: "https://example.com/acme/component-" + pad2(index) + "/" + version + "/sbom.json"},
			},
		}
		if index == 1 && v == 1 {
			// Exactly one SBOM in the whole complex dataset carries real
			// embedded content -- enough to exercise the files/ embedding
			// mechanism without the zip needing hundreds of fake blobs
			// (export.go's collectSHA256 only pulls a checksum into files/
			// when it's SHA-256; a url-only format needs no files/ entry
			// at all and still validates cleanly).
			content := []byte(`{"bomFormat":"CycloneDX","specVersion":"1.6","components":[]}`)
			sum := sha256.Sum256(content)
			hexSum := hex.EncodeToString(sum[:])
			files[hexSum] = content
			sbom.Formats[0].URL = ""
			sbom.Formats[0].Checksums = []tea.Checksum{{AlgType: tea.ChecksumTypeSHA256, AlgValue: hexSum}}
		}

		artifacts := []tea.Artifact{sbom, sharedLicense}
		if index <= 5 {
			artifacts = append(artifacts, sharedBaseImage)
		}
		collections[v-1] = tea.Collection{
			UUID: releaseUUID, Version: 1, CreatedDate: createdDate, BelongsTo: tea.CollectionBelongsToComponentRelease,
			UpdateReason: &tea.UpdateReason{Type: tea.CollectionUpdateReasonInitialRelease, Comment: withMarker("Initial collection for component "+pad2(index)+" release "+version, releaseUUID+":1")},
			Artifacts:    artifacts,
		}
	}

	return componentLadder{component: comp, releases: releases, releaseCollections: collections}
}

// productReleaseHistory is product release 1.0.0's deliberate 4-version
// collection history -- the "multiple collections with different version
// numbers" case, at the product-release level.
func productReleaseHistory(releaseUUID, version string, createdAt time.Time, sharedLicense tea.Artifact) []tea.Collection {
	sbom := productSBOM(version, createdAt)
	vexUUID := deterministicUUID("complex", "artifact", "productrelease-vex", version)
	vex := tea.Artifact{
		UUID: vexUUID, Version: 1, Type: tea.ArtifactTypeVulnerabilities,
		Name: withMarker("VEX for product release "+version, vexUUID),
		Formats: []tea.ArtifactFormat{
			{MediaType: "application/vnd.cyclonedx+json", Description: "CycloneDX VEX (JSON)", URL: "https://example.com/acme/productrelease/" + version + "/vex.json"},
		},
	}
	attestationUUID := deterministicUUID("complex", "artifact", "productrelease-attestation", version)
	attestation := tea.Artifact{
		UUID: attestationUUID, Version: 1, Type: tea.ArtifactTypeAttestation,
		Name: withMarker("Build attestation for product release "+version, attestationUUID),
		Formats: []tea.ArtifactFormat{
			{MediaType: "application/vnd.in-toto+json", Description: "in-toto build attestation", URL: "https://example.com/acme/productrelease/" + version + "/attestation.json"},
		},
	}

	day := func(n int) time.Time { return createdAt.AddDate(0, 0, n) }
	return []tea.Collection{
		{
			UUID: releaseUUID, Version: 1, CreatedDate: day(0), BelongsTo: tea.CollectionBelongsToProductRelease,
			UpdateReason: &tea.UpdateReason{Type: tea.CollectionUpdateReasonInitialRelease, Comment: withMarker("Initial collection for product release "+version, releaseUUID+":1")},
			Artifacts:    []tea.Artifact{sbom, sharedLicense},
		},
		{
			UUID: releaseUUID, Version: 2, CreatedDate: day(7), BelongsTo: tea.CollectionBelongsToProductRelease,
			UpdateReason: &tea.UpdateReason{Type: tea.CollectionUpdateReasonVEXUpdated, Comment: withMarker("Added VEX for product release "+version, releaseUUID+":2")},
			Artifacts:    []tea.Artifact{sbom, sharedLicense, vex},
		},
		{
			UUID: releaseUUID, Version: 3, CreatedDate: day(14), BelongsTo: tea.CollectionBelongsToProductRelease,
			UpdateReason: &tea.UpdateReason{Type: tea.CollectionUpdateReasonArtifactAdded, Comment: withMarker("Added build attestation for product release "+version, releaseUUID+":3")},
			Artifacts:    []tea.Artifact{sbom, sharedLicense, vex, attestation},
		},
		{
			UUID: releaseUUID, Version: 4, CreatedDate: day(21), BelongsTo: tea.CollectionBelongsToProductRelease,
			UpdateReason: &tea.UpdateReason{Type: tea.CollectionUpdateReasonArtifactRemoved, Comment: withMarker("Removed superseded VEX for product release "+version, releaseUUID+":4")},
			Artifacts:    []tea.Artifact{sbom, sharedLicense, attestation},
		},
	}
}

func productSBOM(version string, createdDate time.Time) tea.Artifact {
	uuid := deterministicUUID("complex", "artifact", "productrelease-sbom", version)
	return tea.Artifact{
		UUID: uuid, Version: 1, Type: tea.ArtifactTypeBOM,
		Name:        withMarker("SBOM for product release "+version, uuid),
		CreatedDate: &createdDate,
		Formats: []tea.ArtifactFormat{
			{MediaType: "application/vnd.cyclonedx+json", Description: "CycloneDX SBOM (JSON)", URL: "https://example.com/acme/productrelease/" + version + "/sbom.json"},
		},
	}
}

// multiFormatCertificate is the one artifact in the complex dataset
// published in two formats under the same artifact -- a regulatory
// compliance certificate, as both PDF and DOC. The PDF format carries
// real embedded content (the second of the dataset's 2-3 embedded
// files); the DOC format stays url-only.
func multiFormatCertificate(files map[string][]byte) tea.Artifact {
	uuid := deterministicUUID("complex", "artifact", "compliance-certificate")
	pdfContent := []byte("%PDF-1.4 Acme MegaSuite Regulatory Compliance Certificate (synthetic test content, not a real PDF)")
	sum := sha256.Sum256(pdfContent)
	hexSum := hex.EncodeToString(sum[:])
	files[hexSum] = pdfContent

	return tea.Artifact{
		UUID: uuid, Version: 1, Type: tea.ArtifactTypeCertification,
		Name: withMarker("Acme MegaSuite Regulatory Compliance Certificate", uuid),
		Formats: []tea.ArtifactFormat{
			{
				MediaType: "application/pdf", Description: "Regulatory compliance certificate (PDF)",
				Checksums: []tea.Checksum{{AlgType: tea.ChecksumTypeSHA256, AlgValue: hexSum}},
			},
			{
				MediaType: "application/msword", Description: "Regulatory compliance certificate (DOC)",
				URL: "https://example.com/acme/compliance/certificate.doc",
			},
		},
	}
}

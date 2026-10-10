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

// badBase builds the minimal valid bundle every bad-bundle variant in this
// file starts from -- one product, one release, one component with one
// release linked into it, one collection carrying one SBOM artifact with a
// real embedded file -- then corrupts exactly one thing. namespace keeps
// each variant's deterministic UUIDs distinct from the others and from the
// good datasets.
func badBase(namespace string) (bundle.Manifest, map[string][]byte, string, []byte) {
	createdAt := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)

	productUUID := deterministicUUID(namespace, "product")
	product := tea.Product{
		UUID: productUUID,
		Name: withMarker("Acme Bad Bundle Example", productUUID),
		Identifiers: []tea.Identifier{
			{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/acme-bad-bundle-" + namespace},
		},
	}

	compUUID := deterministicUUID(namespace, "component")
	comp := tea.Component{
		UUID: compUUID,
		Name: withMarker("Acme Bad Bundle Component", compUUID),
		Identifiers: []tea.Identifier{
			{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/acme-bad-bundle-" + namespace + "-component"},
		},
	}

	compReleaseUUID := deterministicUUID(namespace, "componentrelease")
	compRelease := tea.ComponentRelease{
		UUID: compReleaseUUID, Component: compUUID, Version: "1.0.0", CreatedDate: createdAt,
		Identifiers: []tea.Identifier{
			{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/acme-bad-bundle-" + namespace + "-component@1.0.0"},
		},
	}

	content := []byte("Acme Bad Bundle Example SBOM content, namespace=" + namespace)
	sum := sha256.Sum256(content)
	sha256Hex := hex.EncodeToString(sum[:])
	artifactUUID := deterministicUUID(namespace, "artifact")
	artifact := tea.Artifact{
		UUID: artifactUUID, Version: 1, Type: tea.ArtifactTypeBOM,
		Name:        withMarker("SBOM for Acme Bad Bundle Example", artifactUUID),
		CreatedDate: &createdAt,
		Formats: []tea.ArtifactFormat{
			{
				MediaType: "application/vnd.cyclonedx+json", Description: "CycloneDX SBOM (JSON)",
				Checksums: []tea.Checksum{{AlgType: tea.ChecksumTypeSHA256, AlgValue: sha256Hex}},
			},
		},
	}

	releaseUUID := compReleaseUUID
	releaseRef := releaseUUID
	productRelease := tea.ProductRelease{
		UUID: deterministicUUID(namespace, "productrelease"), Product: productUUID, Version: "1.0.0", CreatedDate: createdAt,
		Identifiers: []tea.Identifier{
			{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/acme-bad-bundle-" + namespace + "@1.0.0"},
		},
		Components: []tea.ComponentRef{{UUID: compUUID, Release: &releaseRef}},
	}

	collection := tea.Collection{
		UUID: compReleaseUUID, Version: 1, CreatedDate: createdAt, BelongsTo: tea.CollectionBelongsToComponentRelease,
		UpdateReason: &tea.UpdateReason{Type: tea.CollectionUpdateReasonInitialRelease, Comment: withMarker("Initial collection", compReleaseUUID+":1")},
		Artifacts:    []tea.Artifact{artifact},
	}

	m := bundle.Manifest{
		FormatVersion:     bundle.FormatVersion,
		ExportedAt:        createdAt,
		Product:           bundle.ProductEntry{Product: product},
		ProductReleases:   []bundle.ProductReleaseEntry{{ProductRelease: productRelease}},
		Components:        []bundle.ComponentEntry{{Component: comp}},
		ComponentReleases: []bundle.ComponentReleaseEntry{{ComponentRelease: compRelease}},
		Collections:       []tea.Collection{collection},
	}
	files := map[string][]byte{sha256Hex: content}
	return m, files, sha256Hex, content
}

// BadChecksumMismatch returns a bundle whose single files/<sha256> entry's
// real content does not match its own filename's hash -- the artifact
// format's claimed checksum is correct, but what's actually stored under
// that name has been swapped for different bytes.
func BadChecksumMismatch() (bundle.Manifest, map[string][]byte) {
	m, _, sha256Hex, _ := badBase("bad-checksum-mismatch")
	files := map[string][]byte{sha256Hex: []byte("this is not the content that hashes to this filename")}
	return m, files
}

// BadMissingFile returns a bundle whose artifact format carries a SHA-256
// checksum with no corresponding files/ entry in the bundle at all.
func BadMissingFile() (bundle.Manifest, map[string][]byte) {
	m, _, _, _ := badBase("bad-missing-file")
	return m, nil
}

// BadFormatVersion returns a bundle whose manifest.json declares a
// formatVersion other than "1.0" (the bundle schema's own const
// constraint) -- a schema validation failure, not a dangling reference or
// hash problem.
func BadFormatVersion() (bundle.Manifest, map[string][]byte) {
	m, files, _, _ := badBase("bad-format-version")
	m.FormatVersion = "2.0"
	return m, files
}

// BadDanglingComponentRelease returns a bundle whose product release names
// a component-release UUID (components[].release) absent from the
// manifest's own top-level componentReleases[] array.
func BadDanglingComponentRelease() (bundle.Manifest, map[string][]byte) {
	m, files, _, _ := badBase("bad-dangling-component-release")
	ghost := deterministicUUID("bad-dangling-component-release", "ghost-component-release")
	m.ProductReleases[0].Components[0].Release = &ghost
	return m, files
}

// BadDanglingComponent returns a bundle whose product release names a
// component UUID (components[].uuid) absent from the manifest's own
// top-level components[] array.
func BadDanglingComponent() (bundle.Manifest, map[string][]byte) {
	m, files, _, _ := badBase("bad-dangling-component")
	ghost := deterministicUUID("bad-dangling-component", "ghost-component")
	m.ProductReleases[0].Components[0].UUID = ghost
	return m, files
}

// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package bundle

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/oej/opentea/pkg/tea"
)

// validEvidenceBundle is a minimal, schema-valid TEA Trust Architecture
// evidence bundle (oej's overlay, not part of the official TEA standard --
// see pkg/tea/trust.go) -- populates every field internal/bundle/schema.json's
// evidenceBundle $def marks required, nothing more.
func validEvidenceBundle() *tea.EvidenceBundle {
	return &tea.EvidenceBundle{
		BundleVersion: "1.0",
		Object: tea.EvidenceObjectRef{
			ObjectType: "ARTIFACT_FORMAT",
			Digest:     tea.EvidenceDigest{Algorithm: "sha-256", Value: "deadbeef"},
		},
		Signature:   tea.EvidenceSignature{Format: "ed25519", Value: "c2lnbmF0dXJl"},
		Certificate: tea.EvidenceCertificate{Format: "pem", Certificate: "-----BEGIN CERTIFICATE-----\nMII...\n-----END CERTIFICATE-----"},
	}
}

func validManifestJSON(t *testing.T) []byte {
	t.Helper()
	now := time.Now().UTC()
	m := Manifest{
		FormatVersion: FormatVersion,
		ExportedAt:    time.Now().UTC(),
		Product: ProductEntry{
			Product: tea.Product{
				UUID:        "11111111-1111-1111-1111-111111111111",
				Name:        "Test Product",
				Identifiers: []tea.Identifier{{IDType: tea.IdentifierTypeTEI, IDValue: "urn:tei:1"}},
			},
		},
		ProductReleases: []ProductReleaseEntry{
			{
				ProductRelease: tea.ProductRelease{
					UUID:        "22222222-2222-2222-2222-222222222222",
					Product:     "11111111-1111-1111-1111-111111111111",
					Version:     "1.0.0",
					CreatedDate: time.Now().UTC(),
					Components:  []tea.ComponentRef{{UUID: "33333333-3333-3333-3333-333333333333"}},
				},
			},
		},
		Components: []ComponentEntry{
			{
				Component: tea.Component{
					UUID:        "33333333-3333-3333-3333-333333333333",
					Name:        "libfoo",
					Identifiers: []tea.Identifier{{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/libfoo"}},
				},
			},
		},
		ComponentReleases: []ComponentReleaseEntry{},
		Collections: []tea.Collection{
			{
				UUID:        "22222222-2222-2222-2222-222222222222",
				Version:     1,
				CreatedDate: now,
				BelongsTo:   "PRODUCT_RELEASE",
				Artifacts: []tea.Artifact{
					{
						UUID:        "44444444-4444-4444-4444-444444444444",
						Version:     1,
						Type:        "BOM",
						CreatedDate: &now,
						Formats: []tea.ArtifactFormat{
							{
								MediaType: "application/vnd.cyclonedx+json",
								Checksums: []tea.Checksum{
									{AlgType: "SHA-256", AlgValue: "e3a851f1fa2cdc51abe1e2b9403fe108efeb7547bd9c1878fcbf4750ace837e"},
								},
								EvidenceBundle: validEvidenceBundle(),
							},
						},
					},
				},
			},
		},
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	return raw
}

func TestValidateManifestAccepts(t *testing.T) {
	if err := ValidateManifest(validManifestJSON(t)); err != nil {
		t.Fatalf("ValidateManifest rejected a valid manifest: %v", err)
	}
}

func TestValidateManifestRejectsMissingRequiredField(t *testing.T) {
	broken := []byte(`{"formatVersion":"1.0","exportedAt":"2026-01-01T00:00:00Z"}`)
	err := ValidateManifest(broken)
	if err == nil {
		t.Fatal("expected an error for a manifest missing required fields")
	}
}

func TestValidateManifestRejectsBadFormatVersion(t *testing.T) {
	raw := validManifestJSON(t)
	mutated := strings.Replace(string(raw), `"formatVersion":"1.0"`, `"formatVersion":"9.9"`, 1)
	if mutated == string(raw) {
		t.Fatal("test setup broken: replacement did not apply")
	}
	if err := ValidateManifest([]byte(mutated)); err == nil {
		t.Fatal("expected an error for a manifest with the wrong formatVersion")
	}
}

func TestValidateManifestRejectsUnknownEnumValue(t *testing.T) {
	raw := validManifestJSON(t)
	mutated := strings.Replace(string(raw), `"idType":"TEI"`, `"idType":"NOT_A_REAL_TYPE"`, 1)
	if mutated == string(raw) {
		t.Fatal("test setup broken: replacement did not apply")
	}
	if err := ValidateManifest([]byte(mutated)); err == nil {
		t.Fatal("expected an error for an invalid identifier idType enum value")
	}
}

// TestValidateManifestRejectsMalformedChecksum is the regression test for
// this session's checksum.algValue pattern/length fix (internal/bundle/
// schema.json): upstream's own checksum schema requires a lowercase hex
// digest of a length matching the algorithm; an uppercase value (valid hex
// content, wrong case) must now be rejected, where it previously validated
// against the old bare `{"type": "string"}` constraint.
func TestValidateManifestRejectsMalformedChecksum(t *testing.T) {
	raw := validManifestJSON(t)
	mutated := strings.Replace(string(raw),
		`"e3a851f1fa2cdc51abe1e2b9403fe108efeb7547bd9c1878fcbf4750ace837e"`,
		`"E3A851F1FA2CDC51ABE1E2B9403FE108EFEB7547BD9C1878FCBF4750ACE837E"`, 1)
	if mutated == string(raw) {
		t.Fatal("test setup broken: replacement did not apply")
	}
	if err := ValidateManifest([]byte(mutated)); err == nil {
		t.Fatal("expected an error for an uppercase-hex checksum algValue")
	}
}

// TestValidateManifestRejectsIncompleteEvidenceBundle is the regression
// test for this session's evidenceBundle/evidenceBundleRef $defs
// (internal/bundle/schema.json) -- added so a product with TEA Trust
// Architecture evidence attached (oej's overlay, not part of the official
// TEA standard) doesn't silently fail schema validation the day evidence
// gets wired into the normal Collection/Artifact read path (currently a
// deliberate Phase 1 scope boundary, so no export produces one yet).
// Removing the required "certificate" object proves the $def's own
// required list is actually enforced, not just present and permissive.
func TestValidateManifestRejectsIncompleteEvidenceBundle(t *testing.T) {
	raw := validManifestJSON(t)
	mutated := strings.Replace(string(raw),
		`,"certificate":{"format":"pem","certificate":"-----BEGIN CERTIFICATE-----\nMII...\n-----END CERTIFICATE-----"}`,
		``, 1)
	if mutated == string(raw) {
		t.Fatal("test setup broken: replacement did not apply")
	}
	if err := ValidateManifest([]byte(mutated)); err == nil {
		t.Fatal("expected an error for an evidenceBundle missing its required certificate")
	}
}

func TestValidateManifestRejectsInvalidJSON(t *testing.T) {
	if err := ValidateManifest([]byte("{not json")); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}

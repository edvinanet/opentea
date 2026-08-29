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

func validManifestJSON(t *testing.T) []byte {
	t.Helper()
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
		Collections:       []tea.Collection{},
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

func TestValidateManifestRejectsInvalidJSON(t *testing.T) {
	if err := ValidateManifest([]byte("{not json")); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}

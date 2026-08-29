// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package tea

import "testing"

func TestExtractTEIAuthority(t *testing.T) {
	tests := []struct {
		name    string
		tei     string
		want    string
		wantErr bool
	}{
		{"plain host", "tei://products.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1", "products.example.com", false},
		{"host with port rejected", "tei://127.0.0.1:8443/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1", "", true},
		{"IPv6 host with port rejected", "tei://[2001:db8::1]:8443/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1", "", true},
		{"purl type with slashes in identifier", "tei://cyclonedx.org/purl/pkg:pypi/cyclonedx-python-lib@8.4.0", "cyclonedx.org", false},
		{"old urn syntax rejected", "urn:tei:uuid:products.example.com:d4d9f54a-abcf-11ee-ac79-1a52914d44b1", "", true},
		{"wrong scheme rejected", "https://products.example.com/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1", "", true},
		{"missing host rejected", "tei:///uuid/d4d9f54a", "", true},
		{"not a URL at all", "::::not a url::::", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractTEIAuthority(tt.tei)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ExtractTEIAuthority(%q) = %q, want error", tt.tei, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ExtractTEIAuthority(%q): %v", tt.tei, err)
			}
			if got != tt.want {
				t.Fatalf("ExtractTEIAuthority(%q) = %q, want %q", tt.tei, got, tt.want)
			}
		})
	}
}

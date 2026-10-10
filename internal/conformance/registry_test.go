// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package conformance

import "testing"

func TestOperationsHas28Entries(t *testing.T) {
	if len(Operations) != 28 {
		t.Fatalf("len(Operations) = %d, want 28 (the TEA 1.0 consumer API's full operation count)", len(Operations))
	}
}

func TestLookupMatchesConcretePaths(t *testing.T) {
	cases := []struct {
		method, path, wantOpID string
	}{
		{"GET", "/discovery", "discover"},
		{"GET", "/products", "queryProducts"},
		{"GET", "/product/abc-123", "getProduct"},
		{"GET", "/product/abc-123/releases", "getProductReleases"},
		{"GET", "/product/abc-123/cle", "getProductCLE"},
		{"GET", "/productReleases", "queryProductReleases"},
		{"GET", "/productRelease/abc-123", "getProductRelease"},
		{"GET", "/productRelease/abc-123/collection/latest", "getLatestProductReleaseCollection"},
		{"GET", "/productRelease/abc-123/collection/7", "getProductReleaseCollection"},
		{"GET", "/componentRelease/xyz/collection/latest", "getLatestComponentReleaseCollection"},
		{"GET", "/componentRelease/xyz/collection/2", "getComponentReleaseCollection"},
		{"GET", "/artifact/xyz/latest", "getLatestArtifact"},
		{"GET", "/artifact/xyz/latest/download", "downloadLatestArtifact"},
		{"GET", "/artifact/xyz/latest/signature/download", "downloadLatestArtifactSignature"},
		{"GET", "/artifact/xyz/5", "getArtifactByVersion"},
		{"GET", "/artifact/xyz/5/download", "downloadArtifactByVersion"},
		{"GET", "/artifact/xyz/5/signature/download", "downloadArtifactSignatureByVersion"},
		{"POST", "/token", "exchangeToken"},
	}
	for _, c := range cases {
		op, ok := Lookup(c.method, c.path)
		if !ok {
			t.Errorf("Lookup(%q, %q) did not match any operation", c.method, c.path)
			continue
		}
		if op.OperationID != c.wantOpID {
			t.Errorf("Lookup(%q, %q).OperationID = %q, want %q", c.method, c.path, op.OperationID, c.wantOpID)
		}
	}
}

func TestLookupDistinguishesLatestFromVersioned(t *testing.T) {
	// "latest" must never accidentally match the {artifactVersion} integer
	// pattern, and vice versa -- these are two distinct spec operations.
	if _, ok := Lookup("GET", "/artifact/xyz/latest"); !ok {
		t.Fatal("latest metadata path didn't match")
	}
	if op, ok := Lookup("GET", "/artifact/xyz/latest"); !ok || op.OperationID != "getLatestArtifact" {
		t.Fatalf("latest metadata path matched the wrong operation: %+v, %v", op, ok)
	}
	if op, ok := Lookup("GET", "/artifact/xyz/42"); !ok || op.OperationID != "getArtifactByVersion" {
		t.Fatalf("versioned metadata path matched the wrong operation: %+v, %v", op, ok)
	}
	// A non-numeric, non-"latest" segment must match neither.
	if _, ok := Lookup("GET", "/artifact/xyz/notaversion"); ok {
		t.Fatal("a nonsense version segment matched some operation, want no match")
	}
}

func TestLookupRejectsUnregisteredPath(t *testing.T) {
	if _, ok := Lookup("GET", "/admin/v1/products"); ok {
		t.Fatal("an admin path matched a consumer-API operation, want no match")
	}
	if _, ok := Lookup("DELETE", "/product/abc-123"); ok {
		t.Fatal("DELETE on a GET-only path matched, want no match")
	}
}

func TestEveryOperationKeyIsUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, op := range Operations {
		if seen[op.Key()] {
			t.Fatalf("duplicate operation key: %s", op.Key())
		}
		seen[op.Key()] = true
	}
}

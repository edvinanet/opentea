// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"bytes"
	"context"
	"net/http"
	"testing"
)

// TestCICDAPIRequiresValidBearerToken is the core regression test for the
// requireCICDCredential gate: missing, garbage, and revoked tokens must
// all be rejected with 401 before any request ever reaches a target --
// this pass never stands up a real internal/publisher target (see
// cmd/opentea/cicdapi_test.go for that end-to-end proof), so a call that
// gets past auth here would fail for an unrelated reason (no reachable
// target), which would make this test ambiguous. Using GET
// .../collectionDraft (no body, cheapest to construct) keeps the request
// itself trivial -- the point is entirely about what happens before it
// would be proxied.
func TestCICDAPIRequiresValidBearerToken(t *testing.T) {
	srv, r, _, _ := newTestServer(t)
	target, err := r.CreateTarget(context.Background(), "Acme production", "https://tea.example.com/publisher/v1", "s3cr3t")
	if err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}
	_, token, err := r.CreateCICDCredential(context.Background(), target.UUID, "CI pipeline")
	if err != nil {
		t.Fatalf("CreateCICDCredential: %v", err)
	}
	if err := r.RevokeCICDCredential(context.Background(), func() string {
		list, _ := r.ListCICDCredentials(context.Background(), target.UUID)
		return list[0].UUID
	}()); err != nil {
		t.Fatalf("RevokeCICDCredential: %v", err)
	}

	path := srv.URL + "/cicdapi/v1/productReleases/00000000-0000-4000-8000-000000000000/collectionDraft"

	doRequest := func(authHeader string) int {
		req, err := http.NewRequest(http.MethodGet, path, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	if status := doRequest(""); status != http.StatusUnauthorized {
		t.Fatalf("missing Authorization header: status=%d, want 401", status)
	}
	if status := doRequest("Bearer not-a-real-token"); status != http.StatusUnauthorized {
		t.Fatalf("garbage token: status=%d, want 401", status)
	}
	if status := doRequest("Bearer " + token); status != http.StatusUnauthorized {
		t.Fatalf("revoked token: status=%d, want 401", status)
	}
}

// TestCICDAPIRouteSurfaceExcludesFullScopedOperations confirms the
// full/cicd line is structural (the routes simply don't exist on this
// mux), not a runtime scope check that could be misconfigured -- there is
// no way for a cicd credential, however presented, to reach product
// creation or draft approval, because opentea-publisher's own /cicdapi/v1
// never registers a handler for them at all.
func TestCICDAPIRouteSurfaceExcludesFullScopedOperations(t *testing.T) {
	srv, _, _, _ := newTestServer(t)

	fullScopedPaths := []struct {
		method, path string
	}{
		{http.MethodPost, "/cicdapi/v1/products"},
		{http.MethodPost, "/cicdapi/v1/components"},
		{http.MethodPost, "/cicdapi/v1/productReleases/00000000-0000-4000-8000-000000000000/components"},
		{http.MethodPost, "/cicdapi/v1/products/00000000-0000-4000-8000-000000000000/cle/events"},
		{http.MethodPost, "/cicdapi/v1/productReleases/00000000-0000-4000-8000-000000000000/collectionDraft/approve"},
		{http.MethodPost, "/cicdapi/v1/productReleases/00000000-0000-4000-8000-000000000000/collectionDraft/reject"},
	}
	for _, p := range fullScopedPaths {
		req, err := http.NewRequest(p.method, srv.URL+p.path, bytes.NewReader(nil))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer irrelevant-no-such-route-exists")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s %s: status=%d, want 404 (route must not exist)", p.method, p.path, resp.StatusCode)
		}
	}
}

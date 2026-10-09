// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestProductsRoutesRequireSession confirms every new target-scoped
// products/releases route redirects an unauthenticated caller to /login,
// the same as every other GUI route (requireSession). This package has
// no fake-target-server precedent (cicdapi_test.go), so this only proves
// access control, not real target interaction -- see
// cmd/opentea/productrelease_gui_test.go for the real end-to-end proof.
func TestProductsRoutesRequireSession(t *testing.T) {
	srv, r, _, _ := newTestServer(t)
	target, err := r.CreateTarget(context.Background(), "Acme production", "http://127.0.0.1:1/publisher/v1", "s3cr3t")
	if err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}

	noJarClient := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodGet, "/targets/" + target.UUID + "/products"},
		{http.MethodPost, "/targets/" + target.UUID + "/products"},
		{http.MethodGet, "/targets/" + target.UUID + "/products/some-uuid"},
		{http.MethodPost, "/targets/" + target.UUID + "/products/some-uuid/releases"},
	} {
		req, err := http.NewRequest(tc.method, srv.URL+tc.path, strings.NewReader(""))
		if err != nil {
			t.Fatalf("new request %s %s: %v", tc.method, tc.path, err)
		}
		if tc.method == http.MethodPost {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", srv.URL)
		}
		resp, err := noJarClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", tc.method, tc.path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login" {
			t.Fatalf("%s %s: status=%d location=%q, want 303 to /login", tc.method, tc.path, resp.StatusCode, resp.Header.Get("Location"))
		}
	}
}

// TestCreateProductFormRejectsEmptyName confirms the minimal form
// validation runs before any target call is attempted -- a missing name
// never reaches teaPublisherClientForTarget at all, so this doesn't need
// a reachable target.
func TestCreateProductFormRejectsEmptyName(t *testing.T) {
	srv, r, username, password := newTestServer(t)
	target, err := r.CreateTarget(context.Background(), "Acme production", "http://127.0.0.1:1/publisher/v1", "s3cr3t")
	if err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}
	client := loggedInClient(t, srv, username, password)

	form := url.Values{"name": {""}}
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/targets/"+target.UUID+"/products", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", srv.URL)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST products: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200 (rerendered with an error)", resp.StatusCode)
	}
}

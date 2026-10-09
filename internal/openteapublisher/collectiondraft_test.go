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

// TestProductReleaseRoutesRequireSession mirrors TestProductsRoutesRequireSession
// for the release-detail/draft/sign-publish routes.
func TestProductReleaseRoutesRequireSession(t *testing.T) {
	srv, r, _, _ := newTestServer(t)
	target, err := r.CreateTarget(context.Background(), "Acme production", "http://127.0.0.1:1/publisher/v1", "s3cr3t")
	if err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}

	noJarClient := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	base := "/targets/" + target.UUID + "/productReleases/some-uuid"

	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodGet, base},
		{http.MethodPost, base + "/collectionDraft"},
		{http.MethodPost, base + "/collectionDraft/approve"},
		{http.MethodPost, base + "/collectionDraft/reject"},
		{http.MethodPost, base + "/collectionDraft/signAndPublish"},
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

// TestCollectionDraftDecideRequiresApprovalRole confirms the protocol-
// level approve/reject routes are gated by requireApprovalRole exactly
// like /approvals' own decide route (TestApprovalWorkflowRoleGating) --
// design/publisher-service.md §18.9's own named gap, closed by reusing
// the same security_compliance_approver role. The target is deliberately
// unreachable (127.0.0.1:1): a 403 must come from the role gate itself,
// before any attempt to call the target, so this doesn't need a working
// target to prove the gate exists. A request that gets *past* the gate
// (the approver's case) still fails further in -- with something other
// than 403 -- since there's nothing real to talk to; that's the real
// end-to-end proof cmd/opentea/productrelease_gui_test.go provides.
func TestCollectionDraftDecideRequiresApprovalRole(t *testing.T) {
	srv, r, adminUsername, adminPassword := newTestServer(t)
	target, err := r.CreateTarget(context.Background(), "Acme production", "http://127.0.0.1:1/publisher/v1", "s3cr3t")
	if err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}
	if _, err := r.CreateStaff(context.Background(), "carol", "hunter444", StaffRoleMember, StaffWorkflowRoleSecurityComplianceApprover); err != nil {
		t.Fatalf("CreateStaff (approver): %v", err)
	}

	admin := loggedInClient(t, srv, adminUsername, adminPassword)
	approver := loggedInClient(t, srv, "carol", "hunter444")

	path := srv.URL + "/targets/" + target.UUID + "/productReleases/some-uuid/collectionDraft/approve"
	decide := func(client *http.Client) *http.Response {
		req, err := http.NewRequest(http.MethodPost, path, strings.NewReader(url.Values{}.Encode()))
		if err != nil {
			t.Fatalf("new decide request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", srv.URL)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST approve: %v", err)
		}
		return resp
	}

	// An admin with no security_compliance_approver workflow role cannot
	// decide -- separation of duties, same as /approvals' own gate.
	if resp := decide(admin); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("admin approve: status=%d, want 403", resp.StatusCode)
	} else {
		_ = resp.Body.Close()
	}

	// The approver gets past the gate -- it then fails for an unrelated
	// reason (the target is unreachable), but specifically not 403.
	if resp := decide(approver); resp.StatusCode == http.StatusForbidden {
		t.Fatalf("approver approve: status=403, want the role gate to let this through")
	} else {
		_ = resp.Body.Close()
	}
}

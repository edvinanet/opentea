// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestApprovalWorkflowRoleGating is the core regression test for this
// scaffold: any authenticated staff member may create an approval
// request, but only a security_compliance_approver may decide one, and
// the requester can't decide their own request even if they hold that
// role (maker-checker, §10.1).
func TestApprovalWorkflowRoleGating(t *testing.T) {
	srv, r, adminUsername, adminPassword := newTestServer(t)
	target, err := r.CreateTarget(context.Background(), "Acme production", "https://tea.example.com/publisher/v1", "s3cr3t")
	if err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}
	if _, err := r.CreateStaff(context.Background(), "carol", "hunter444", StaffRoleMember, StaffWorkflowRoleSecurityComplianceApprover); err != nil {
		t.Fatalf("CreateStaff (approver): %v", err)
	}
	if _, err := r.CreateStaff(context.Background(), "bob", "hunter333", StaffRoleMember, StaffWorkflowRoleReleaseManager); err != nil {
		t.Fatalf("CreateStaff (requester): %v", err)
	}

	admin := loggedInClient(t, srv, adminUsername, adminPassword)
	approver := loggedInClient(t, srv, "carol", "hunter444")
	requester := loggedInClient(t, srv, "bob", "hunter333")

	createRequest := func(client *http.Client) *http.Response {
		form := url.Values{
			"targetUuid": {target.UUID}, "releaseKind": {ReleaseKindProduct},
			"releaseUuid": {"release-uuid-1"}, "notes": {"please review"},
		}
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/approvals", strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatalf("new create-approval request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", srv.URL)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST /approvals: %v", err)
		}
		return resp
	}

	// Any authenticated staff member (here: the requester, who holds no
	// admin/approver capability at all) can create a request.
	createResp := createRequest(requester)
	body, _ := io.ReadAll(createResp.Body)
	_ = createResp.Body.Close()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("requester POST /approvals: status=%d body=%s", createResp.StatusCode, body)
	}

	requests, err := r.ListApprovalRequests(context.Background(), "")
	if err != nil || len(requests) != 1 {
		t.Fatalf("ListApprovalRequests: requests=%+v err=%v", requests, err)
	}
	reqUUID := requests[0].UUID

	decide := func(client *http.Client) *http.Response {
		form := url.Values{"decision": {ApprovalDecisionApproved}}
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/approvals/"+reqUUID+"/decide", strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatalf("new decide request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", srv.URL)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST decide: %v", err)
		}
		return resp
	}

	// An admin account with no security_compliance_approver workflow role
	// cannot decide -- being StaffRoleAdmin does not imply approval
	// capability (separation of duties).
	if resp := decide(admin); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("admin decide: status=%d, want 403", resp.StatusCode)
	} else {
		_ = resp.Body.Close()
	}

	// The requester holds no approver workflow role either.
	if resp := decide(requester); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("requester decide: status=%d, want 403", resp.StatusCode)
	} else {
		_ = resp.Body.Close()
	}

	// The approver can decide.
	approveResp := decide(approver)
	approveBody, _ := io.ReadAll(approveResp.Body)
	_ = approveResp.Body.Close()
	if approveResp.StatusCode != http.StatusOK {
		t.Fatalf("approver decide: status=%d body=%s", approveResp.StatusCode, approveBody)
	}

	updated, err := r.GetApprovalRequest(context.Background(), reqUUID)
	if err != nil {
		t.Fatalf("GetApprovalRequest: %v", err)
	}
	if updated.Status != ApprovalStatusApproved {
		t.Fatalf("updated.Status = %q, want approved", updated.Status)
	}

	entries, err := r.ListAuditEntries(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListAuditEntries: %v", err)
	}
	var sawCreate, sawDecide bool
	for _, e := range entries {
		switch e.Operation {
		case "approval_request.create":
			sawCreate = true
		case "approval_request.decide":
			sawDecide = true
		}
	}
	if !sawCreate || !sawDecide {
		t.Fatalf("entries = %+v, want both approval_request.create and approval_request.decide", entries)
	}
}

// TestApprovalSelfApprovalDeniedEvenWithApproverRole confirms the
// maker-checker guard applies at the HTTP layer too: a requester who also
// holds the security_compliance_approver workflow role still can't decide
// their own request.
func TestApprovalSelfApprovalDeniedEvenWithApproverRole(t *testing.T) {
	srv, r, _, _ := newTestServer(t)
	target, err := r.CreateTarget(context.Background(), "Acme production", "https://tea.example.com/publisher/v1", "s3cr3t")
	if err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}
	if _, err := r.CreateStaff(context.Background(), "erin", "hunter666", StaffRoleMember, StaffWorkflowRoleSecurityComplianceApprover); err != nil {
		t.Fatalf("CreateStaff: %v", err)
	}
	erin := loggedInClient(t, srv, "erin", "hunter666")

	form := url.Values{"targetUuid": {target.UUID}, "releaseKind": {ReleaseKindProduct}, "releaseUuid": {"release-uuid-2"}}
	createReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/approvals", strings.NewReader(form.Encode()))
	createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createReq.Header.Set("Origin", srv.URL)
	createResp, err := erin.Do(createReq)
	if err != nil {
		t.Fatalf("POST /approvals: %v", err)
	}
	_ = createResp.Body.Close()

	requests, err := r.ListApprovalRequests(context.Background(), "")
	if err != nil || len(requests) != 1 {
		t.Fatalf("ListApprovalRequests: requests=%+v err=%v", requests, err)
	}

	decideForm := url.Values{"decision": {ApprovalDecisionApproved}}
	decideReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/approvals/"+requests[0].UUID+"/decide", strings.NewReader(decideForm.Encode()))
	decideReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	decideReq.Header.Set("Origin", srv.URL)
	decideResp, err := erin.Do(decideReq)
	if err != nil {
		t.Fatalf("POST decide: %v", err)
	}
	body, _ := io.ReadAll(decideResp.Body)
	_ = decideResp.Body.Close()
	// requireApprovalRole lets erin through (she holds the role); the
	// rejection comes from RecordApprovalDecision's own maker-checker
	// check, surfaced as a rerendered 200 page with an error message, not
	// a 403 -- self-approval is a business-rule violation, not an
	// authorization failure.
	if decideResp.StatusCode != http.StatusOK || !strings.Contains(string(body), "cannot decide your own") {
		t.Fatalf("decide (self-approval): status=%d body=%s", decideResp.StatusCode, body)
	}

	unchanged, err := r.GetApprovalRequest(context.Background(), requests[0].UUID)
	if err != nil {
		t.Fatalf("GetApprovalRequest: %v", err)
	}
	if unchanged.Status != ApprovalStatusPending {
		t.Fatalf("unchanged.Status = %q, want still pending", unchanged.Status)
	}
}

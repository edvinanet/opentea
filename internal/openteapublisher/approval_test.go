// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"context"
	"testing"
)

func mustCreateTargetForApproval(t *testing.T, r *Repo) Target {
	t.Helper()
	target, err := r.CreateTarget(context.Background(), "Acme production", "https://tea.example.com/publisher/v1", "s3cr3t")
	if err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}
	return target
}

func TestCreateGetListApprovalRequest(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	target := mustCreateTargetForApproval(t, r)
	requester, err := r.CreateStaff(ctx, "alice", "hunter222", StaffRoleMember, StaffWorkflowRoleReleaseManager)
	if err != nil {
		t.Fatalf("CreateStaff: %v", err)
	}

	created, err := r.CreateApprovalRequest(ctx, ApprovalRequestInput{
		TargetUUID: target.UUID, ReleaseKind: ReleaseKindProduct, ReleaseUUID: "release-uuid-1",
		Notes: "please approve", RequestedBy: requester.UUID,
	})
	if err != nil {
		t.Fatalf("CreateApprovalRequest: %v", err)
	}
	if created.Status != ApprovalStatusPending || created.RequiredApprovals != 1 {
		t.Fatalf("created = %+v, want pending/1", created)
	}

	got, err := r.GetApprovalRequest(ctx, created.UUID)
	if err != nil {
		t.Fatalf("GetApprovalRequest: %v", err)
	}
	if got.ReleaseUUID != "release-uuid-1" || got.RequestedBy != requester.UUID {
		t.Fatalf("got = %+v", got)
	}
	if _, err := r.GetApprovalRequest(ctx, "00000000-0000-4000-8000-000000000000"); err != ErrNotFound {
		t.Fatalf("GetApprovalRequest (unknown): err = %v, want ErrNotFound", err)
	}

	list, err := r.ListApprovalRequests(ctx, "")
	if err != nil {
		t.Fatalf("ListApprovalRequests: %v", err)
	}
	if len(list) != 1 || list[0].UUID != created.UUID {
		t.Fatalf("list = %+v", list)
	}

	scoped, err := r.ListApprovalRequests(ctx, target.UUID)
	if err != nil {
		t.Fatalf("ListApprovalRequests (scoped): %v", err)
	}
	if len(scoped) != 1 {
		t.Fatalf("scoped = %+v", scoped)
	}
	if empty, err := r.ListApprovalRequests(ctx, "00000000-0000-4000-8000-000000000000"); err != nil || len(empty) != 0 {
		t.Fatalf("ListApprovalRequests (unknown target): list=%+v err=%v", empty, err)
	}
}

func TestCreateApprovalRequestInvalidReleaseKind(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	target := mustCreateTargetForApproval(t, r)

	if _, err := r.CreateApprovalRequest(ctx, ApprovalRequestInput{
		TargetUUID: target.UUID, ReleaseKind: "somethingElse", ReleaseUUID: "release-uuid-1",
	}); err != ErrInvalidReleaseKind {
		t.Fatalf("err = %v, want ErrInvalidReleaseKind", err)
	}
}

func TestRecordApprovalDecisionSelfApprovalRejected(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	target := mustCreateTargetForApproval(t, r)
	requester, err := r.CreateStaff(ctx, "alice", "hunter222", StaffRoleMember, StaffWorkflowRoleSecurityComplianceApprover)
	if err != nil {
		t.Fatalf("CreateStaff: %v", err)
	}
	req, err := r.CreateApprovalRequest(ctx, ApprovalRequestInput{
		TargetUUID: target.UUID, ReleaseKind: ReleaseKindProduct, ReleaseUUID: "release-uuid-1", RequestedBy: requester.UUID,
	})
	if err != nil {
		t.Fatalf("CreateApprovalRequest: %v", err)
	}

	if _, err := r.RecordApprovalDecision(ctx, req.UUID, requester.UUID, ApprovalDecisionApproved, ""); err != ErrSelfApproval {
		t.Fatalf("err = %v, want ErrSelfApproval", err)
	}
}

func TestRecordApprovalDecisionRejectionClosesImmediately(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	target := mustCreateTargetForApproval(t, r)
	requester, err := r.CreateStaff(ctx, "alice", "hunter222", StaffRoleMember, StaffWorkflowRoleReleaseManager)
	if err != nil {
		t.Fatalf("CreateStaff (requester): %v", err)
	}
	approver, err := r.CreateStaff(ctx, "carol", "hunter444", StaffRoleMember, StaffWorkflowRoleSecurityComplianceApprover)
	if err != nil {
		t.Fatalf("CreateStaff (approver): %v", err)
	}
	req, err := r.CreateApprovalRequest(ctx, ApprovalRequestInput{
		TargetUUID: target.UUID, ReleaseKind: ReleaseKindProduct, ReleaseUUID: "release-uuid-1", RequestedBy: requester.UUID,
	})
	if err != nil {
		t.Fatalf("CreateApprovalRequest: %v", err)
	}

	updated, err := r.RecordApprovalDecision(ctx, req.UUID, approver.UUID, ApprovalDecisionRejected, "no")
	if err != nil {
		t.Fatalf("RecordApprovalDecision: %v", err)
	}
	if updated.Status != ApprovalStatusRejected {
		t.Fatalf("updated.Status = %q, want rejected", updated.Status)
	}

	// A closed request can't be decided again.
	if _, err := r.RecordApprovalDecision(ctx, req.UUID, approver.UUID, ApprovalDecisionApproved, ""); err != ErrRequestNotPending {
		t.Fatalf("err = %v, want ErrRequestNotPending", err)
	}

	decisions, err := r.ListApprovalDecisions(ctx, req.UUID)
	if err != nil {
		t.Fatalf("ListApprovalDecisions: %v", err)
	}
	if len(decisions) != 1 || decisions[0].Decision != ApprovalDecisionRejected || decisions[0].StaffUUID != approver.UUID {
		t.Fatalf("decisions = %+v", decisions)
	}
}

func TestRecordApprovalDecisionPartialThenCompleteApproval(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	target := mustCreateTargetForApproval(t, r)
	requester, err := r.CreateStaff(ctx, "alice", "hunter222", StaffRoleMember, StaffWorkflowRoleReleaseManager)
	if err != nil {
		t.Fatalf("CreateStaff (requester): %v", err)
	}
	approver1, err := r.CreateStaff(ctx, "carol", "hunter444", StaffRoleMember, StaffWorkflowRoleSecurityComplianceApprover)
	if err != nil {
		t.Fatalf("CreateStaff (approver1): %v", err)
	}
	approver2, err := r.CreateStaff(ctx, "dave", "hunter555", StaffRoleMember, StaffWorkflowRoleSecurityComplianceApprover)
	if err != nil {
		t.Fatalf("CreateStaff (approver2): %v", err)
	}
	req, err := r.CreateApprovalRequest(ctx, ApprovalRequestInput{
		TargetUUID: target.UUID, ReleaseKind: ReleaseKindProduct, ReleaseUUID: "release-uuid-1",
		RequestedBy: requester.UUID, RequiredApprovals: 2,
	})
	if err != nil {
		t.Fatalf("CreateApprovalRequest: %v", err)
	}

	afterFirst, err := r.RecordApprovalDecision(ctx, req.UUID, approver1.UUID, ApprovalDecisionApproved, "")
	if err != nil {
		t.Fatalf("RecordApprovalDecision (1st): %v", err)
	}
	if afterFirst.Status != ApprovalStatusPending {
		t.Fatalf("afterFirst.Status = %q, want pending (1 of 2)", afterFirst.Status)
	}

	// The same approver deciding twice must not double-count toward
	// required_approvals.
	if _, err := r.RecordApprovalDecision(ctx, req.UUID, approver1.UUID, ApprovalDecisionApproved, ""); err != nil {
		t.Fatalf("RecordApprovalDecision (1st approver again, should be allowed since the request is still pending): %v", err)
	}

	afterSecond, err := r.RecordApprovalDecision(ctx, req.UUID, approver2.UUID, ApprovalDecisionApproved, "")
	if err != nil {
		t.Fatalf("RecordApprovalDecision (2nd distinct approver): %v", err)
	}
	if afterSecond.Status != ApprovalStatusApproved {
		t.Fatalf("afterSecond.Status = %q, want approved (2 distinct approvers reached)", afterSecond.Status)
	}

	decisions, err := r.ListApprovalDecisions(ctx, req.UUID)
	if err != nil {
		t.Fatalf("ListApprovalDecisions: %v", err)
	}
	if len(decisions) != 3 {
		t.Fatalf("decisions = %+v, want 3 recorded decisions", decisions)
	}
}

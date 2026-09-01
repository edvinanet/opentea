// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/oej/opentea/internal/idgen"
)

// Release kinds an approval request can reference -- matches pkg/tea's own
// productRelease/componentRelease vocabulary.
const (
	ReleaseKindProduct   = "productRelease"
	ReleaseKindComponent = "componentRelease"
)

// Approval request statuses.
const (
	ApprovalStatusPending  = "pending"
	ApprovalStatusApproved = "approved"
	ApprovalStatusRejected = "rejected"
)

// Approval decision values.
const (
	ApprovalDecisionApproved = "approved"
	ApprovalDecisionRejected = "rejected"
)

var (
	// ErrInvalidReleaseKind is returned by CreateApprovalRequest when
	// releaseKind isn't ReleaseKindProduct/Component.
	ErrInvalidReleaseKind = errors.New("openteapublisher: release kind must be \"productRelease\" or \"componentRelease\"")
	// ErrSelfApproval is returned by RecordApprovalDecision when the
	// deciding staff account is the same one that created the request --
	// mirrors internal/publisher's own maker-checker error, and §10.1's
	// own text: "whoever approves a commit must be a different verified
	// identity than whoever built the draft."
	ErrSelfApproval = errors.New("openteapublisher: the requester cannot decide their own approval request")
	// ErrRequestNotPending is returned by RecordApprovalDecision when the
	// request has already been approved or rejected.
	ErrRequestNotPending = errors.New("openteapublisher: this approval request is already closed")
)

// ApprovalRequest is one internal business-approval request against a
// target's release (design/publisher-service.md §18.8). ReleaseUUID is
// stored and displayed as-is -- no live lookup against the target to
// resolve it to a human-readable label in this pass; see the migration's
// own comment for why.
type ApprovalRequest struct {
	UUID              string
	TargetUUID        string
	ReleaseKind       string
	ReleaseUUID       string
	Notes             string
	RequestedBy       string
	RequiredApprovals int
	Status            string
	CreatedAt         time.Time
}

// ApprovalRequestInput carries what CreateApprovalRequest needs.
type ApprovalRequestInput struct {
	TargetUUID        string
	ReleaseKind       string
	ReleaseUUID       string
	Notes             string
	RequestedBy       string
	RequiredApprovals int
}

// ApprovalDecision is one approve/reject decision recorded against an
// ApprovalRequest.
type ApprovalDecision struct {
	ID          int64
	RequestUUID string
	StaffUUID   string
	Decision    string
	Comment     string
	DecidedAt   time.Time
}

// CreateApprovalRequest creates a new pending approval request. No role
// gate here -- either a release manager or component maintainer might
// reasonably request one; nothing in this pass enforces who may request,
// only who may decide (see RecordApprovalDecision and
// requireApprovalRole, auth_middleware.go). RequiredApprovals defaults to
// 1 if zero.
func (r *Repo) CreateApprovalRequest(ctx context.Context, in ApprovalRequestInput) (ApprovalRequest, error) {
	if in.ReleaseKind != ReleaseKindProduct && in.ReleaseKind != ReleaseKindComponent {
		return ApprovalRequest{}, ErrInvalidReleaseKind
	}
	requiredApprovals := in.RequiredApprovals
	if requiredApprovals == 0 {
		requiredApprovals = 1
	}

	uuid := idgen.New()
	if _, err := r.conn().ExecContext(ctx,
		`INSERT INTO approval_request (uuid, target_uuid, release_kind, release_uuid, notes, requested_by, required_approvals)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid, in.TargetUUID, in.ReleaseKind, in.ReleaseUUID, nullIfEmpty(in.Notes), nullIfEmpty(in.RequestedBy), requiredApprovals,
	); err != nil {
		return ApprovalRequest{}, err
	}
	return r.GetApprovalRequest(ctx, uuid)
}

// GetApprovalRequest fetches an approval request by UUID. Returns
// ErrNotFound if uuid doesn't exist.
func (r *Repo) GetApprovalRequest(ctx context.Context, uuid string) (ApprovalRequest, error) {
	var req ApprovalRequest
	var notes, requestedBy sql.NullString
	var createdAt string
	err := r.conn().QueryRowContext(ctx,
		`SELECT uuid, target_uuid, release_kind, release_uuid, notes, requested_by, required_approvals, status, created_at
		 FROM approval_request WHERE uuid = ?`, uuid,
	).Scan(&req.UUID, &req.TargetUUID, &req.ReleaseKind, &req.ReleaseUUID, &notes, &requestedBy, &req.RequiredApprovals, &req.Status, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ApprovalRequest{}, ErrNotFound
	}
	if err != nil {
		return ApprovalRequest{}, err
	}
	req.Notes = notes.String
	req.RequestedBy = requestedBy.String
	created, err := parseTime(createdAt)
	if err != nil {
		return ApprovalRequest{}, err
	}
	req.CreatedAt = created
	return req, nil
}

// ListApprovalRequests returns approval requests newest-first. An empty
// targetUUID lists every request across all targets; a non-empty one
// scopes to that target. Unpaginated -- matches ListTargets/ListStaff's
// own "list everyone at once" convention for this app's current size.
func (r *Repo) ListApprovalRequests(ctx context.Context, targetUUID string) ([]ApprovalRequest, error) {
	query := `SELECT uuid, target_uuid, release_kind, release_uuid, notes, requested_by, required_approvals, status, created_at
	          FROM approval_request`
	args := []any{}
	if targetUUID != "" {
		query += ` WHERE target_uuid = ?`
		args = append(args, targetUUID)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := r.conn().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []ApprovalRequest{}
	for rows.Next() {
		var req ApprovalRequest
		var notes, requestedBy sql.NullString
		var createdAt string
		if err := rows.Scan(&req.UUID, &req.TargetUUID, &req.ReleaseKind, &req.ReleaseUUID, &notes, &requestedBy, &req.RequiredApprovals, &req.Status, &createdAt); err != nil {
			return nil, err
		}
		req.Notes = notes.String
		req.RequestedBy = requestedBy.String
		created, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		req.CreatedAt = created
		out = append(out, req)
	}
	return out, rows.Err()
}

// ListApprovalDecisions returns every decision recorded against
// requestUUID, oldest first.
func (r *Repo) ListApprovalDecisions(ctx context.Context, requestUUID string) ([]ApprovalDecision, error) {
	rows, err := r.conn().QueryContext(ctx,
		`SELECT id, request_uuid, staff_uuid, decision, comment, decided_at
		 FROM approval_decision WHERE request_uuid = ? ORDER BY id ASC`, requestUUID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []ApprovalDecision{}
	for rows.Next() {
		var d ApprovalDecision
		var staffUUID, comment sql.NullString
		var decidedAt string
		if err := rows.Scan(&d.ID, &d.RequestUUID, &staffUUID, &d.Decision, &comment, &decidedAt); err != nil {
			return nil, err
		}
		d.StaffUUID = staffUUID.String
		d.Comment = comment.String
		decided, err := parseTime(decidedAt)
		if err != nil {
			return nil, err
		}
		d.DecidedAt = decided
		out = append(out, d)
	}
	return out, rows.Err()
}

// RecordApprovalDecision records staffUUID's decision ("approved" or
// "rejected") against requestUUID and returns the request's state after
// applying it. Returns ErrNotFound if requestUUID doesn't exist,
// ErrRequestNotPending if the request is already closed, or
// ErrSelfApproval if staffUUID is the request's own requester (maker-
// checker, §10.1). A rejection closes the request immediately; an
// approval closes it once this staff account's approving decisions (this
// is the first, since a given staff account may only decide once --
// enforced by the caller having no prior decision from that account
// counted twice, see the DISTINCT count below) reach RequiredApprovals,
// otherwise the request stays pending.
func (r *Repo) RecordApprovalDecision(ctx context.Context, requestUUID, staffUUID, decision, comment string) (ApprovalRequest, error) {
	req, err := r.GetApprovalRequest(ctx, requestUUID)
	if err != nil {
		return ApprovalRequest{}, err
	}
	if req.Status != ApprovalStatusPending {
		return ApprovalRequest{}, ErrRequestNotPending
	}
	if staffUUID != "" && staffUUID == req.RequestedBy {
		return ApprovalRequest{}, ErrSelfApproval
	}

	if _, err := r.conn().ExecContext(ctx,
		`INSERT INTO approval_decision (request_uuid, staff_uuid, decision, comment) VALUES (?, ?, ?, ?)`,
		requestUUID, nullIfEmpty(staffUUID), decision, nullIfEmpty(comment),
	); err != nil {
		return ApprovalRequest{}, err
	}

	newStatus := ApprovalStatusPending
	if decision == ApprovalDecisionRejected {
		newStatus = ApprovalStatusRejected
	} else {
		var approvingCount int
		if err := r.conn().QueryRowContext(ctx,
			`SELECT COUNT(DISTINCT staff_uuid) FROM approval_decision WHERE request_uuid = ? AND decision = ?`,
			requestUUID, ApprovalDecisionApproved,
		).Scan(&approvingCount); err != nil {
			return ApprovalRequest{}, err
		}
		if approvingCount >= req.RequiredApprovals {
			newStatus = ApprovalStatusApproved
		}
	}

	if newStatus != ApprovalStatusPending {
		if _, err := r.conn().ExecContext(ctx, `UPDATE approval_request SET status = ? WHERE uuid = ?`, newStatus, requestUUID); err != nil {
			return ApprovalRequest{}, err
		}
	}

	return r.GetApprovalRequest(ctx, requestUUID)
}

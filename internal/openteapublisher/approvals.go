// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/oej/opentea/internal/httpx"
)

// auditableApprovalRequest is ApprovalRequest as recorded in the audit
// log -- no field needs redacting (unlike auditableTarget's BearerToken),
// this is just a named type for symmetry with targets.go/staffmanagement.go's
// own convention.
type auditableApprovalRequest struct {
	UUID              string
	TargetUUID        string
	ReleaseKind       string
	ReleaseUUID       string
	Notes             string
	RequestedBy       string
	RequiredApprovals int
	Status            string
}

func auditableRequest(req ApprovalRequest) auditableApprovalRequest {
	return auditableApprovalRequest{
		UUID: req.UUID, TargetUUID: req.TargetUUID, ReleaseKind: req.ReleaseKind,
		ReleaseUUID: req.ReleaseUUID, Notes: req.Notes, RequestedBy: req.RequestedBy,
		RequiredApprovals: req.RequiredApprovals, Status: req.Status,
	}
}

func (s *Server) approvalsPage(w http.ResponseWriter, r *http.Request, staff Staff) {
	requests, err := s.repo.ListApprovalRequests(r.Context(), "")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	targets, err := s.repo.ListTargets(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.renderAuthenticated(w, "approvals", pageData{Staff: staff, ApprovalRequests: requests, Targets: targets})
}

func (s *Server) createApprovalRequestForm(w http.ResponseWriter, r *http.Request, staff Staff) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/approvals", http.StatusSeeOther)
		return
	}
	targetUUID := r.PostFormValue("targetUuid")
	releaseKind := r.PostFormValue("releaseKind")
	releaseUUID := r.PostFormValue("releaseUuid")
	notes := r.PostFormValue("notes")
	requiredApprovals := 1
	if v := r.PostFormValue("requiredApprovals"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			s.rerenderApprovalsWithError(w, r, staff, "required approvals must be a positive whole number")
			return
		}
		requiredApprovals = n
	}

	if targetUUID == "" || releaseUUID == "" {
		s.rerenderApprovalsWithError(w, r, staff, "target and release are required")
		return
	}

	var created ApprovalRequest
	err := s.repo.WithTx(r.Context(), func(tx *Repo) error {
		var err error
		created, err = tx.CreateApprovalRequest(r.Context(), ApprovalRequestInput{
			TargetUUID: targetUUID, ReleaseKind: releaseKind, ReleaseUUID: releaseUUID,
			Notes: notes, RequestedBy: staff.UUID, RequiredApprovals: requiredApprovals,
		})
		if err != nil {
			return err
		}
		return tx.RecordAudit(r.Context(), AuditInput{
			StaffUUID: staff.UUID, RequestID: httpx.RequestID(r.Context()),
			Operation: "approval_request.create", TargetType: "approval_request", TargetID: created.UUID,
			Resulting: auditableRequest(created),
		})
	})
	if errors.Is(err, ErrInvalidReleaseKind) {
		s.rerenderApprovalsWithError(w, r, staff, err.Error())
		return
	}
	if err != nil {
		s.rerenderApprovalsWithError(w, r, staff, "internal error, please try again")
		return
	}
	http.Redirect(w, r, "/approvals", http.StatusSeeOther)
}

func (s *Server) decideApprovalRequestForm(w http.ResponseWriter, r *http.Request, staff Staff) {
	uuid := r.PathValue("uuid")
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/approvals", http.StatusSeeOther)
		return
	}
	decision := r.PostFormValue("decision")
	comment := r.PostFormValue("comment")

	if decision != ApprovalDecisionApproved && decision != ApprovalDecisionRejected {
		s.rerenderApprovalsWithError(w, r, staff, "decision must be \"approved\" or \"rejected\"")
		return
	}

	existing, err := s.repo.GetApprovalRequest(r.Context(), uuid)
	if errors.Is(err, ErrNotFound) {
		http.Redirect(w, r, "/approvals", http.StatusSeeOther)
		return
	}
	if err != nil {
		s.rerenderApprovalsWithError(w, r, staff, "internal error, please try again")
		return
	}

	var updated ApprovalRequest
	err = s.repo.WithTx(r.Context(), func(tx *Repo) error {
		var err error
		updated, err = tx.RecordApprovalDecision(r.Context(), uuid, staff.UUID, decision, comment)
		if err != nil {
			return err
		}
		return tx.RecordAudit(r.Context(), AuditInput{
			StaffUUID: staff.UUID, RequestID: httpx.RequestID(r.Context()),
			Operation: "approval_request.decide", TargetType: "approval_request", TargetID: uuid,
			Previous: auditableRequest(existing), Resulting: auditableRequest(updated),
		})
	})
	if errors.Is(err, ErrSelfApproval) {
		s.rerenderApprovalsWithError(w, r, staff, "you cannot decide your own approval request")
		return
	}
	if errors.Is(err, ErrRequestNotPending) {
		s.rerenderApprovalsWithError(w, r, staff, "this approval request is already closed")
		return
	}
	if err != nil {
		s.rerenderApprovalsWithError(w, r, staff, "internal error, please try again")
		return
	}
	http.Redirect(w, r, "/approvals", http.StatusSeeOther)
}

func (s *Server) rerenderApprovalsWithError(w http.ResponseWriter, r *http.Request, staff Staff, message string) {
	requests, err := s.repo.ListApprovalRequests(r.Context(), "")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	targets, err := s.repo.ListTargets(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.renderAuthenticated(w, "approvals", pageData{Staff: staff, ApprovalRequests: requests, Targets: targets, Error: message})
}

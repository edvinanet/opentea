// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import "net/http"

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /login", s.loginForm)
	mux.HandleFunc("POST /login", s.requireSameOrigin(s.loginSubmit))
	mux.HandleFunc("POST /logout", s.requireSameOrigin(s.logout))

	// {$} restricts this to an exact match on / -- otherwise, as a prefix
	// pattern, it would also catch unknown sub-paths as a side effect.
	mux.HandleFunc("GET /{$}", s.requireSession(s.dashboard))

	// Target management is admin-only: a target's bearer_token is a real,
	// usable credential (§18.11), not a resource any logged-in staff
	// member should be able to add/remove.
	mux.HandleFunc("POST /targets", s.requireRole(StaffRoleAdmin, s.createTargetForm))
	mux.HandleFunc("POST /targets/{uuid}/delete", s.requireRole(StaffRoleAdmin, s.deleteTargetForm))

	// Staff management is admin-only too.
	mux.HandleFunc("GET /staff", s.requireRole(StaffRoleAdmin, s.staffPage))
	mux.HandleFunc("POST /staff", s.requireRole(StaffRoleAdmin, s.createStaffForm))
	mux.HandleFunc("POST /staff/{uuid}/delete", s.requireRole(StaffRoleAdmin, s.deleteStaffForm))

	// Any authenticated staff member may view requests and create one
	// (mirrors "release manager or component maintainer" not being
	// admin-only, §10.1) -- only a security_compliance_approver may
	// decide one (separation of duties, requireApprovalRole).
	mux.HandleFunc("GET /approvals", s.requireSession(s.approvalsPage))
	mux.HandleFunc("POST /approvals", s.requireSession(s.createApprovalRequestForm))
	mux.HandleFunc("POST /approvals/{uuid}/decide", s.requireApprovalRole(s.decideApprovalRequestForm))

	// CI/CD credential management is admin-only too -- a cicd_credential is
	// a real, usable secret (§18.11), same rationale as target management.
	mux.HandleFunc("GET /cicd-credentials", s.requireRole(StaffRoleAdmin, s.cicdCredentialsPage))
	mux.HandleFunc("POST /cicd-credentials", s.requireRole(StaffRoleAdmin, s.createCICDCredentialForm))
	mux.HandleFunc("POST /cicd-credentials/{uuid}/revoke", s.requireRole(StaffRoleAdmin, s.revokeCICDCredentialForm))
}

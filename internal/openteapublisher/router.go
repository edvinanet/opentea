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

	// Critical-path publishing workflow (§18.3/§18.7/§18.9/§18.10):
	// products/releases, collection-draft assembly, protocol-level
	// approval, and Sign & Publish -- every route target-scoped under
	// /targets/{targetUuid}/..., reached via the dashboard's per-target
	// "Products" link rather than a global nav item (§18.1). Any
	// authenticated staff member may do everything here except decide a
	// protocol-level approve/reject, which requireApprovalRole reserves
	// for a security_compliance_approver (§18.9's own named gap, closed
	// here by reusing that same role).
	mux.HandleFunc("GET /targets/{targetUuid}/products", s.requireSession(s.productsPage))
	mux.HandleFunc("POST /targets/{targetUuid}/products", s.requireSession(s.createProductForm))
	mux.HandleFunc("GET /targets/{targetUuid}/products/{uuid}", s.requireSession(s.productPage))
	mux.HandleFunc("POST /targets/{targetUuid}/products/{uuid}/releases", s.requireSession(s.createProductReleaseForm))

	mux.HandleFunc("GET /targets/{targetUuid}/productReleases/{uuid}", s.requireSession(s.productReleasePage))
	mux.HandleFunc("POST /targets/{targetUuid}/productReleases/{uuid}/collectionDraft", s.requireSession(s.updateCollectionDraftForm))
	mux.HandleFunc("POST /targets/{targetUuid}/productReleases/{uuid}/collectionDraft/approve", s.requireApprovalRole(s.decideCollectionDraftForm(true)))
	mux.HandleFunc("POST /targets/{targetUuid}/productReleases/{uuid}/collectionDraft/reject", s.requireApprovalRole(s.decideCollectionDraftForm(false)))
	mux.HandleFunc("POST /targets/{targetUuid}/productReleases/{uuid}/collectionDraft/signAndPublish", s.requireSession(s.signAndPublishForm))
}

// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"log/slog"
	"net/http"

	"github.com/oej/opentea/pkg/tea"
	"github.com/oej/opentea/pkg/teapublisher"
)

// pageData is the single data type passed to every template -- only the
// fields relevant to a given page are populated. Mirrors
// internal/webadmin/render.go's own pageData convention.
type pageData struct {
	Staff Staff
	Error string

	Targets []Target

	StaffList []Staff

	ApprovalRequests []ApprovalRequest

	CICDCredentials  []CICDCredential
	TargetLabels     map[string]string
	CreatedCICDToken string

	// Target-scoped critical-path publishing screens (products.go,
	// collectiondraft.go, design/publisher-service.md §18.3/§18.7/§18.9/
	// §18.10) -- Target is the owning target for all of these.
	Target          Target
	Products        []tea.Product
	Product         tea.Product
	ProductReleases []tea.ProductRelease
	ProductRelease  tea.ProductReleaseWithCollection
	Draft           teapublisher.CollectionDraft
	// CanApprove reports whether the logged-in staff member's workflow
	// role permits deciding a protocol-level collection-draft approval
	// (requireApprovalRole's own check, surfaced here so the template can
	// show/hide the Approve/Reject forms instead of rendering them only
	// to have the POST 403).
	CanApprove bool
	// IsDrafter reports whether the logged-in staff member is the
	// draft's own DraftedBy -- a UX courtesy to hide the Approve/Reject
	// forms for the obvious self-approval case, mirroring design/
	// publisher-service.md §18.9's own instruction; the target's 403 is
	// the real enforcement regardless.
	IsDrafter bool
}

func (s *Server) renderAuthenticated(w http.ResponseWriter, page string, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates[page].ExecuteTemplate(w, "layout", data); err != nil {
		slog.Error("render template failed", "page", page, "error", err)
	}
}

func (s *Server) renderLogin(w http.ResponseWriter, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates["login"].ExecuteTemplate(w, "login", data); err != nil {
		slog.Error("render login template failed", "error", err)
	}
}

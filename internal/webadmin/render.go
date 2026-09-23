// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package webadmin

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/pkg/tea"
)

// pageData is the single data type passed to every template -- only the
// fields relevant to a given page are populated. Small enough a GUI that
// per-page types would be pure overhead.
type pageData struct {
	User    model.User
	Error   string
	RootURL string
	OrgName string

	// APIBasePath mirrors config.Config.APIBasePath -- the path prefix the
	// consumer API is actually served under ("/tea/v1" by default). Used
	// for any link/example this GUI shows pointing at that API, so they
	// stay correct on a deployment that's changed TEA_API_BASE_PATH.
	APIBasePath string

	// TrustArchitectureEnabled mirrors config.Config.TrustArchitectureEnabled
	// -- shown as a "Trusted TEA"/"Default TEA" badge in layout.html's nav,
	// on every page, set below alongside OrgName/RootURL rather than by
	// each handler individually.
	TrustArchitectureEnabled bool

	Stats model.Stats

	Users []model.User

	TokenInfo *time.Time // nil if the user has no API key yet
	NewKeyID  string     // set only right after generation; safe to show/copy indefinitely otherwise, but this page only ever displays the freshly-generated one
	NewSecret string     // set only right after generation; shown once

	Products        []tea.Product
	Product         tea.Product
	ProductCLE      tea.CLE
	ProductReleases []tea.ProductRelease

	ProductRelease           tea.ProductRelease
	ProductReleaseCLE        tea.CLE
	ProductReleaseComponents []componentRefView // linked components, resolved to a name

	// Collections is shared by both the productRelease and componentRelease
	// detail pages -- each collectionView already carries its own
	// artifactViews. A view type, not []tea.Collection directly, so each
	// item can carry its resolved evidence-bundle badge (see evidence.go)
	// without touching tea.Collection/tea.Artifact's own EvidenceBundle
	// fields, which stay unpopulated by repo Get/List calls in this phase.
	Collections []collectionView

	Components        []tea.Component
	Component         tea.Component
	ComponentCLE      tea.CLE
	ComponentReleases []tea.ComponentRelease

	ComponentRelease    tea.ComponentRelease
	ComponentReleaseCLE tea.CLE
}

func (s *Server) renderAuthenticated(w http.ResponseWriter, page string, data pageData) {
	data.RootURL = s.cfg.RootURL
	data.OrgName = s.cfg.OrgName
	data.APIBasePath = s.cfg.APIBasePath
	data.TrustArchitectureEnabled = s.cfg.TrustArchitectureEnabled
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates[page].ExecuteTemplate(w, "layout", data); err != nil {
		slog.Error("render template failed", "page", page, "error", err)
	}
}

func (s *Server) renderLogin(w http.ResponseWriter, data pageData) {
	data.OrgName = s.cfg.OrgName
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates["login"].ExecuteTemplate(w, "login", data); err != nil {
		slog.Error("render login template failed", "error", err)
	}
}

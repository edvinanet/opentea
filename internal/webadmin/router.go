// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package webadmin

import (
	"net/http"

	"github.com/oej/opentea/internal/model"
)

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin/ui/login", s.loginForm)
	mux.HandleFunc("POST /admin/ui/login", s.requireSameOrigin(s.loginSubmit))
	mux.HandleFunc("POST /admin/ui/logout", s.requireSameOrigin(s.logout))

	// {$} restricts this to an exact match on /admin/ui/ -- otherwise, as a
	// prefix pattern, it would also catch unknown sub-paths as a side effect.
	mux.HandleFunc("GET /admin/ui/{$}", s.requireRole(model.RoleConsumer, s.dashboard))

	mux.HandleFunc("GET /admin/ui/users", s.requireRole(model.RoleAdmin, s.usersPage))
	mux.HandleFunc("POST /admin/ui/users", s.requireRole(model.RoleAdmin, s.createUserForm))
	mux.HandleFunc("POST /admin/ui/users/{uuid}/delete", s.requireRole(model.RoleAdmin, s.deleteUserForm))

	mux.HandleFunc("GET /admin/ui/token", s.requireRole(model.RoleConsumer, s.tokenPage))
	mux.HandleFunc("POST /admin/ui/token/generate", s.requireRole(model.RoleConsumer, s.generateToken))

	// Read-only data browsing -- same role level as the dashboard/token
	// pages. Collections/artifacts have no owner-less list, so they're
	// rendered inline on the owning release's detail page rather than
	// getting their own routes here.
	mux.HandleFunc("GET /admin/ui/products", s.requireRole(model.RoleConsumer, s.productsPage))
	mux.HandleFunc("GET /admin/ui/products/{uuid}", s.requireRole(model.RoleConsumer, s.productDetailPage))
	mux.HandleFunc("GET /admin/ui/productReleases/{uuid}", s.requireRole(model.RoleConsumer, s.productReleaseDetailPage))
	mux.HandleFunc("GET /admin/ui/components", s.requireRole(model.RoleConsumer, s.componentsPage))
	mux.HandleFunc("GET /admin/ui/components/{uuid}", s.requireRole(model.RoleConsumer, s.componentDetailPage))
	mux.HandleFunc("GET /admin/ui/componentReleases/{uuid}", s.requireRole(model.RoleConsumer, s.componentReleaseDetailPage))
}

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
}

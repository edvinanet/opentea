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

	mux.HandleFunc("POST /targets", s.requireSession(s.createTargetForm))
	mux.HandleFunc("POST /targets/{uuid}/delete", s.requireSession(s.deleteTargetForm))
}

// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/authn"
)

// staffHandler is like http.HandlerFunc but also receives the resolved
// staff account -- every authenticated page needs it (at least to render
// the nav's logout button).
type staffHandler func(w http.ResponseWriter, r *http.Request, staff Staff)

// sessionUser resolves the logged-in staff account from the session
// cookie on r, if there is a valid, unexpired session -- mirrors
// internal/authn.SessionUser's own contract (a missing/unknown/expired
// cookie is silently treated as anonymous), against this package's own
// Repo/Staff types instead of internal/repo's, which are hard-wired to
// opentea's own database.
func (s *Server) sessionUser(r *http.Request) (Staff, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return Staff{}, false
	}
	staff, err := s.repo.GetSessionStaff(r.Context(), cookie.Value)
	if errors.Is(err, ErrNotFound) {
		return Staff{}, false
	}
	if err != nil {
		return Staff{}, false
	}
	return staff, true
}

// requireSession wraps next so it only runs for a request with a valid
// session -- otherwise it redirects to the login page (no session) or
// rejects a cross-origin state-changing request (CSRF defense-in-depth,
// internal/authn.SameOrigin -- generic, no opentea-specific type
// dependency, reused as-is).
func (s *Server) requireSession(next staffHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		staff, ok := s.sessionUser(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if !authn.SameOrigin(r, s.cfg.RootURL) {
			http.Error(w, "Forbidden: cross-origin request rejected.", http.StatusForbidden)
			return
		}
		next(w, r, staff)
	}
}

// requireSameOrigin applies authn.SameOrigin's CSRF check alone, for
// routes reachable without a session (login) that requireSession can't
// cover since it requires an authenticated staff account first.
func (s *Server) requireSameOrigin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authn.SameOrigin(r, s.cfg.RootURL) {
			http.Error(w, "Forbidden: cross-origin request rejected.", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

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
// dependency, reused as-is). Any authenticated staff member satisfies
// this, regardless of role -- use requireRole for actions that need more.
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

// requireRole wraps next so it only runs for a logged-in staff member
// whose role satisfies minRole (RoleSatisfies) -- otherwise it redirects
// to login (no session) or renders a plain 403 (authenticated,
// insufficient role). Mirrors internal/admin/auth_middleware.go's own
// requireRole exactly.
func (s *Server) requireRole(minRole string, next staffHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		staff, ok := s.sessionUser(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if !RoleSatisfies(staff.Role, minRole) {
			http.Error(w, "Forbidden: your role doesn't have access to this page.", http.StatusForbidden)
			return
		}
		if !authn.SameOrigin(r, s.cfg.RootURL) {
			http.Error(w, "Forbidden: cross-origin request rejected.", http.StatusForbidden)
			return
		}
		next(w, r, staff)
	}
}

// requireApprovalRole wraps next so it only runs for a logged-in staff
// member whose workflow role is StaffWorkflowRoleSecurityComplianceApprover
// -- otherwise it redirects to login (no session) or renders a plain 403
// (authenticated, wrong workflow role). A separate gate from requireRole:
// StaffRoleAdmin/Member governs administrative capability (who manages
// this tool); this governs workflow participation (who may decide an
// approval request, §10.1) -- an admin account does not automatically
// satisfy this check, by design (separation of duties).
func (s *Server) requireApprovalRole(next staffHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		staff, ok := s.sessionUser(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if staff.WorkflowRole != StaffWorkflowRoleSecurityComplianceApprover {
			http.Error(w, "Forbidden: your workflow role doesn't permit deciding approval requests.", http.StatusForbidden)
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

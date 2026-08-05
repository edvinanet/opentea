package admin

import (
	"net/http"

	"github.com/oej/opentea/internal/authn"
	"github.com/oej/opentea/internal/httpx"
)

// requireRole wraps next so it only runs for a logged-in user whose role
// satisfies minRole -- otherwise it writes a 401 (no/invalid session) or 403
// (authenticated, insufficient role) JSON response.
func (s *Server) requireRole(minRole string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := authn.SessionUser(r.Context(), r, s.repo)
		if !ok {
			httpx.Unauthorized(w, "authentication required")
			return
		}
		if !authn.RoleSatisfies(user.Role, minRole) {
			httpx.Forbidden(w, "forbidden")
			return
		}
		if !authn.SameOrigin(r, s.cfg.RootURL) {
			httpx.Forbidden(w, "cross-origin request rejected")
			return
		}
		next(w, r)
	}
}

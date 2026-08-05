package webadmin

import (
	"net/http"

	"github.com/oej/opentea/internal/authn"
	"github.com/oej/opentea/internal/model"
)

// roleHandler is like http.HandlerFunc but also receives the resolved user
// -- every authenticated GUI page needs it (at least to render the nav).
type roleHandler func(w http.ResponseWriter, r *http.Request, user model.User)

// requireRole wraps next so it only runs for a logged-in user whose role
// satisfies minRole -- otherwise it redirects to the login page (no session)
// or renders a plain 403 (authenticated, insufficient role).
func (s *Server) requireRole(minRole string, next roleHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := authn.SessionUser(r.Context(), r, s.repo)
		if !ok {
			http.Redirect(w, r, "/admin/ui/login", http.StatusSeeOther)
			return
		}
		if !authn.RoleSatisfies(user.Role, minRole) {
			http.Error(w, "Forbidden: your role doesn't have access to this page.", http.StatusForbidden)
			return
		}
		if !authn.SameOrigin(r, s.cfg.RootURL) {
			http.Error(w, "Forbidden: cross-origin request rejected.", http.StatusForbidden)
			return
		}
		next(w, r, user)
	}
}

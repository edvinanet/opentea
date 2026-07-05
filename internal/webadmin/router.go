package webadmin

import (
	"net/http"

	"github.com/oej/opentea/internal/model"
)

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin/ui/login", s.loginForm)
	mux.HandleFunc("POST /admin/ui/login", s.loginSubmit)
	mux.HandleFunc("POST /admin/ui/logout", s.logout)

	// {$} restricts this to an exact match on /admin/ui/ -- otherwise, as a
	// prefix pattern, it would also catch unknown sub-paths as a side effect.
	mux.HandleFunc("GET /admin/ui/{$}", s.requireRole(model.RoleConsumer, s.dashboard))

	mux.HandleFunc("GET /admin/ui/users", s.requireRole(model.RoleAdmin, s.usersPage))
	mux.HandleFunc("POST /admin/ui/users", s.requireRole(model.RoleAdmin, s.createUserForm))
	mux.HandleFunc("POST /admin/ui/users/{uuid}/delete", s.requireRole(model.RoleAdmin, s.deleteUserForm))

	mux.HandleFunc("GET /admin/ui/token", s.requireRole(model.RoleConsumer, s.tokenPage))
	mux.HandleFunc("POST /admin/ui/token/generate", s.requireRole(model.RoleConsumer, s.generateToken))
}

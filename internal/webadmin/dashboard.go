package webadmin

import (
	"net/http"

	"github.com/oej/opentea/internal/model"
)

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request, user model.User) {
	stats, err := s.repo.GetStats(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.renderAuthenticated(w, "dashboard", pageData{User: user, Stats: stats})
}

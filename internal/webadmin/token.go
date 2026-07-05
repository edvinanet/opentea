package webadmin

import (
	"net/http"

	"github.com/oej/opentea/internal/model"
)

func (s *Server) tokenPage(w http.ResponseWriter, r *http.Request, user model.User) {
	createdAt, err := s.repo.GetAPITokenCreatedAt(r.Context(), user.UUID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.renderAuthenticated(w, "token", pageData{User: user, TokenInfo: createdAt})
}

func (s *Server) generateToken(w http.ResponseWriter, r *http.Request, user model.User) {
	token, err := s.repo.SetAPIToken(r.Context(), user.UUID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	createdAt, err := s.repo.GetAPITokenCreatedAt(r.Context(), user.UUID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.renderAuthenticated(w, "token", pageData{User: user, TokenInfo: createdAt, NewToken: token})
}

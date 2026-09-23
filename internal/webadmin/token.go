// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package webadmin

import (
	"net/http"

	"github.com/oej/opentea/internal/model"
)

func (s *Server) tokenPage(w http.ResponseWriter, r *http.Request, user model.User) {
	createdAt, err := s.repo.GetAPIKeyCreatedAt(r.Context(), user.UUID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.renderAuthenticated(w, "token", pageData{User: user, TokenInfo: createdAt})
}

func (s *Server) generateToken(w http.ResponseWriter, r *http.Request, user model.User) {
	keyID, secret, err := s.repo.SetAPIKey(r.Context(), user.UUID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	createdAt, err := s.repo.GetAPIKeyCreatedAt(r.Context(), user.UUID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.renderAuthenticated(w, "token", pageData{User: user, TokenInfo: createdAt, NewKeyID: keyID, NewSecret: secret})
}

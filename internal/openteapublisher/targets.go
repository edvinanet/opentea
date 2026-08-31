// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"errors"
	"net/http"
)

func (s *Server) createTargetForm(w http.ResponseWriter, r *http.Request, staff Staff) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	label := r.PostFormValue("label")
	baseURL := r.PostFormValue("baseUrl")
	bearerToken := r.PostFormValue("bearerToken")

	if label == "" || baseURL == "" || bearerToken == "" {
		s.rerenderDashboardWithError(w, r, staff, "label, base URL, and bearer token are all required")
		return
	}

	if _, err := s.repo.CreateTarget(r.Context(), label, baseURL, bearerToken); err != nil {
		s.rerenderDashboardWithError(w, r, staff, "internal error, please try again")
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) deleteTargetForm(w http.ResponseWriter, r *http.Request, staff Staff) {
	uuid := r.PathValue("uuid")
	err := s.repo.DeleteTarget(r.Context(), uuid)
	if err == nil || errors.Is(err, ErrNotFound) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.rerenderDashboardWithError(w, r, staff, "internal error, please try again")
}

func (s *Server) rerenderDashboardWithError(w http.ResponseWriter, r *http.Request, staff Staff, message string) {
	targets, err := s.repo.ListTargets(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.renderAuthenticated(w, "dashboard", pageData{Staff: staff, Targets: targets, Error: message})
}

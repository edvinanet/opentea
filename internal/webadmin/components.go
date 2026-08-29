// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package webadmin

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/repo"
)

func (s *Server) componentsPage(w http.ResponseWriter, r *http.Request, user model.User) {
	components, err := s.repo.QueryComponents(r.Context(), "", "", "name", "asc", nil, listPageLimit)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.renderAuthenticated(w, "components", pageData{User: user, Components: components})
}

func (s *Server) componentDetailPage(w http.ResponseWriter, r *http.Request, user model.User) {
	uuid := r.PathValue("uuid")

	component, err := s.repo.GetComponent(r.Context(), uuid)
	if errors.Is(err, repo.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	cle, err := s.repo.GetCLE(r.Context(), repo.OwnerComponent, uuid)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	releases, err := s.repo.ListComponentReleasesByComponent(r.Context(), uuid, "", "asc", nil, listPageLimit)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	s.renderAuthenticated(w, "component", pageData{
		User: user, Component: component, ComponentCLE: cle, ComponentReleases: releases,
	})
}

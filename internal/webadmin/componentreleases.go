// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package webadmin

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/repo"
)

func (s *Server) componentReleaseDetailPage(w http.ResponseWriter, r *http.Request, user model.User) {
	uuid := r.PathValue("uuid")

	release, err := s.repo.GetComponentRelease(r.Context(), uuid)
	if errors.Is(err, repo.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	cle, err := s.repo.GetCLE(r.Context(), repo.OwnerComponentRelease, uuid)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	collections, err := s.repo.ListCollections(r.Context(), uuid, "desc", nil, listPageLimit, repo.BelongsToComponentRelease)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	s.renderAuthenticated(w, "componentRelease", pageData{
		User: user, ComponentRelease: release, ComponentReleaseCLE: cle, Collections: s.buildCollectionViews(r.Context(), collections),
	})
}

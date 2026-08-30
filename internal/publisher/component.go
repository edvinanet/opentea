// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package publisher

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/pkg/teapublisher"
)

// findComponentsLimit caps findComponents -- a find-before-create search
// aid, not the spec's paginated read surface, so a generous fixed cap is
// enough (matches internal/admin's own adminListLimit convention).
const findComponentsLimit = 1000

// findComponents implements findComponents: free-text search against
// name, to avoid duplicate rows for the same real component.
func (s *Server) findComponents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	components, err := s.repo.SearchComponents(r.Context(), q, findComponentsLimit)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, components)
}

// createComponent implements createComponent.
func (s *Server) createComponent(w http.ResponseWriter, r *http.Request) {
	var req teapublisher.ComponentCreate
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	if req.Name == "" {
		httpx.BadRequest(w, "name is required")
		return
	}
	c, err := s.repo.CreateComponent(r.Context(), req.Name, req.Identifiers)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, c)
}

// createComponentRelease implements createComponentRelease.
func (s *Server) createComponentRelease(w http.ResponseWriter, r *http.Request) {
	componentUUID, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	var req teapublisher.ComponentReleaseCreate
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	if req.Version == "" {
		httpx.BadRequest(w, "version is required")
		return
	}
	if req.CreatedDate.IsZero() {
		httpx.BadRequest(w, "createdDate is required")
		return
	}

	preRelease := false
	if req.PreRelease != nil {
		preRelease = *req.PreRelease
	}
	cr, err := s.repo.CreateComponentRelease(r.Context(), componentUUID, repo.ComponentReleaseInput{
		Version:     req.Version,
		CreatedDate: req.CreatedDate,
		ReleaseDate: req.ReleaseDate,
		PreRelease:  preRelease,
		Identifiers: req.Identifiers,
	})
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, cr)
}

// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package publisher

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/pkg/tea"
	"github.com/oej/opentea/pkg/teapublisher"
)

// createProduct implements createProduct (design/publisher-openapi.yaml):
// stable identity, no staging.
func (s *Server) createProduct(w http.ResponseWriter, r *http.Request) {
	var req teapublisher.ProductCreate
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	if req.Name == "" {
		httpx.BadRequest(w, "name is required")
		return
	}
	p, err := s.repo.CreateProduct(r.Context(), req.Name, req.Identifiers)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, p)
}

// createProductRelease implements createProductRelease.
func (s *Server) createProductRelease(w http.ResponseWriter, r *http.Request) {
	productUUID, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	var req teapublisher.ProductReleaseCreate
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
	pr, err := s.repo.CreateProductRelease(r.Context(), productUUID, repo.ProductReleaseInput{
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
	httpx.WriteJSON(w, http.StatusCreated, pr)
}

// linkComponent implements linkComponent: link a component to a product
// release, optionally pinned to a specific component release.
func (s *Server) linkComponent(w http.ResponseWriter, r *http.Request) {
	productReleaseUUID, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	var ref tea.ComponentRef
	if err := decodeJSON(r, &ref); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	if ref.UUID == "" {
		httpx.BadRequest(w, "uuid is required")
		return
	}

	pr, err := s.repo.LinkComponent(r.Context(), productReleaseUUID, ref)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, pr)
}

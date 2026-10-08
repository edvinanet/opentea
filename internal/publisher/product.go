// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package publisher

import (
	"errors"
	"net/http"
	"time"

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
		badRequest(w, r, teapublisher.ErrorInvalidRequestBody, "invalid JSON body: "+err.Error())
		return
	}
	if req.Name == "" {
		badRequest(w, r, teapublisher.ErrorMissingField, "name is required", teapublisher.FieldError{Field: "name", Message: "is required"})
		return
	}
	p, err := s.repo.CreateProduct(r.Context(), req.Name, req.Identifiers)
	if writeIdentifierValidationError(w, r, err) {
		return
	}
	if err != nil {
		internalErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, p)
}

// createProductRelease implements createProductRelease. CreatedDate is
// server-assigned (time.Now()), not taken from the request -- see
// teapublisher.ProductReleaseCreate's own doc comment for why
// (docs/security-review-publisher-design-260828.md finding 11).
func (s *Server) createProductRelease(w http.ResponseWriter, r *http.Request) {
	productUUID, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		badRequest(w, r, teapublisher.ErrorInvalidPathParameter, "invalid uuid")
		return
	}
	var req teapublisher.ProductReleaseCreate
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, r, teapublisher.ErrorInvalidRequestBody, "invalid JSON body: "+err.Error())
		return
	}
	if req.Version == "" {
		badRequest(w, r, teapublisher.ErrorMissingField, "version is required", teapublisher.FieldError{Field: "version", Message: "is required"})
		return
	}

	preRelease := false
	if req.PreRelease != nil {
		preRelease = *req.PreRelease
	}
	pr, err := s.repo.CreateProductRelease(r.Context(), productUUID, repo.ProductReleaseInput{
		Version:     req.Version,
		CreatedDate: time.Now(),
		ReleaseDate: req.ReleaseDate,
		PreRelease:  preRelease,
		Identifiers: req.Identifiers,
	})
	if errors.Is(err, repo.ErrNotFound) {
		notFoundErr(w, r)
		return
	}
	if writeIdentifierValidationError(w, r, err) {
		return
	}
	if err != nil {
		internalErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, pr)
}

// linkComponent implements linkComponent: link a component to a product
// release, optionally pinned to a specific component release.
func (s *Server) linkComponent(w http.ResponseWriter, r *http.Request) {
	productReleaseUUID, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		badRequest(w, r, teapublisher.ErrorInvalidPathParameter, "invalid uuid")
		return
	}
	var ref tea.ComponentRef
	if err := decodeJSON(r, &ref); err != nil {
		badRequest(w, r, teapublisher.ErrorInvalidRequestBody, "invalid JSON body: "+err.Error())
		return
	}
	if ref.UUID == "" {
		badRequest(w, r, teapublisher.ErrorMissingField, "uuid is required", teapublisher.FieldError{Field: "uuid", Message: "is required"})
		return
	}

	pr, err := s.repo.LinkComponent(r.Context(), productReleaseUUID, ref)
	if errors.Is(err, repo.ErrNotFound) {
		notFoundErr(w, r)
		return
	}
	if err != nil {
		internalErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, pr)
}

// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// products.go: the minimal Products/Releases screens (design/publisher-
// service.md §18.3) -- the first step of the critical-path publishing
// workflow (create a product, create a release under it) that everything
// else (collectiondraft.go) builds on. Deliberately thin: name/version
// only, no identifiers -- a "minimal form," not the eventual full screen
// §18.3 describes.
package openteapublisher

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/oej/opentea/pkg/tea"
	"github.com/oej/opentea/pkg/teaclient"
	"github.com/oej/opentea/pkg/teapublisher"
)

func (s *Server) productsPage(w http.ResponseWriter, r *http.Request, staff Staff) {
	targetUUID := r.PathValue("targetUuid")
	target, err := s.repo.GetTarget(r.Context(), targetUUID)
	if errors.Is(err, ErrNotFound) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	client, err := s.teaClientForTarget(r.Context(), targetUUID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	products, err := client.QueryProducts(r.Context(), teaclient.ListParams{})
	if err != nil {
		s.rerenderProductsWithError(w, r, staff, target, "could not list products from the target: "+decodeTeaClientErrorMessage(err))
		return
	}
	s.renderAuthenticated(w, "products", pageData{Staff: staff, Target: target, Products: products.Results})
}

func (s *Server) createProductForm(w http.ResponseWriter, r *http.Request, staff Staff) {
	targetUUID := r.PathValue("targetUuid")
	target, err := s.repo.GetTarget(r.Context(), targetUUID)
	if errors.Is(err, ErrNotFound) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/targets/"+targetUUID+"/products", http.StatusSeeOther)
		return
	}
	name := r.PostFormValue("name")
	if name == "" {
		s.rerenderProductsWithError(w, r, staff, target, "name is required")
		return
	}

	client, err := s.teaPublisherClientForTarget(r.Context(), targetUUID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	product, err := client.CreateProduct(r.Context(), teapublisher.ProductCreate{Name: name})
	if err != nil {
		s.rerenderProductsWithError(w, r, staff, target, decodeAPIErrorMessage(err))
		return
	}
	http.Redirect(w, r, "/targets/"+targetUUID+"/products/"+product.UUID, http.StatusSeeOther)
}

func (s *Server) rerenderProductsWithError(w http.ResponseWriter, r *http.Request, staff Staff, target Target, message string) {
	client, err := s.teaClientForTarget(r.Context(), target.UUID)
	var products tea.PaginatedProducts
	if err == nil {
		products, _ = client.QueryProducts(r.Context(), teaclient.ListParams{})
	}
	s.renderAuthenticated(w, "products", pageData{Staff: staff, Target: target, Products: products.Results, Error: message})
}

func (s *Server) productPage(w http.ResponseWriter, r *http.Request, staff Staff) {
	targetUUID := r.PathValue("targetUuid")
	productUUID := r.PathValue("uuid")
	target, err := s.repo.GetTarget(r.Context(), targetUUID)
	if errors.Is(err, ErrNotFound) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	client, err := s.teaClientForTarget(r.Context(), targetUUID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	product, err := client.GetProduct(r.Context(), productUUID)
	if teaclient.IsNotFound(err) {
		http.Redirect(w, r, "/targets/"+targetUUID+"/products", http.StatusSeeOther)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	releases, err := client.ListReleasesByProduct(r.Context(), productUUID, teaclient.ListParams{})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.renderAuthenticated(w, "product", pageData{Staff: staff, Target: target, Product: product, ProductReleases: releases.Results})
}

func (s *Server) createProductReleaseForm(w http.ResponseWriter, r *http.Request, staff Staff) {
	targetUUID := r.PathValue("targetUuid")
	productUUID := r.PathValue("uuid")
	target, err := s.repo.GetTarget(r.Context(), targetUUID)
	if errors.Is(err, ErrNotFound) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/targets/"+targetUUID+"/products/"+productUUID, http.StatusSeeOther)
		return
	}
	version := r.PostFormValue("version")
	if version == "" {
		s.rerenderProductWithError(w, r, staff, target, productUUID, "version is required")
		return
	}

	client, err := s.teaPublisherClientForTarget(r.Context(), targetUUID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	release, err := client.CreateProductRelease(r.Context(), productUUID, teapublisher.ProductReleaseCreate{Version: version})
	if err != nil {
		s.rerenderProductWithError(w, r, staff, target, productUUID, decodeAPIErrorMessage(err))
		return
	}
	http.Redirect(w, r, "/targets/"+targetUUID+"/productReleases/"+release.UUID, http.StatusSeeOther)
}

func (s *Server) rerenderProductWithError(w http.ResponseWriter, r *http.Request, staff Staff, target Target, productUUID, message string) {
	client, err := s.teaClientForTarget(r.Context(), target.UUID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	product, err := client.GetProduct(r.Context(), productUUID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	releases, _ := client.ListReleasesByProduct(r.Context(), productUUID, teaclient.ListParams{})
	s.renderAuthenticated(w, "product", pageData{Staff: staff, Target: target, Product: product, ProductReleases: releases.Results, Error: message})
}

// decodeTeaClientErrorMessage mirrors decodeAPIErrorMessage (targetclients.go)
// but for teaclient.APIError (the read side) -- /tea/v1's error-response
// schema is the consumer spec's own {error, message} shape (pkg/tea.ErrorResponse),
// a different type from /publisher/v1's teapublisher.ErrorResponse, hence a
// separate decode.
func decodeTeaClientErrorMessage(err error) string {
	var apiErr *teaclient.APIError
	if !errors.As(err, &apiErr) {
		return err.Error()
	}
	var parsed tea.ErrorResponse
	if jsonErr := json.Unmarshal(apiErr.Body, &parsed); jsonErr == nil && parsed.Error != "" {
		if parsed.Message != "" {
			return parsed.Message
		}
		return parsed.Error
	}
	return string(apiErr.Body)
}

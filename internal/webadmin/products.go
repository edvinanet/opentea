package webadmin

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/repo"
)

// listPageLimit caps unpaginated browse pages -- matches the existing
// precedent set by usersPage (ListUsers also has no cursor/limit), just
// generous enough that this won't clip a realistic dataset. Proper
// pagination UI is a follow-up if this ever becomes a real limitation (see
// TODO.md).
const listPageLimit = 500

func (s *Server) productsPage(w http.ResponseWriter, r *http.Request, user model.User) {
	products, err := s.repo.QueryProducts(r.Context(), "", "", "name", "asc", nil, listPageLimit)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.renderAuthenticated(w, "products", pageData{User: user, Products: products})
}

func (s *Server) productDetailPage(w http.ResponseWriter, r *http.Request, user model.User) {
	uuid := r.PathValue("uuid")

	product, err := s.repo.GetProduct(r.Context(), uuid)
	if errors.Is(err, repo.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	cle, err := s.repo.GetCLE(r.Context(), repo.OwnerProduct, uuid)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	releases, err := s.repo.ListProductReleasesByProduct(r.Context(), uuid, "", "asc", nil, listPageLimit)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	s.renderAuthenticated(w, "product", pageData{
		User: user, Product: product, ProductCLE: cle, ProductReleases: releases,
	})
}

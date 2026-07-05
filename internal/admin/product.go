package admin

import (
	"errors"
	"net/http"
	"time"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/pkg/tea"
)

type createProductRequest struct {
	Name        string           `json:"name"`
	Identifiers []tea.Identifier `json:"identifiers"`
}

func (s *Server) createProduct(w http.ResponseWriter, r *http.Request) {
	var req createProductRequest
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

// adminListLimit caps the plain (non-paginated) admin list endpoints -- the
// admin API is an internal ingestion tool, not the spec's paginated read
// surface, so a generous fixed cap is enough for Phase 1.
const adminListLimit = 1000

func (s *Server) listProducts(w http.ResponseWriter, r *http.Request) {
	products, err := s.repo.QueryProducts(r.Context(), "", "", "name", "asc", nil, adminListLimit)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, products)
}

func (s *Server) getProduct(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	p, err := s.repo.GetProduct(r.Context(), uuid)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

func (s *Server) deleteProduct(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	if err := s.repo.DeleteProduct(r.Context(), uuid); errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type releaseCreateRequest struct {
	Version     string           `json:"version"`
	CreatedDate time.Time        `json:"createdDate"`
	ReleaseDate *time.Time       `json:"releaseDate,omitempty"`
	PreRelease  bool             `json:"preRelease,omitempty"`
	Identifiers []tea.Identifier `json:"identifiers,omitempty"`
}

func (s *Server) createProductRelease(w http.ResponseWriter, r *http.Request) {
	productUUID, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	var req releaseCreateRequest
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

	pr, err := s.repo.CreateProductRelease(r.Context(), productUUID, repo.ProductReleaseInput{
		Version:     req.Version,
		CreatedDate: req.CreatedDate,
		ReleaseDate: req.ReleaseDate,
		PreRelease:  req.PreRelease,
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

func (s *Server) getProductRelease(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	pr, err := s.repo.GetProductRelease(r.Context(), uuid)
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

func (s *Server) deleteProductRelease(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	if err := s.repo.DeleteProductRelease(r.Context(), uuid); errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

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

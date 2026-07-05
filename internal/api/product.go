package api

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/pkg/tea"
)

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

var productReleaseSortFields = []string{"createdDate", "releaseDate", "version"}

func (s *Server) listReleasesByProduct(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	if _, err := s.repo.GetProduct(r.Context(), uuid); errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}

	pp, ok := parsePageParams(w, r, productReleaseSortFields)
	if !ok {
		return
	}

	rows, err := s.repo.ListProductReleasesByProduct(r.Context(), uuid, pp.SortField, pp.SortOrder, pp.Cursor, pp.PageSize+1)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	page, hasNext := splitPage(rows, pp.PageSize)

	resp := tea.PaginatedProductReleases{Results: page}
	resp.HasNext = hasNext
	if hasNext {
		last := page[len(page)-1]
		resp.NextPageToken = nextPageToken(true, pp.SortField, pp.SortOrder, productReleaseSortValue(last, pp.SortField), last.UUID)
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

var productSortFields = []string{"name"}

func (s *Server) queryProducts(w http.ResponseWriter, r *http.Request) {
	idType, idValue, err := httpx.IDFilter(r)
	if err != nil {
		httpx.BadRequest(w, "invalid idType")
		return
	}
	pp, ok := parsePageParams(w, r, productSortFields)
	if !ok {
		return
	}

	rows, err := s.repo.QueryProducts(r.Context(), idType, idValue, pp.SortField, pp.SortOrder, pp.Cursor, pp.PageSize+1)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	page, hasNext := splitPage(rows, pp.PageSize)

	resp := tea.PaginatedProducts{Results: page}
	resp.HasNext = hasNext
	if hasNext {
		last := page[len(page)-1]
		resp.NextPageToken = nextPageToken(true, pp.SortField, pp.SortOrder, last.Name, last.UUID)
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/oej/opentea/internal/authz"
	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/pagination"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/pkg/tea"
)

func (s *Server) getProduct(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}

	// Product has no revision column (it's provably immutable -- no update
	// path exists), so its ETag is built from identity alone; this
	// existence check is still required (not skippable), since a deleted
	// product's cached ETag must not keep matching forever -- see
	// repo.ExistsProduct's doc comment.
	if err := s.repo.ExistsProduct(r.Context(), uuid); errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if !s.authorize(w, r, authz.CapProductRead, authz.Resource{ProductUUID: uuid}) {
		return
	}
	if s.conditional(w, r, cacheControlRevalidate, "product", uuid) {
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
	// Parent existence gate: if the caller can't even discover this
	// product, its release list is equally hidden (spec Sec 18). Each row
	// is then independently filtered by release.read below -- product
	// access does NOT imply seeing every release (spec Sec 16.1).
	if !s.authorize(w, r, authz.CapProductDiscover, authz.Resource{ProductUUID: uuid}) {
		return
	}

	pp, ok := parsePageParams(w, r, productReleaseSortFields)
	if !ok {
		return
	}

	watermark, err := s.repo.GetWatermark(r.Context(), repo.WatermarkProductReleases)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if s.conditional(w, r, cacheControlRevalidate, "productReleases", uuid, pp.SortField, pp.SortOrder, cursorPart(pp.Cursor), strconv.FormatInt(watermark, 10)) {
		return
	}

	page, hasNext, err := filterAuthorized(
		func(cursor *pagination.Cursor, limit int) ([]tea.ProductRelease, error) {
			return s.repo.ListProductReleasesByProduct(r.Context(), uuid, pp.SortField, pp.SortOrder, cursor, limit)
		},
		func(pr tea.ProductRelease) pagination.Cursor {
			return pagination.Cursor{SortField: pp.SortField, SortOrder: pp.SortOrder, LastValue: productReleaseSortValue(pr, pp.SortField), LastUUID: pr.UUID}
		},
		func(pr tea.ProductRelease) (bool, error) {
			return s.decide(r, authz.CapReleaseRead, authz.Resource{ProductReleaseUUID: pr.UUID})
		},
		pp.Cursor, pp.PageSize,
	)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}

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

	watermark, err := s.repo.GetWatermark(r.Context(), repo.WatermarkProducts)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if s.conditional(w, r, cacheControlRevalidate, "products", idType, idValue, pp.SortField, pp.SortOrder, cursorPart(pp.Cursor), strconv.FormatInt(watermark, 10)) {
		return
	}

	page, hasNext, err := filterAuthorized(
		func(cursor *pagination.Cursor, limit int) ([]tea.Product, error) {
			return s.repo.QueryProducts(r.Context(), idType, idValue, pp.SortField, pp.SortOrder, cursor, limit)
		},
		func(p tea.Product) pagination.Cursor {
			return pagination.Cursor{SortField: pp.SortField, SortOrder: pp.SortOrder, LastValue: p.Name, LastUUID: p.UUID}
		},
		func(p tea.Product) (bool, error) {
			return s.decide(r, authz.CapProductRead, authz.Resource{ProductUUID: p.UUID})
		},
		pp.Cursor, pp.PageSize,
	)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}

	resp := tea.PaginatedProducts{Results: page}
	resp.HasNext = hasNext
	if hasNext {
		last := page[len(page)-1]
		resp.NextPageToken = nextPageToken(true, pp.SortField, pp.SortOrder, last.Name, last.UUID)
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

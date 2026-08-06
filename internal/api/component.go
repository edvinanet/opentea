package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/pkg/tea"
)

func (s *Server) getComponent(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}

	// See getProduct's comment (internal/api/product.go) -- component is
	// the same provably-immutable, existence-check-only case.
	if err := s.repo.ExistsComponent(r.Context(), uuid); errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if httpx.WriteConditional(w, r, httpx.BuildETag("component", uuid), cacheControlRevalidate) {
		return
	}

	c, err := s.repo.GetComponent(r.Context(), uuid)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}

var componentReleaseSortFields = []string{"createdDate", "releaseDate", "version"}

func (s *Server) listReleasesByComponent(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	if _, err := s.repo.GetComponent(r.Context(), uuid); errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}

	pp, ok := parsePageParams(w, r, componentReleaseSortFields)
	if !ok {
		return
	}

	watermark, err := s.repo.GetWatermark(r.Context(), repo.WatermarkComponentReleases)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	etag := httpx.BuildETag("componentReleases", uuid, pp.SortField, pp.SortOrder, cursorPart(pp.Cursor), strconv.FormatInt(watermark, 10))
	if httpx.WriteConditional(w, r, etag, cacheControlRevalidate) {
		return
	}

	rows, err := s.repo.ListComponentReleasesByComponent(r.Context(), uuid, pp.SortField, pp.SortOrder, pp.Cursor, pp.PageSize+1)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	page, hasNext := splitPage(rows, pp.PageSize)

	resp := tea.PaginatedComponentReleases{Results: page}
	resp.HasNext = hasNext
	if hasNext {
		last := page[len(page)-1]
		resp.NextPageToken = nextPageToken(true, pp.SortField, pp.SortOrder, componentReleaseSortValue(last, pp.SortField), last.UUID)
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

var componentSortFields = []string{"name"}

func (s *Server) queryComponents(w http.ResponseWriter, r *http.Request) {
	idType, idValue, err := httpx.IDFilter(r)
	if err != nil {
		httpx.BadRequest(w, "invalid idType")
		return
	}
	pp, ok := parsePageParams(w, r, componentSortFields)
	if !ok {
		return
	}

	watermark, err := s.repo.GetWatermark(r.Context(), repo.WatermarkComponents)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	etag := httpx.BuildETag("components", idType, idValue, pp.SortField, pp.SortOrder, cursorPart(pp.Cursor), strconv.FormatInt(watermark, 10))
	if httpx.WriteConditional(w, r, etag, cacheControlRevalidate) {
		return
	}

	rows, err := s.repo.QueryComponents(r.Context(), idType, idValue, pp.SortField, pp.SortOrder, pp.Cursor, pp.PageSize+1)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	page, hasNext := splitPage(rows, pp.PageSize)

	resp := tea.PaginatedComponents{Results: page}
	resp.HasNext = hasNext
	if hasNext {
		last := page[len(page)-1]
		resp.NextPageToken = nextPageToken(true, pp.SortField, pp.SortOrder, last.Name, last.UUID)
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

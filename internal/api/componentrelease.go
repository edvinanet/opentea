package api

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/pkg/tea"
)

// getComponentReleaseWithCollection returns component-release-with-collection.
// The spec marks both release and latestCollection as required, so if the
// release has no collection yet, we 404 rather than fabricate an empty one.
func (s *Server) getComponentReleaseWithCollection(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	cr, err := s.repo.GetComponentRelease(r.Context(), uuid)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}

	latest, err := s.repo.GetLatestCollection(r.Context(), uuid, repo.BelongsToComponentRelease)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, tea.ComponentReleaseWithCollection{Release: cr, LatestCollection: latest})
}

func (s *Server) queryComponentReleases(w http.ResponseWriter, r *http.Request) {
	idType, idValue, err := httpx.IDFilter(r)
	if err != nil {
		httpx.BadRequest(w, "invalid idType")
		return
	}
	pp, ok := parsePageParams(w, r, componentReleaseSortFields)
	if !ok {
		return
	}

	rows, err := s.repo.QueryComponentReleases(r.Context(), idType, idValue, pp.SortField, pp.SortOrder, pp.Cursor, pp.PageSize+1)
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

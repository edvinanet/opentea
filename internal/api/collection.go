package api

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/pkg/tea"
)

var collectionSortFields = []string{"version"}

func (s *Server) latestCollectionForComponentRelease(w http.ResponseWriter, r *http.Request) {
	s.latestCollection(w, r, repo.BelongsToComponentRelease)
}

func (s *Server) latestCollectionForProductRelease(w http.ResponseWriter, r *http.Request) {
	s.latestCollection(w, r, repo.BelongsToProductRelease)
}

// latestCollection is shared by both route variants above, each passing the
// release type its own route is scoped to -- so a collection belonging to
// the *other* type is never resolvable through the wrong route, even if its
// uuid happens to collide with one that does belong to this route's type
// (see repo.GetLatestCollection's doc comment).
func (s *Server) latestCollection(w http.ResponseWriter, r *http.Request, belongsTo string) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	c, err := s.repo.GetLatestCollection(r.Context(), uuid, belongsTo)
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

func (s *Server) getCollectionForComponentRelease(w http.ResponseWriter, r *http.Request) {
	s.getCollectionByVersion(w, r, repo.BelongsToComponentRelease)
}

func (s *Server) getCollectionForProductRelease(w http.ResponseWriter, r *http.Request) {
	s.getCollectionByVersion(w, r, repo.BelongsToProductRelease)
}

func (s *Server) getCollectionByVersion(w http.ResponseWriter, r *http.Request, belongsTo string) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	version, err := httpx.PathPositiveInt(r, "collectionVersion")
	if err != nil {
		httpx.BadRequest(w, "invalid collectionVersion")
		return
	}
	c, err := s.repo.GetCollectionByVersion(r.Context(), uuid, version, belongsTo)
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

func (s *Server) listCollectionsForComponentRelease(w http.ResponseWriter, r *http.Request) {
	s.listCollections(w, r, repo.BelongsToComponentRelease)
}

func (s *Server) listCollectionsForProductRelease(w http.ResponseWriter, r *http.Request) {
	s.listCollections(w, r, repo.BelongsToProductRelease)
}

func (s *Server) listCollections(w http.ResponseWriter, r *http.Request, belongsTo string) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	pp, ok := parsePageParams(w, r, collectionSortFields)
	if !ok {
		return
	}

	rows, err := s.repo.ListCollections(r.Context(), uuid, pp.SortOrder, pp.Cursor, pp.PageSize+1, belongsTo)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	page, hasNext := splitPage(rows, pp.PageSize)

	resp := tea.PaginatedCollections{Results: page}
	resp.HasNext = hasNext
	if hasNext {
		last := page[len(page)-1]
		resp.NextPageToken = nextPageToken(true, pp.SortField, pp.SortOrder, collectionSortValue(last), last.UUID)
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

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
	s.latestCollection(w, r)
}

func (s *Server) latestCollectionForProductRelease(w http.ResponseWriter, r *http.Request) {
	s.latestCollection(w, r)
}

func (s *Server) latestCollection(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	c, err := s.repo.GetLatestCollection(r.Context(), uuid)
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
	s.getCollectionByVersion(w, r)
}

func (s *Server) getCollectionForProductRelease(w http.ResponseWriter, r *http.Request) {
	s.getCollectionByVersion(w, r)
}

func (s *Server) getCollectionByVersion(w http.ResponseWriter, r *http.Request) {
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
	c, err := s.repo.GetCollectionByVersion(r.Context(), uuid, version)
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
	s.listCollections(w, r)
}

func (s *Server) listCollectionsForProductRelease(w http.ResponseWriter, r *http.Request) {
	s.listCollections(w, r)
}

func (s *Server) listCollections(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	pp, ok := parsePageParams(w, r, collectionSortFields)
	if !ok {
		return
	}

	rows, err := s.repo.ListCollections(r.Context(), uuid, pp.SortOrder, pp.Cursor, pp.PageSize+1)
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

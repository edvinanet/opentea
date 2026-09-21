// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

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

func (s *Server) getComponent(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequestTyped(w, tea.ErrorInvalidRequest, "invalid uuid")
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
	// Components share product.*'s capability names -- the spec's
	// vocabulary has one generic product.read, not separate product/
	// component capabilities; Resource.ComponentUUID is what tells the
	// policy engine which branch of the scope hierarchy to expand.
	if !s.authorize(w, r, authz.CapProductRead, authz.Resource{ComponentUUID: uuid}) {
		return
	}
	if s.conditional(w, r, cacheControlRevalidate, "component", uuid) {
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
		httpx.BadRequestTyped(w, tea.ErrorInvalidRequest, "invalid uuid")
		return
	}
	if _, err := s.repo.GetComponent(r.Context(), uuid); errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if !s.authorize(w, r, authz.CapProductDiscover, authz.Resource{ComponentUUID: uuid}) {
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
	if s.conditional(w, r, cacheControlRevalidate, "componentReleases", uuid, pp.SortField, pp.SortOrder, cursorPart(pp.Cursor), strconv.FormatInt(watermark, 10)) {
		return
	}

	page, hasNext, err := filterAuthorized(
		func(cursor *pagination.Cursor, limit int) ([]tea.ComponentRelease, error) {
			return s.repo.ListComponentReleasesByComponent(r.Context(), uuid, pp.SortField, pp.SortOrder, cursor, limit)
		},
		func(cr tea.ComponentRelease) pagination.Cursor {
			return pagination.Cursor{SortField: pp.SortField, SortOrder: pp.SortOrder, LastValue: componentReleaseSortValue(cr, pp.SortField), LastUUID: cr.UUID}
		},
		func(cr tea.ComponentRelease) (bool, error) {
			return s.decide(r, authz.CapReleaseRead, authz.Resource{ComponentReleaseUUID: cr.UUID})
		},
		pp.Cursor, pp.PageSize,
	)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}

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
		httpx.BadRequestTyped(w, tea.ErrorInvalidRequest, "invalid idType")
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
	if s.conditional(w, r, cacheControlRevalidate, "components", idType, idValue, pp.SortField, pp.SortOrder, cursorPart(pp.Cursor), strconv.FormatInt(watermark, 10)) {
		return
	}

	page, hasNext, err := filterAuthorized(
		func(cursor *pagination.Cursor, limit int) ([]tea.Component, error) {
			return s.repo.QueryComponents(r.Context(), idType, idValue, pp.SortField, pp.SortOrder, cursor, limit)
		},
		func(c tea.Component) pagination.Cursor {
			return pagination.Cursor{SortField: pp.SortField, SortOrder: pp.SortOrder, LastValue: c.Name, LastUUID: c.UUID}
		},
		func(c tea.Component) (bool, error) {
			return s.decide(r, authz.CapProductRead, authz.Resource{ComponentUUID: c.UUID})
		},
		pp.Cursor, pp.PageSize,
	)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}

	resp := tea.PaginatedComponents{Results: page}
	resp.HasNext = hasNext
	if hasNext {
		last := page[len(page)-1]
		resp.NextPageToken = nextPageToken(true, pp.SortField, pp.SortOrder, last.Name, last.UUID)
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

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

func (s *Server) getProductRelease(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequestTyped(w, tea.ErrorInvalidRequest, "invalid uuid")
		return
	}

	revision, err := s.repo.GetProductReleaseRevision(r.Context(), uuid)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if !s.authorize(w, r, authz.CapReleaseRead, authz.Resource{ProductReleaseUUID: uuid}) {
		return
	}
	if s.conditional(w, r, cacheControlRevalidate, "productRelease", uuid, strconv.FormatInt(revision, 10)) {
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

func (s *Server) queryProductReleases(w http.ResponseWriter, r *http.Request) {
	idType, idValue, err := httpx.IDFilter(r)
	if err != nil {
		httpx.BadRequestTyped(w, tea.ErrorInvalidRequest, "invalid idType")
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
	if s.conditional(w, r, cacheControlRevalidate, "productReleases", idType, idValue, pp.SortField, pp.SortOrder, cursorPart(pp.Cursor), strconv.FormatInt(watermark, 10)) {
		return
	}

	page, hasNext, err := filterAuthorized(
		func(cursor *pagination.Cursor, limit int) ([]tea.ProductRelease, error) {
			return s.repo.QueryProductReleases(r.Context(), idType, idValue, pp.SortField, pp.SortOrder, cursor, limit)
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

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

// getComponentReleaseWithCollection returns component-release-with-collection.
// The spec marks both release and latestCollection as required, so if the
// release has no collection yet, we 404 rather than fabricate an empty one.
func (s *Server) getComponentReleaseWithCollection(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}

	// This endpoint's response embeds BOTH the component release and its
	// latest collection, so its light-fetch is genuinely two single-column
	// lookups, not one -- the release's own revision, plus the latest
	// collection version (which can change independent of that revision:
	// a brand-new collection appearing, or a newer one becoming latest).
	revision, err := s.repo.GetComponentReleaseRevision(r.Context(), uuid)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	latestVersion, hasCollection, err := s.repo.LatestCollectionVersion(r.Context(), uuid, repo.BelongsToComponentRelease)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if !hasCollection {
		// Matches the full-fetch path below: no collection yet is a 404 for
		// this endpoint specifically (spec requires latestCollection), not
		// an empty/absent field.
		httpx.NotFound(w)
		return
	}
	// This response can't omit latestCollection (the wire contract requires
	// it), so denying release access without also checking collection
	// access would let a collection-only grant leak the release, and vice
	// versa -- both capabilities must allow. collectionUUID == uuid in this
	// schema (see internal/db/migrations/0001_init.sql's comment on the
	// collection table).
	if !s.authorize(w, r, authz.CapReleaseRead, authz.Resource{ComponentReleaseUUID: uuid}) {
		return
	}
	if !s.authorize(w, r, authz.CapCollectionRead, authz.Resource{CollectionUUID: uuid}) {
		return
	}
	if s.conditional(w, r, cacheControlRevalidate, "componentRelease", uuid, strconv.FormatInt(revision, 10), strconv.Itoa(latestVersion)) {
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

	watermark, err := s.repo.GetWatermark(r.Context(), repo.WatermarkComponentReleases)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if s.conditional(w, r, cacheControlRevalidate, "componentReleases", idType, idValue, pp.SortField, pp.SortOrder, cursorPart(pp.Cursor), strconv.FormatInt(watermark, 10)) {
		return
	}

	page, hasNext, err := filterAuthorized(
		func(cursor *pagination.Cursor, limit int) ([]tea.ComponentRelease, error) {
			return s.repo.QueryComponentReleases(r.Context(), idType, idValue, pp.SortField, pp.SortOrder, cursor, limit)
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

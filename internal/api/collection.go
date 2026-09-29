// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/oej/opentea/internal/authz"
	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/pkg/tea"
)

var collectionSortFields = []string{"version"}

// collectionArtifactETagParts converts revisions (from
// repo.CollectionArtifactRevisions) into extra ETag parts, shared by
// every endpoint whose response embeds a specific collection version's
// full artifact list (docs/security-review-260923.md finding #8) --
// getCollectionByVersion/latestCollection here, and
// getProductRelease/getComponentReleaseWithCollection (productrelease.go/
// componentrelease.go), which embed a release's latest collection the
// same way since findings #5/#7's fix.
func collectionArtifactETagParts(revisions []int64) []string {
	parts := make([]string, len(revisions))
	for i, rev := range revisions {
		parts[i] = strconv.FormatInt(rev, 10)
	}
	return parts
}

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
		httpx.BadRequestTyped(w, tea.ErrorInvalidRequest, "invalid uuid")
		return
	}

	version, ok, err := s.repo.LatestCollectionVersion(r.Context(), uuid, belongsTo)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if !ok {
		httpx.NotFound(w)
		return
	}
	// Collection authorization is evaluated independently of product/
	// release visibility (spec Sec 12.2/17.1); the policy engine derives
	// belongs_to and the owning release itself from collectionUUID, so
	// callers here just pass CollectionUUID (== uuid, this schema's
	// identity convention).
	if !s.authorize(w, r, authz.CapCollectionRead, authz.Resource{CollectionUUID: uuid}) {
		return
	}
	// The response embeds this version's full artifact list, fetched live
	// on every read -- uploading new format content to one of those
	// artifacts changes the response body without changing collection
	// identity/version, so their own revisions must be part of the ETag
	// too (docs/security-review-260923.md finding #8).
	revisions, err := s.repo.CollectionArtifactRevisions(r.Context(), uuid, version)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	etagParts := append([]string{"collection-latest", belongsTo, uuid, strconv.Itoa(version)}, collectionArtifactETagParts(revisions)...)
	if s.conditional(w, r, cacheControlRevalidate, etagParts...) {
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
		httpx.BadRequestTyped(w, tea.ErrorInvalidRequest, "invalid uuid")
		return
	}
	version, err := httpx.PathPositiveInt(r, "collectionVersion")
	if err != nil {
		httpx.BadRequestTyped(w, tea.ErrorInvalidRequest, "invalid collectionVersion")
		return
	}

	// A specific collection version's own row is immutable (insert-only,
	// no update path -- see repo.ExistsCollectionVersion's doc comment),
	// but the response it produces is NOT: it embeds each referenced
	// artifact's CURRENT format content on every read, and this codebase's
	// own two-step create-then-upload flow can still add or replace that
	// content after the collection was published (the exact same reason
	// the versioned artifact-download endpoint isn't cacheControlImmutable
	// either -- see downloadArtifactByVersion's doc comment,
	// artifactdownload.go). Was previously cacheControlImmutable with an
	// ETag built from collection identity alone, which a real,
	// content-changing artifact upload never invalidated
	// (docs/security-review-260923.md finding #8).
	if err := s.repo.ExistsCollectionVersion(r.Context(), uuid, version, belongsTo); errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	// Entitlement scope is keyed by collection identity (uuid), not by
	// individual version -- every version under this uuid shares the same
	// decision, matching entitlement.resource_id's semantics (see
	// internal/db/migrations/0005_authz.sql).
	if !s.authorize(w, r, authz.CapCollectionRead, authz.Resource{CollectionUUID: uuid}) {
		return
	}
	revisions, err := s.repo.CollectionArtifactRevisions(r.Context(), uuid, version)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	etagParts := append([]string{"collection", belongsTo, uuid, strconv.Itoa(version)}, collectionArtifactETagParts(revisions)...)
	if s.conditional(w, r, cacheControlRevalidate, etagParts...) {
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
		httpx.BadRequestTyped(w, tea.ErrorInvalidRequest, "invalid uuid")
		return
	}
	// Every row this lists shares one collection identity (uuid) and thus
	// one entitlement scope -- a single check up front, not per-row
	// filtering (unlike the product/release list endpoints, whose rows are
	// each a distinct, independently-scoped identity). collection.read,
	// not .discover, since this returns full collection objects, not just
	// existence/minimal metadata.
	if !s.authorize(w, r, authz.CapCollectionRead, authz.Resource{CollectionUUID: uuid}) {
		return
	}

	// The endpoint prefix folds in belongsTo directly (rather than passing
	// it as a separate pageScope part) since it's really encoding *which
	// literal route* this is (/productRelease/.../collections vs
	// /componentRelease/.../collections), not a result-affecting filter
	// value.
	collectionsScope := "productRelease/collections"
	if belongsTo == repo.BelongsToComponentRelease {
		collectionsScope = "componentRelease/collections"
	}
	scope := pageScope(collectionsScope, uuid)
	pp, ok := parsePageParams(w, r, collectionSortFields, scope)
	if !ok {
		return
	}

	watermark, err := s.repo.GetWatermark(r.Context(), repo.WatermarkCollections)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if s.conditional(w, r, cacheControlRevalidate, "collections", belongsTo, uuid, pp.SortOrder, cursorPart(pp.Cursor), strconv.FormatInt(watermark, 10)) {
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
		resp.NextPageToken = nextPageToken(true, pp.SortField, pp.SortOrder, collectionSortValue(last), last.UUID, scope)
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

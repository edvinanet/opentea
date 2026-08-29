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
)

func (s *Server) getLatestArtifact(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}

	version, ok, err := s.repo.LatestArtifactVersion(r.Context(), uuid)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if !ok {
		httpx.NotFound(w)
		return
	}
	// The artifact's type is needed before the authz check (a template rule
	// can be constrained to one artifact classification -- spec Sec 17.1),
	// so it's fetched here rather than after the ETag short-circuit like
	// the rest of the object; the authz check must still happen before any
	// conditional-request response is written, so an unauthorized caller
	// never gets a 304 confirming the resource's current state (spec
	// Sec 18's existence-hiding).
	artifactType, err := s.repo.GetArtifactType(r.Context(), uuid, version)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	// No CollectionUUID: this is a direct-by-artifact-uuid fetch with no
	// collection context, so every collection referencing the artifact is
	// considered (spec Sec 17.2: a shared artifact is reachable via any one
	// authorized relationship).
	if !s.authorize(w, r, authz.CapArtifactMetadataRead, authz.Resource{ArtifactUUID: uuid, ArtifactType: authz.ArtifactType(artifactType)}) {
		return
	}

	revision, err := s.repo.GetArtifactRevision(r.Context(), uuid, version)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if s.conditional(w, r, cacheControlRevalidate, "artifact", uuid, strconv.Itoa(version), strconv.FormatInt(revision, 10)) {
		return
	}

	a, err := s.repo.GetArtifactLatest(r.Context(), uuid)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a)
}

func (s *Server) getArtifactByVersion(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	version, err := httpx.PathPositiveInt(r, "artifactVersion")
	if err != nil {
		httpx.BadRequest(w, "invalid artifactVersion")
		return
	}

	// See getLatestArtifact's comment on why the type is fetched before the
	// authz check, and why authz must precede the ETag/conditional check.
	artifactType, err := s.repo.GetArtifactType(r.Context(), uuid, version)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if !s.authorize(w, r, authz.CapArtifactMetadataRead, authz.Resource{ArtifactUUID: uuid, ArtifactType: authz.ArtifactType(artifactType)}) {
		return
	}

	// Not cacheControlImmutable, deliberately: unlike the proposal's
	// generic assumption, artifact revisions aren't strictly enforced
	// immutable in this codebase -- SetArtifactFormatFile mutates an
	// existing artifact_format row (the create-then-upload flow) after
	// creation, tracked via artifact.revision -- so this must still
	// revalidate, not cache forever.
	revision, err := s.repo.GetArtifactRevision(r.Context(), uuid, version)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if s.conditional(w, r, cacheControlRevalidate, "artifact", uuid, strconv.Itoa(version), strconv.FormatInt(revision, 10)) {
		return
	}

	a, err := s.repo.GetArtifactByVersion(r.Context(), uuid, version)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a)
}

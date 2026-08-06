package api

import (
	"errors"
	"net/http"
	"strconv"

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
	revision, err := s.repo.GetArtifactRevision(r.Context(), uuid, version)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	etag := httpx.BuildETag("artifact", uuid, strconv.Itoa(version), strconv.FormatInt(revision, 10))
	if httpx.WriteConditional(w, r, etag, cacheControlRevalidate) {
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
	etag := httpx.BuildETag("artifact", uuid, strconv.Itoa(version), strconv.FormatInt(revision, 10))
	if httpx.WriteConditional(w, r, etag, cacheControlRevalidate) {
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

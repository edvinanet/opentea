package admin

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/pkg/tea"
)

type artifactRefRequest struct {
	UUID    string `json:"uuid"`
	Version int    `json:"version"`
}

type createCollectionRequest struct {
	UpdateReason *tea.UpdateReason    `json:"updateReason,omitempty"`
	Artifacts    []artifactRefRequest `json:"artifacts"`
}

func (req createCollectionRequest) toInput() repo.CollectionInput {
	refs := make([]repo.ArtifactRef, len(req.Artifacts))
	for i, a := range req.Artifacts {
		version := a.Version
		if version == 0 {
			version = 1 // matches the artifact schema's default version
		}
		refs[i] = repo.ArtifactRef{UUID: a.UUID, Version: version}
	}
	return repo.CollectionInput{UpdateReason: req.UpdateReason, Artifacts: refs}
}

func (s *Server) createCollectionForComponentRelease(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	var req createCollectionRequest
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}

	c, err := s.repo.CreateCollectionForComponentRelease(r.Context(), uuid, req.toInput())
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, c)
}

func (s *Server) createCollectionForProductRelease(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	var req createCollectionRequest
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}

	c, err := s.repo.CreateCollectionForProductRelease(r.Context(), uuid, req.toInput())
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, c)
}

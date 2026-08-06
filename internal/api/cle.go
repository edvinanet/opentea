package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
)

func (s *Server) cleByProduct(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	if _, err := s.repo.GetProduct(r.Context(), uuid); errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	s.writeCLE(w, r, repo.OwnerProduct, uuid)
}

func (s *Server) cleByProductRelease(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	if _, err := s.repo.GetProductRelease(r.Context(), uuid); errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	s.writeCLE(w, r, repo.OwnerProductRelease, uuid)
}

func (s *Server) cleByComponent(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	if _, err := s.repo.GetComponent(r.Context(), uuid); errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	s.writeCLE(w, r, repo.OwnerComponent, uuid)
}

func (s *Server) cleByComponentRelease(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	if _, err := s.repo.GetComponentRelease(r.Context(), uuid); errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	s.writeCLE(w, r, repo.OwnerComponentRelease, uuid)
}

// writeCLE is called only after the caller has already confirmed ownerUUID
// itself exists (each of the 4 handlers above does its own existence check
// first), so unlike GetCLE it doesn't need one here -- GetCLERevision
// returns 0 (not an error) for an owner with no CLE data yet, matching
// GetCLE's own "empty CLE is valid" semantics.
func (s *Server) writeCLE(w http.ResponseWriter, r *http.Request, ownerType, ownerUUID string) {
	revision, err := s.repo.GetCLERevision(r.Context(), ownerType, ownerUUID)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	etag := httpx.BuildETag("cle", ownerType, ownerUUID, strconv.FormatInt(revision, 10))
	if httpx.WriteConditional(w, r, etag, cacheControlRevalidate) {
		return
	}

	cle, err := s.repo.GetCLE(r.Context(), ownerType, ownerUUID)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, cle)
}

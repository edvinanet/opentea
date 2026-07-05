package admin

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
)

func (s *Server) createComponent(w http.ResponseWriter, r *http.Request) {
	var req createProductRequest // same shape (name + identifiers) as product creation
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	if req.Name == "" {
		httpx.BadRequest(w, "name is required")
		return
	}
	c, err := s.repo.CreateComponent(r.Context(), req.Name, req.Identifiers)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, c)
}

func (s *Server) listComponents(w http.ResponseWriter, r *http.Request) {
	components, err := s.repo.QueryComponents(r.Context(), "", "", "name", "asc", nil, adminListLimit)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, components)
}

func (s *Server) getComponent(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
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

func (s *Server) deleteComponent(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	if err := s.repo.DeleteComponent(r.Context(), uuid); errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createComponentRelease(w http.ResponseWriter, r *http.Request) {
	componentUUID, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	var req releaseCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	if req.Version == "" {
		httpx.BadRequest(w, "version is required")
		return
	}
	if req.CreatedDate.IsZero() {
		httpx.BadRequest(w, "createdDate is required")
		return
	}

	cr, err := s.repo.CreateComponentRelease(r.Context(), componentUUID, repo.ComponentReleaseInput{
		Version:     req.Version,
		CreatedDate: req.CreatedDate,
		ReleaseDate: req.ReleaseDate,
		PreRelease:  req.PreRelease,
		Identifiers: req.Identifiers,
	})
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, cr)
}

func (s *Server) getComponentRelease(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
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
	httpx.WriteJSON(w, http.StatusOK, cr)
}

func (s *Server) deleteComponentRelease(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	if err := s.repo.DeleteComponentRelease(r.Context(), uuid); errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

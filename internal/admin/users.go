package admin

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/repo"
)

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	if req.Username == "" || req.Password == "" {
		httpx.BadRequest(w, "username and password are required")
		return
	}
	if req.Role != model.RoleAdmin && req.Role != model.RoleConsumer {
		httpx.BadRequest(w, "role must be \"admin\" or \"consumer\"")
		return
	}

	u, err := s.repo.CreateUser(r.Context(), req.Username, req.Password, req.Role)
	if errors.Is(err, repo.ErrUsernameTaken) {
		httpx.BadRequest(w, "username already taken")
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, u)
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.repo.ListUsers(r.Context())
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, users)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	err = s.repo.DeleteUser(r.Context(), uuid)
	switch {
	case errors.Is(err, repo.ErrNotFound):
		httpx.NotFound(w)
	case errors.Is(err, repo.ErrLastAdmin):
		httpx.BadRequest(w, "cannot delete the last admin user")
	case err != nil:
		httpx.InternalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

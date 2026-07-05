package admin

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
)

type createDistributionRequest struct {
	Description string `json:"description"`
}

func (s *Server) createDistribution(w http.ResponseWriter, r *http.Request) {
	componentReleaseUUID, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	var req createDistributionRequest
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}

	d, err := s.repo.CreateDistribution(r.Context(), componentReleaseUUID, req.Description)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, d)
}

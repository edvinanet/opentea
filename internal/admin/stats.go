package admin

import (
	"net/http"

	"github.com/oej/opentea/internal/httpx"
)

func (s *Server) getStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.repo.GetStats(r.Context())
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	stats.StartedAt = s.startedAt
	stats.OrgName = s.cfg.OrgName
	httpx.WriteJSON(w, http.StatusOK, stats)
}

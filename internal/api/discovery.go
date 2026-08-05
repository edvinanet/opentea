package api

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/pkg/tea"
)

// discoveryByTEI is a self-authoritative lookup (no federation in Phase 1):
// it resolves tei to a productReleaseUuid if this server hosts it, and
// always describes itself as the (only) server for that release.
func (s *Server) discoveryByTEI(w http.ResponseWriter, r *http.Request) {
	tei := r.URL.Query().Get("tei")
	if tei == "" {
		httpx.BadRequest(w, "tei query parameter is required")
		return
	}

	productReleaseUUID, err := s.repo.FindProductReleaseUUIDByTEI(r.Context(), tei)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.WriteJSON(w, http.StatusOK, []tea.DiscoveryInfo{})
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, []tea.DiscoveryInfo{
		{
			ProductReleaseUUID: productReleaseUUID,
			Servers: []tea.ServerInfo{
				{RootURL: s.cfg.RootURL, Versions: s.cfg.Versions},
			},
		},
	})
}

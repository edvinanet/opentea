// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package api

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/authz"
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
		// Upstream TEA 1.0 (spec/openapi.yaml, /discovery): "If the server
		// does not resolve the identifier, it responds with 404 and
		// error: OBJECT_UNKNOWN" -- no longer 200 with an empty array.
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}

	// An unauthorized match must be indistinguishable from no match at all
	// (spec Sec 18) -- discovery reveals a release's existence just as much
	// as a direct lookup would. Same 404+OBJECT_UNKNOWN response as the
	// true-no-match branch above, for the same reason.
	allowed, err := s.decide(r, authz.CapReleaseDiscover, authz.Resource{ProductReleaseUUID: productReleaseUUID})
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if !allowed {
		httpx.NotFound(w)
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

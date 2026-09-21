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

// discovery is a self-authoritative lookup (no federation in Phase 1): it
// resolves a TEI or a PURL to a productReleaseUuid if this server hosts
// it, and always describes itself as the (only) server for that release.
// Upstream TEA 1.0 (spec/openapi.yaml, /discovery) added purl alongside
// the original tei -- "Exactly one of the tei and purl query parameters
// shall be provided. A request with neither, or with both, is rejected
// with 400" -- purl resolves "within this TEA server's inventory" to the
// same target type (a product release) tei does, so both share every
// step here past the initial parameter resolution.
func (s *Server) discovery(w http.ResponseWriter, r *http.Request) {
	tei := r.URL.Query().Get("tei")
	purl := r.URL.Query().Get("purl")
	switch {
	case tei == "" && purl == "":
		httpx.BadRequestTyped(w, tea.ErrorInvalidRequest, "exactly one of the tei or purl query parameters is required")
		return
	case tei != "" && purl != "":
		httpx.BadRequestTyped(w, tea.ErrorInvalidRequest, "tei and purl query parameters are mutually exclusive")
		return
	}

	var productReleaseUUID string
	var err error
	if tei != "" {
		productReleaseUUID, err = s.repo.FindProductReleaseUUIDByTEI(r.Context(), tei)
	} else {
		productReleaseUUID, err = s.repo.FindProductReleaseUUIDByPURL(r.Context(), purl)
	}
	if errors.Is(err, repo.ErrNotFound) {
		// Upstream TEA 1.0: "If the server does not resolve the identifier,
		// it responds with 404 and error: OBJECT_UNKNOWN" -- no longer 200
		// with an empty array.
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

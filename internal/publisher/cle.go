// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package publisher

import (
	"net/http"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/pkg/teapublisher"
)

// createCLEEventForOwner implements createProductCLEEvent/
// createProductReleaseCLEEvent and the component/componentRelease
// equivalents (design/publisher-openapi.yaml) -- identical shape,
// different owner, mirroring internal/admin/cle.go's own
// createCLEEventForOwner exactly.
func (s *Server) createCLEEventForOwner(ownerType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ownerUUID, err := httpx.PathUUID(r, "uuid")
		if err != nil {
			httpx.BadRequest(w, "invalid uuid")
			return
		}
		var req teapublisher.CLEEventCreate
		if err := decodeJSON(r, &req); err != nil {
			httpx.BadRequest(w, "invalid JSON body: "+err.Error())
			return
		}
		if req.Type == "" {
			httpx.BadRequest(w, "type is required")
			return
		}
		if req.Effective.IsZero() || req.Published.IsZero() {
			httpx.BadRequest(w, "effective and published are required")
			return
		}

		e, err := s.repo.CreateCLEEvent(r.Context(), ownerType, ownerUUID, repo.CLEEventInput{
			Type:                req.Type,
			Effective:           req.Effective,
			Published:           req.Published,
			Version:             req.Version,
			Versions:            req.Versions,
			SupportID:           req.SupportID,
			License:             req.License,
			SupersededByVersion: req.SupersededByVersion,
			Identifiers:         req.Identifiers,
			EventID:             req.EventID,
			Reason:              req.Reason,
			Description:         req.Description,
			References:          req.References,
		})
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, e)
	}
}

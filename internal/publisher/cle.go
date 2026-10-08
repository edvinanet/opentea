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
			badRequest(w, r, teapublisher.ErrorInvalidPathParameter, "invalid uuid")
			return
		}
		var req teapublisher.CLEEventCreate
		if err := decodeJSON(r, &req); err != nil {
			badRequest(w, r, teapublisher.ErrorInvalidRequestBody, "invalid JSON body: "+err.Error())
			return
		}
		if req.Type == "" {
			badRequest(w, r, teapublisher.ErrorMissingField, "type is required", teapublisher.FieldError{Field: "type", Message: "is required"})
			return
		}
		var missing []teapublisher.FieldError
		if req.Effective.IsZero() {
			missing = append(missing, teapublisher.FieldError{Field: "effective", Message: "is required"})
		}
		if req.Published.IsZero() {
			missing = append(missing, teapublisher.FieldError{Field: "published", Message: "is required"})
		}
		if len(missing) > 0 {
			badRequest(w, r, teapublisher.ErrorMissingField, "effective and published are required", missing...)
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
		if writeIdentifierValidationError(w, r, err) {
			return
		}
		if err != nil {
			internalErr(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, e)
	}
}

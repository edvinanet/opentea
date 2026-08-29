// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package admin

import (
	"net/http"
	"time"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
)

type createArtifactFormatRequest struct {
	MediaType   string `json:"mediaType"`
	Description string `json:"description,omitempty"`
}

type createArtifactRequest struct {
	Name            string                        `json:"name,omitempty"`
	Type            string                        `json:"type"`
	CreatedDate     *time.Time                    `json:"createdDate,omitempty"`
	DistributionIDs []string                      `json:"distributionIds,omitempty"`
	Formats         []createArtifactFormatRequest `json:"formats"`
}

var validArtifactTypes = map[string]bool{
	"ATTESTATION": true, "BOM": true, "BUILD_META": true, "CERTIFICATION": true,
	"FORMULATION": true, "LICENSE": true, "RELEASE_NOTES": true, "SECURITY_TXT": true,
	"THREAT_MODEL": true, "VULNERABILITIES": true, "OTHER": true,
}

func (s *Server) createArtifact(w http.ResponseWriter, r *http.Request) {
	var req createArtifactRequest
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	if !validArtifactTypes[req.Type] {
		httpx.BadRequest(w, "type must be a valid artifact-type enum value")
		return
	}
	if len(req.Formats) == 0 {
		httpx.BadRequest(w, "at least one format is required")
		return
	}

	formats := make([]repo.ArtifactFormatInput, len(req.Formats))
	for i, f := range req.Formats {
		if f.MediaType == "" {
			httpx.BadRequest(w, "formats[].mediaType is required")
			return
		}
		formats[i] = repo.ArtifactFormatInput{MediaType: f.MediaType, Description: f.Description}
	}

	a, err := s.repo.CreateArtifact(r.Context(), repo.ArtifactInput{
		Name:            req.Name,
		Type:            req.Type,
		CreatedDate:     req.CreatedDate,
		DistributionIDs: req.DistributionIDs,
		Formats:         formats,
	})
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, a)
}

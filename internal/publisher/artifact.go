// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package publisher

import (
	"errors"
	"net/http"
	"time"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/pkg/teapublisher"
)

// validArtifactTypes mirrors internal/admin/artifact.go's own local
// duplicate of the spec's artifact-type enum (pkg/tea/enums.go's
// ArtifactType* constants) -- no shared exported validator exists for
// either package to reuse.
var validArtifactTypes = map[string]bool{
	"ATTESTATION": true, "BOM": true, "BUILD_META": true, "CERTIFICATION": true,
	"FORMULATION": true, "LICENSE": true, "RELEASE_NOTES": true, "SECURITY_TXT": true,
	"THREAT_MODEL": true, "VULNERABILITIES": true, "OTHER": true,
}

// createArtifact implements createArtifact: metadata only -- file content
// (uploadArtifactFile) and evidence (prepareArtifactEvidence/
// submitArtifactEvidence) are attached by separate operations.
// createdDate is server-assigned (design/publisher-openapi.yaml's
// artifact-create schema omits it from the request on purpose), set to
// now.
func (s *Server) createArtifact(w http.ResponseWriter, r *http.Request) {
	var req teapublisher.ArtifactCreate
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

	now := time.Now()
	a, err := s.repo.CreateArtifact(r.Context(), repo.ArtifactInput{
		Name:            req.Name,
		Type:            req.Type,
		CreatedDate:     &now,
		DistributionIDs: req.DistributionIDs,
		Formats:         formats,
	})
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, a)
}

const maxUploadBody = 1 << 30 // 1 GiB cap per uploaded file, matches internal/admin/upload.go

// uploadArtifactFile implements uploadArtifactFile -- close to
// internal/admin/upload.go's uploadArtifactFormatFile (server computes
// checksums from the uploaded bytes, a caller never supplies them
// directly; existence is confirmed before anything is persisted, same
// reasoning as internal/admin/upload.go's receiveFile doc comment), but
// addresses the target format by mediaType instead of admin's own
// formatIndex query param -- see the mediaType field's own comment below.
func (s *Server) uploadArtifactFile(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	version, err := httpx.PathPositiveInt(r, "version")
	if err != nil {
		httpx.BadRequest(w, "invalid version")
		return
	}

	artifact, err := s.repo.GetArtifactByVersion(r.Context(), uuid, version)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}

	// Reject once evidence exists for this artifact version -- otherwise a
	// re-upload after submitArtifactEvidence would silently change the
	// format's checksum out from under a signature that already attests to
	// the artifact's current (now stale) canonical form (found by external
	// security review, docs/security-review-publisher-design-260828.md
	// finding 1). A new revision belongs under a new artifact version, not
	// a file swap on an already-validated one.
	if _, err := s.repo.GetEvidenceBundleForOwner(r.Context(), "ARTIFACT", uuid, version); err == nil {
		httpx.Conflict(w, "this artifact version already has evidence submitted -- file content is frozen once validated")
		return
	} else if !errors.Is(err, repo.ErrNotFound) {
		httpx.InternalError(w, r, err)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBody)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httpx.BadRequest(w, "invalid multipart form: "+err.Error())
		return
	}
	// Addressed by mediaType, not a positional array index -- format order
	// is not a durable identifier (found by external security review,
	// docs/security-review-publisher-design-260828.md finding 14).
	mediaType := r.FormValue("mediaType")
	if mediaType == "" {
		httpx.BadRequest(w, "mediaType is required")
		return
	}
	formatIndex := -1
	for i, f := range artifact.Formats {
		if f.MediaType != mediaType {
			continue
		}
		if formatIndex != -1 {
			httpx.BadRequest(w, "mediaType is ambiguous: more than one format of this artifact version shares it")
			return
		}
		formatIndex = i
	}
	if formatIndex == -1 {
		httpx.NotFound(w)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.BadRequest(w, "missing \"file\" form field: "+err.Error())
		return
	}
	defer func() { _ = file.Close() }()

	sha256Hex, size, err := s.storage.Put(r.Context(), file)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if err := s.repo.UpsertBlob(r.Context(), sha256Hex, size, header.Header.Get("Content-Type")); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	url := s.cfg.RootURL + "/files/" + sha256Hex

	if _, err := s.repo.SetArtifactFormatFile(r.Context(), uuid, version, formatIndex, url, sha256Hex); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			httpx.NotFound(w)
			return
		}
		httpx.InternalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

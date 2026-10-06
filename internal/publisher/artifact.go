// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package publisher

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/pkg/tea"
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
	formats, ok := validateArtifactFields(w, req.Type, req.Formats)
	if !ok {
		return
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
	writeArtifactCreated(w, r, s, a)
}

// writeArtifactCreated writes a's ArtifactCreated response (finding 14):
// a's own creation already succeeded by the time this is called, so a
// failure looking up its format ids is reported as 500, not rolled back
// into the create itself -- the artifact and its formats already exist
// either way, this only affects what this one response body contains.
func writeArtifactCreated(w http.ResponseWriter, r *http.Request, s *Server, a tea.Artifact) {
	formatIDs, err := s.repo.ArtifactFormatIDs(r.Context(), a.UUID, a.Version)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, teapublisher.ArtifactCreated{Artifact: a, FormatIDs: formatIDs})
}

// maxArtifactFormats caps how many formats a single artifact (version) may
// declare -- external security review, docs/security-review-publisher-design-260828.md
// finding 14's "maximum format count": previously unbounded. Generously
// above any real CycloneDX-shaped use case (a handful of serializations of
// the same document -- JSON, XML, maybe a protobuf or two) while still
// ruling out an unbounded request inflating this artifact version's row
// count indefinitely.
const maxArtifactFormats = 50

// validateArtifactFields validates the type/formats fields shared by
// createArtifact's and createArtifactVersion's request bodies, converting
// formats to repo.ArtifactFormatInput on success. Writes the appropriate
// 400 response and returns ok=false on the first violation. Duplicate
// mediaType values across formats are deliberately still allowed (finding
// 14's "duplicate media-type handling", resolved this way rather than
// forbidden): each format now has its own stable, server-assigned id
// (ArtifactCreated.FormatIDs) a caller can address it by, so a shared
// mediaType no longer makes any format unreachable the way it did when
// mediaType was the only addressing option.
func validateArtifactFields(w http.ResponseWriter, artifactType string, in []teapublisher.ArtifactFormatCreate) (formats []repo.ArtifactFormatInput, ok bool) {
	if !validArtifactTypes[artifactType] {
		httpx.BadRequest(w, "type must be a valid artifact-type enum value")
		return nil, false
	}
	if len(in) == 0 {
		httpx.BadRequest(w, "at least one format is required")
		return nil, false
	}
	if len(in) > maxArtifactFormats {
		httpx.BadRequest(w, fmt.Sprintf("formats: at most %d allowed, got %d", maxArtifactFormats, len(in)))
		return nil, false
	}
	formats = make([]repo.ArtifactFormatInput, len(in))
	for i, f := range in {
		if f.MediaType == "" {
			httpx.BadRequest(w, "formats[].mediaType is required")
			return nil, false
		}
		formats[i] = repo.ArtifactFormatInput{MediaType: f.MediaType, Description: f.Description}
	}
	return formats, true
}

// createArtifactVersion implements createArtifactVersion: adds a new,
// server-numbered version under an already-existing artifact uuid --
// closing the gap identified by external security review (finding 4,
// docs/security-review-publisher-design-260828.md) that createArtifact
// above could only ever mint a brand new artifact identity, never a
// correction/update to one that already exists
// (design/publisher-service.md §6 scenario 3, "SBOM correction"). Nothing
// is carried forward from the prior version automatically -- see
// repo.CreateArtifactVersion's doc comment for why: the request supplies
// every field fresh, exactly like createArtifact's own body.
func (s *Server) createArtifactVersion(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}

	var req teapublisher.ArtifactVersionCreate
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	formats, ok := validateArtifactFields(w, req.Type, req.Formats)
	if !ok {
		return
	}

	now := time.Now()
	a, err := s.repo.CreateArtifactVersion(r.Context(), uuid, req.PreviousVersion, repo.ArtifactInput{
		Name:            req.Name,
		Type:            req.Type,
		CreatedDate:     &now,
		DistributionIDs: req.DistributionIDs,
		Formats:         formats,
	})
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if errors.Is(err, repo.ErrVersionConflict) {
		httpx.Conflict(w, err.Error())
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	writeArtifactCreated(w, r, s, a)
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
	formatIndex, ok := s.resolveArtifactFormat(w, r, artifact, uuid, version)
	if !ok {
		return
	}
	sha256Hex, size, ok := s.receiveUploadedFile(w, r)
	if !ok {
		return
	}
	// No url is published for self-hosted content -- TEA 1.0 reserves that
	// field for genuinely external locations (spec/openapi.yaml); this
	// server's own content is instead retrieved via the artifact download
	// endpoints (internal/api/artifactdownload.go), resolved from the
	// checksum SetArtifactFormatFile records.
	if _, err := s.repo.SetArtifactFormatFile(r.Context(), uuid, version, formatIndex, sha256Hex); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			httpx.NotFound(w)
			return
		}
		httpx.InternalError(w, r, err)
		return
	}
	// Returns the checksum and size the server actually computed from the
	// uploaded bytes, not just 204 -- external security review,
	// docs/security-review-publisher-design-260828.md finding 14: "checksum
	// and size returned after upload," previously neither was.
	httpx.WriteJSON(w, http.StatusOK, teapublisher.ArtifactFileUploaded{SHA256: sha256Hex, Size: size})
}

// resolveArtifactFormat resolves the target format of an already-parsed
// multipart upload request against the stable, server-assigned formatId
// form field if present, falling back to the mediaType form field
// otherwise (security review finding 14,
// docs/security-review-publisher-design-260828.md: "array order is not a
// durable identifier... use a server-assigned format ID or stable format
// key"). formatId is unambiguous even when two formats share a mediaType
// (allowed, see validateArtifactFields's own doc comment); mediaType stays
// supported for the common case where a caller doesn't need to -- or
// hasn't kept -- the id ArtifactCreated returned at creation time.
func (s *Server) resolveArtifactFormat(w http.ResponseWriter, r *http.Request, artifact tea.Artifact, artifactUUID string, artifactVersion int) (formatIndex int, ok bool) {
	if formatID := r.FormValue("formatId"); formatID != "" {
		index, err := s.repo.ArtifactFormatIndexByID(r.Context(), artifactUUID, artifactVersion, formatID)
		if errors.Is(err, repo.ErrNotFound) {
			httpx.NotFound(w)
			return -1, false
		}
		if err != nil {
			httpx.InternalError(w, r, err)
			return -1, false
		}
		return index, true
	}
	mediaType := r.FormValue("mediaType")
	if mediaType == "" {
		httpx.BadRequest(w, "formatId or mediaType is required")
		return -1, false
	}
	return resolveArtifactFormatIndex(w, artifact, mediaType)
}

// resolveArtifactFormatIndex finds the index of the one format among
// artifact.Formats whose MediaType matches mediaType exactly -- not a
// positional array index (security review finding 14, docs/security-
// review-publisher-design-260828.md). Writes the appropriate error
// response and returns ok=false if none match (404) or more than one does
// (400, ambiguous -- a caller with a genuinely ambiguous mediaType must
// use formatId instead, see resolveArtifactFormat).
func resolveArtifactFormatIndex(w http.ResponseWriter, artifact tea.Artifact, mediaType string) (formatIndex int, ok bool) {
	formatIndex = -1
	for i, f := range artifact.Formats {
		if f.MediaType != mediaType {
			continue
		}
		if formatIndex != -1 {
			httpx.BadRequest(w, "mediaType is ambiguous: more than one format of this artifact version shares it -- use formatId instead")
			return -1, false
		}
		formatIndex = i
	}
	if formatIndex == -1 {
		httpx.NotFound(w)
		return -1, false
	}
	return formatIndex, true
}

// receiveUploadedFile extracts the "file" multipart field (already parsed
// by ParseMultipartForm), stores it via the blob store, and records blob
// bookkeeping -- shared by uploadArtifactFile and uploadArtifactSignatureFile,
// which differ only in what they do with the resulting sha256Hex.
func (s *Server) receiveUploadedFile(w http.ResponseWriter, r *http.Request) (sha256Hex string, size int64, ok bool) {
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.BadRequest(w, "missing \"file\" form field: "+err.Error())
		return "", 0, false
	}
	defer func() { _ = file.Close() }()

	sha256Hex, size, err = s.storage.Put(r.Context(), file)
	if err != nil {
		httpx.InternalError(w, r, err)
		return "", 0, false
	}
	if err := s.repo.UpsertBlob(r.Context(), sha256Hex, size, header.Header.Get("Content-Type")); err != nil {
		httpx.InternalError(w, r, err)
		return "", 0, false
	}
	return sha256Hex, size, true
}

// uploadArtifactSignatureFile mirrors uploadArtifactFile, addressing the
// target format by mediaType the same way, but stores the uploaded bytes as
// the format's detached signature (artifact_format.signature_sha256)
// instead of its content. Unlike uploadArtifactFile, this is not blocked by
// an already-submitted evidence bundle: evidence attests to the format's
// content checksum, which a signature upload never changes.
func (s *Server) uploadArtifactSignatureFile(w http.ResponseWriter, r *http.Request) {
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

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBody)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httpx.BadRequest(w, "invalid multipart form: "+err.Error())
		return
	}
	formatIndex, ok := s.resolveArtifactFormat(w, r, artifact, uuid, version)
	if !ok {
		return
	}
	sha256Hex, size, ok := s.receiveUploadedFile(w, r)
	if !ok {
		return
	}
	if _, err := s.repo.SetArtifactFormatSignatureFile(r.Context(), uuid, version, formatIndex, sha256Hex); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			httpx.NotFound(w)
			return
		}
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, teapublisher.ArtifactFileUploaded{SHA256: sha256Hex, Size: size})
}

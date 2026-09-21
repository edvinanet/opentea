// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package admin

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
)

const maxUploadBody = 1 << 30 // 1 GiB cap per uploaded file

// receiveFile extracts the "file" multipart field, stores it via the blob
// store, records blob bookkeeping (for /files/{sha256}'s Content-Type), and
// returns the resulting download URL and sha256.
func (s *Server) receiveFile(w http.ResponseWriter, r *http.Request) (url, sha256Hex string, ok bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBody)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httpx.BadRequest(w, "invalid multipart form: "+err.Error())
		return "", "", false
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.BadRequest(w, "missing \"file\" form field: "+err.Error())
		return "", "", false
	}
	defer func() { _ = file.Close() }()

	sha256Hex, size, err := s.storage.Put(r.Context(), file)
	if err != nil {
		httpx.InternalError(w, r, err)
		return "", "", false
	}

	mediaType := header.Header.Get("Content-Type")
	if err := s.repo.UpsertBlob(r.Context(), sha256Hex, size, mediaType); err != nil {
		httpx.InternalError(w, r, err)
		return "", "", false
	}

	return s.cfg.RootURL + "/files/" + sha256Hex, sha256Hex, true
}

func (s *Server) uploadDistributionFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		httpx.BadRequest(w, "missing distribution id")
		return
	}

	// Confirm the target exists before receiveFile persists anything --
	// otherwise an upload to a bad id always stores (and permanently
	// orphans, since nothing then references it) a blob whose attach step
	// was never going to succeed. See TODO.md for the remaining, harder
	// to close window (a real error/disconnect between receiveFile and
	// SetDistributionFile below).
	if _, err := s.repo.GetDistribution(r.Context(), id); errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}

	url, sha256Hex, ok := s.receiveFile(w, r)
	if !ok {
		return
	}

	d, err := s.repo.SetDistributionFile(r.Context(), id, url, sha256Hex)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}

func (s *Server) uploadArtifactFormatFile(w http.ResponseWriter, r *http.Request) {
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
	formatIndex := 0
	if v := r.URL.Query().Get("formatIndex"); v != "" {
		formatIndex, err = strconv.Atoi(v)
		if err != nil || formatIndex < 0 {
			httpx.BadRequest(w, "invalid formatIndex")
			return
		}
	}

	// Confirm the target artifact revision and formatIndex exist before
	// receiveFile persists anything -- same reasoning as
	// uploadDistributionFile above.
	artifact, err := s.repo.GetArtifactByVersion(r.Context(), uuid, version)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if formatIndex >= len(artifact.Formats) {
		httpx.NotFound(w)
		return
	}

	// receiveFile's returned URL is discarded here -- TEA 1.0's url field is
	// reserved for genuinely external locations (spec/openapi.yaml); this
	// server's own /files/{sha256} link is no longer published as it, and
	// self-hosted content is instead retrieved via the artifact download
	// endpoints (internal/api/artifactdownload.go), resolved from the
	// checksum SetArtifactFormatFile records.
	_, sha256Hex, ok := s.receiveFile(w, r)
	if !ok {
		return
	}

	a, err := s.repo.SetArtifactFormatFile(r.Context(), uuid, version, formatIndex, sha256Hex)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a)
}

// uploadArtifactFormatSignatureFile mirrors uploadArtifactFormatFile exactly,
// but stores the uploaded bytes as the format's detached signature
// (artifact_format.signature_sha256) instead of its content.
func (s *Server) uploadArtifactFormatSignatureFile(w http.ResponseWriter, r *http.Request) {
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
	formatIndex := 0
	if v := r.URL.Query().Get("formatIndex"); v != "" {
		formatIndex, err = strconv.Atoi(v)
		if err != nil || formatIndex < 0 {
			httpx.BadRequest(w, "invalid formatIndex")
			return
		}
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
	if formatIndex >= len(artifact.Formats) {
		httpx.NotFound(w)
		return
	}

	_, sha256Hex, ok := s.receiveFile(w, r)
	if !ok {
		return
	}

	a, err := s.repo.SetArtifactFormatSignatureFile(r.Context(), uuid, version, formatIndex, sha256Hex)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a)
}

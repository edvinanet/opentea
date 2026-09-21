// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// artifactdownload.go implements TEA 1.0's four artifact download
// endpoints (spec/openapi.yaml): /artifact/{uuid}/latest/download,
// /artifact/{uuid}/{version}/download, and their /signature/download
// counterparts. Content that has no external url (TEA 1.0 reserves that
// field for genuinely external locations -- see internal/repo/artifact.go's
// SetArtifactFormatFile) is served from this server's own blob storage,
// the same storage.Storage/Repo.GetBlobMediaType machinery
// internal/files/handler.go already uses for /files/{sha256} -- these
// endpoints are the spec-conformant replacement for reaching that content,
// not a second independent implementation of it.
package api

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/oej/opentea/internal/authz"
	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/pkg/tea"
)

// requireGetOrHead wraps next so it only runs for GET/HEAD -- see
// router.go's own comment on why these routes are registered without a
// method prefix (a Go ServeMux registration conflict between a literal
// "latest" pattern and a wildcard {artifactVersion} one across methods)
// and therefore need this guard done by hand instead.
func requireGetOrHead(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next(w, r)
	}
}

func (s *Server) downloadLatestArtifact(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequestTyped(w, tea.ErrorInvalidRequest, "invalid uuid")
		return
	}
	version, ok, err := s.repo.LatestArtifactVersion(r.Context(), uuid)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if !ok {
		httpx.NotFound(w)
		return
	}
	s.downloadArtifactContent(w, r, uuid, version, cacheControlRevalidate)
}

func (s *Server) downloadArtifactByVersion(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequestTyped(w, tea.ErrorInvalidRequest, "invalid uuid")
		return
	}
	version, err := httpx.PathPositiveInt(r, "artifactVersion")
	if err != nil {
		httpx.BadRequestTyped(w, tea.ErrorInvalidRequest, "invalid artifactVersion")
		return
	}
	// Not cacheControlImmutable, deliberately, same reasoning as
	// getArtifactByVersion (artifact.go): artifact revisions aren't
	// strictly enforced immutable in this codebase (the create-then-upload
	// flow can still add a format's content after creation), so even the
	// versioned endpoint must still revalidate.
	s.downloadArtifactContent(w, r, uuid, version, cacheControlRevalidate)
}

func (s *Server) downloadLatestArtifactSignature(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequestTyped(w, tea.ErrorInvalidRequest, "invalid uuid")
		return
	}
	version, ok, err := s.repo.LatestArtifactVersion(r.Context(), uuid)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if !ok {
		httpx.NotFound(w)
		return
	}
	s.downloadArtifactSignature(w, r, uuid, version, cacheControlRevalidate)
}

func (s *Server) downloadArtifactSignatureByVersion(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequestTyped(w, tea.ErrorInvalidRequest, "invalid uuid")
		return
	}
	version, err := httpx.PathPositiveInt(r, "artifactVersion")
	if err != nil {
		httpx.BadRequestTyped(w, tea.ErrorInvalidRequest, "invalid artifactVersion")
		return
	}
	s.downloadArtifactSignature(w, r, uuid, version, cacheControlRevalidate)
}

// downloadArtifactContent is downloadLatestArtifact/downloadArtifactByVersion's
// shared implementation, parameterized over the already-resolved version
// (the "latest" variant resolves it before calling in, so both variants
// share every step past that point, same convention as
// getLatestArtifact/getArtifactByVersion in artifact.go).
func (s *Server) downloadArtifactContent(w http.ResponseWriter, r *http.Request, uuid string, version int, cacheControlBase string) {
	artifactType, err := s.repo.GetArtifactType(r.Context(), uuid, version)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	// authz must precede everything below -- including format negotiation,
	// which would otherwise leak which formats exist to a caller who isn't
	// even authorized to know the artifact exists (spec Sec 18).
	if !s.authorize(w, r, authz.CapArtifactDownload, authz.Resource{ArtifactUUID: uuid, ArtifactType: authz.ArtifactType(artifactType)}) {
		return
	}

	a, err := s.repo.GetArtifactByVersion(r.Context(), uuid, version)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}

	mediaType, usedAccept, ok := selectFormatMediaType(r, a.Formats)
	if !ok {
		httpx.NotAcceptable(w, "no format matches the requested mediaType or Accept header")
		return
	}

	sha256Hex, externalURL, err := s.repo.GetArtifactFormatForDownload(r.Context(), uuid, version, mediaType)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if errors.Is(err, repo.ErrNoMatchingFormat) {
		// selectFormatMediaType already matched a real format by this exact
		// value, so this shouldn't happen -- defensive, not expected.
		httpx.NotAcceptable(w, "no format matches the requested mediaType or Accept header")
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}

	// 302 redirects are outside conditional semantics (TEA 1.0: "If-None-Match
	// applies only to the TEA-hosted download response, not to following an
	// external Location") -- redirect unconditionally, before any ETag is
	// ever computed for this representation.
	if externalURL != "" {
		http.Redirect(w, r, externalURL, http.StatusFound)
		return
	}

	revision, err := s.repo.GetArtifactRevision(r.Context(), uuid, version)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if usedAccept {
		w.Header().Set("Vary", "Accept")
	}
	contentLocation := s.artifactDownloadLocation(uuid, version, mediaType)
	w.Header().Set("Content-Location", contentLocation)
	w.Header().Set("Content-Disposition", contentDispositionFor(a.Name, mediaType))
	if s.conditional(w, r, cacheControlBase, "artifact-download", uuid, strconv.Itoa(version), mediaType, strconv.FormatInt(revision, 10)) {
		return
	}

	s.streamBlob(w, r, sha256Hex, mediaType)
}

func (s *Server) downloadArtifactSignature(w http.ResponseWriter, r *http.Request, uuid string, version int, cacheControlBase string) {
	artifactType, err := s.repo.GetArtifactType(r.Context(), uuid, version)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if !s.authorize(w, r, authz.CapArtifactDownload, authz.Resource{ArtifactUUID: uuid, ArtifactType: authz.ArtifactType(artifactType)}) {
		return
	}

	a, err := s.repo.GetArtifactByVersion(r.Context(), uuid, version)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}

	mediaType, _, ok := selectFormatMediaType(r, a.Formats)
	if !ok {
		httpx.NotAcceptable(w, "no format matches the requested mediaType")
		return
	}

	sha256Hex, externalURL, err := s.repo.GetArtifactFormatForSignatureDownload(r.Context(), uuid, version, mediaType)
	// A concealed/nonexistent artifact must answer OBJECT_UNKNOWN for every
	// sub-resource, signatures included (TEA 1.0's own text) -- ErrNotFound
	// and ErrNoMatchingFormat both map to the same NotFound() here, unlike
	// ErrSignatureNotFound below, which is deliberately a different, more
	// specific 404.
	if errors.Is(err, repo.ErrNotFound) || errors.Is(err, repo.ErrNoMatchingFormat) {
		httpx.NotFound(w)
		return
	}
	if errors.Is(err, repo.ErrSignatureNotFound) {
		httpx.NotFoundTyped(w, tea.ErrorSignatureNotFound)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}

	if externalURL != "" {
		http.Redirect(w, r, externalURL, http.StatusFound)
		return
	}

	revision, err := s.repo.GetArtifactRevision(r.Context(), uuid, version)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	contentLocation := s.artifactSignatureDownloadLocation(uuid, version, mediaType)
	w.Header().Set("Content-Location", contentLocation)
	w.Header().Set("Content-Disposition", contentDispositionFor(a.Name+".sig", "application/octet-stream"))
	if s.conditional(w, r, cacheControlBase, "artifact-signature-download", uuid, strconv.Itoa(version), mediaType, strconv.FormatInt(revision, 10)) {
		return
	}

	s.streamBlob(w, r, sha256Hex, "application/octet-stream")
}

// streamBlob writes sha256Hex's stored bytes as the response body, or
// nothing beyond headers for a HEAD request -- mirrors
// internal/files/handler.go's own serve, reused rather than duplicated
// where the storage/lookup calls are identical; Content-Type here is the
// format's own declared, trusted mediaType (unlike /files/{sha256}, which
// treats an uploader-supplied Content-Type as untrusted and forces
// nosniff/attachment for that reason -- Content-Disposition is still set
// by the caller above for the spec's own suggested-filename requirement,
// not as an XSS defense here).
func (s *Server) streamBlob(w http.ResponseWriter, r *http.Request, sha256Hex, contentType string) {
	f, err := s.storage.Open(r.Context(), sha256Hex)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	defer func() { _ = f.Close() }()

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	if _, err := io.Copy(w, f); err != nil {
		// Headers/status are already sent; nothing more to do but this
		// matches internal/files/handler.go's own reasoning for not
		// treating a broken mid-stream write as a fresh error response.
		return
	}
}

func (s *Server) artifactDownloadLocation(uuid string, version int, mediaType string) string {
	return s.cfg.RootURL + s.cfg.APIBasePath + "/artifact/" + uuid + "/" + strconv.Itoa(version) +
		"/download?mediaType=" + url.QueryEscape(mediaType)
}

func (s *Server) artifactSignatureDownloadLocation(uuid string, version int, mediaType string) string {
	return s.cfg.RootURL + s.cfg.APIBasePath + "/artifact/" + uuid + "/" + strconv.Itoa(version) +
		"/signature/download?mediaType=" + url.QueryEscape(mediaType)
}

// selectFormatMediaType picks which of formats to serve: an explicit
// mediaType query parameter always wins (matched case-insensitively,
// TEA 1.0: "case-insensitive for the type and subtype"); otherwise the
// request's Accept header is consulted (a bare, pragmatic first pass --
// the first format whose mediaType appears in Accept's comma-separated
// list wins, ignoring q-value weighting; not full RFC 9110 section 12
// content negotiation). usedAccept reports whether Accept (rather than an
// explicit mediaType) drove the result, for the caller to decide whether
// to send Vary: Accept. ok is false if formats is empty or nothing
// matches.
func selectFormatMediaType(r *http.Request, formats []tea.ArtifactFormat) (mediaType string, usedAccept bool, ok bool) {
	if q := r.URL.Query().Get("mediaType"); q != "" {
		for _, f := range formats {
			if strings.EqualFold(f.MediaType, q) {
				return f.MediaType, false, true
			}
		}
		return "", false, false
	}

	accept := r.Header.Get("Accept")
	if accept == "" || strings.Contains(accept, "*/*") {
		if len(formats) == 0 {
			return "", false, false
		}
		return formats[0].MediaType, false, true
	}
	candidates := strings.Split(accept, ",")
	for i := range candidates {
		if semi := strings.IndexByte(candidates[i], ';'); semi >= 0 {
			candidates[i] = candidates[i][:semi]
		}
		candidates[i] = strings.TrimSpace(candidates[i])
	}
	for _, f := range formats {
		for _, c := range candidates {
			if strings.EqualFold(f.MediaType, c) {
				return f.MediaType, true, true
			}
		}
	}
	return "", false, false
}

// contentDispositionFor builds a best-effort suggested filename (TEA 1.0:
// optional, not required) from name if set, otherwise a generic one with
// an extension guessed from mediaType via the stdlib mime package -- never
// blocks a download on not having a good name.
func contentDispositionFor(name, mediaType string) string {
	filename := name
	if filename == "" {
		filename = "artifact"
		if exts, err := mime.ExtensionsByType(mediaType); err == nil && len(exts) > 0 {
			filename += exts[0]
		}
	}
	filename = strings.NewReplacer(`"`, "", "\r", "", "\n", "").Replace(filename)
	return `attachment; filename="` + filename + `"`
}

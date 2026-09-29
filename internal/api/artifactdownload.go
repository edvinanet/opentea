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
	"encoding/base64"
	"encoding/hex"
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

// downloadLatestArtifact uses cacheControlLatest, not cacheControlRevalidate:
// "latest" resolves to a different revision the moment a new one is
// published, and the spec requires revalidation before reuse for that
// reason specifically -- stale-while-revalidate (cacheControlRevalidate's
// whole point) is exactly what that forbids
// (docs/security-review-260923.md finding #13).
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
	s.downloadArtifactContent(w, r, uuid, version, cacheControlLatest)
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
	// Not a long-lived, never-revalidate cache policy, deliberately, same
	// reasoning as getArtifactByVersion (artifact.go): artifact revisions
	// aren't strictly enforced immutable in this codebase (the create-
	// then-upload flow can still add a format's content after creation),
	// so even the versioned endpoint must still revalidate.
	s.downloadArtifactContent(w, r, uuid, version, cacheControlRevalidate)
}

// downloadLatestArtifactSignature uses cacheControlLatest -- see
// downloadLatestArtifact's own doc comment for why.
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
	s.downloadArtifactSignature(w, r, uuid, version, cacheControlLatest)
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
	if digest, ok := reprDigestHeader(sha256Hex); ok {
		w.Header().Set("Repr-Digest", digest)
	}
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

	mediaType, ok := selectSignatureFormatMediaType(r, a.Formats)
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
	if digest, ok := reprDigestHeader(sha256Hex); ok {
		w.Header().Set("Repr-Digest", digest)
	}
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

// reprDigestHeader builds an RFC 9530 Repr-Digest structured-field
// dictionary value for sha256Hex -- the format/signature content's own
// SHA-256 hash, the one algorithm this codebase always self-computes (see
// internal/repo's own reasoning on why only SHA-256 is independently
// verified for uploaded content). RFC 9530 dictionary members are byte
// sequences, base64-encoded (`sf-binary`), not hex -- distinct from TEA's
// own checksum.algValue, which is lowercase hex; the two encodings must
// not be confused. Only set for self-hosted content (never for a 302
// redirect to an external url/signatureUrl -- there's no local
// representation for this server to digest there). ok is false only if
// sha256Hex somehow isn't valid hex, which should never happen for a value
// this codebase stored itself; skip the header rather than emit a
// malformed one.
func reprDigestHeader(sha256Hex string) (value string, ok bool) {
	raw, err := hex.DecodeString(sha256Hex)
	if err != nil {
		return "", false
	}
	return "sha-256=:" + base64.StdEncoding.EncodeToString(raw) + ":", true
}

func (s *Server) artifactDownloadLocation(uuid string, version int, mediaType string) string {
	return s.cfg.RootURL + s.cfg.APIBasePath + "/artifact/" + uuid + "/" + strconv.Itoa(version) +
		"/download?mediaType=" + url.QueryEscape(mediaType)
}

func (s *Server) artifactSignatureDownloadLocation(uuid string, version int, mediaType string) string {
	return s.cfg.RootURL + s.cfg.APIBasePath + "/artifact/" + uuid + "/" + strconv.Itoa(version) +
		"/signature/download?mediaType=" + url.QueryEscape(mediaType)
}

// acceptRange is one parsed entry from an Accept header's comma-separated
// list of media ranges (RFC 9110 section 12.5.1): typ/subtype (each "*"
// for a wildcard), plus its q weight (default 1; an explicit q=0 means
// "not acceptable at all", RFC 9110 section 12.4.2). Parameters other
// than q are ignored when matching -- no artifact mediaType this codebase
// serves ever carries type-level parameters (charset and the like) for
// those to distinguish.
type acceptRange struct {
	typ, subtype string
	q            float64
}

// parseAccept parses an Accept header's comma-separated media ranges. A
// malformed range or qvalue is skipped/defaulted rather than failing the
// whole header -- lenient parsing, matching how HTTP servers commonly
// treat this request header in practice; RFC 9110 sets no server-side
// hard-failure requirement for a malformed Accept this permissive.
func parseAccept(header string) []acceptRange {
	var ranges []acceptRange
	for _, part := range strings.Split(header, ",") {
		segments := strings.Split(part, ";")
		typ, subtype, found := strings.Cut(strings.TrimSpace(segments[0]), "/")
		if !found || typ == "" || subtype == "" {
			continue
		}
		q := 1.0
		for _, param := range segments[1:] {
			name, value, found := strings.Cut(param, "=")
			if !found || !strings.EqualFold(strings.TrimSpace(name), "q") {
				continue
			}
			// q, when present, is always the first accept-param (RFC 9110
			// section 12.5.1) -- an unparseable value defaults to 1 (the
			// same as if q were absent) rather than excluding the range.
			if parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
				q = parsed
			}
			break
		}
		ranges = append(ranges, acceptRange{typ: strings.ToLower(typ), subtype: strings.ToLower(subtype), q: q})
	}
	return ranges
}

// bestAcceptQuality returns the quality of the most specific range in
// ranges that matches mediaType -- an exact type/subtype match outranks a
// type/* match, which outranks */* (RFC 9110 section 12.5.1: "a more
// specific reference has precedence"). matched is false if no range
// matches at all, or if the single most specific matching range has q=0
// (explicitly not acceptable) -- either way, mediaType is not a candidate,
// and the caller must not silently fall back to it.
func bestAcceptQuality(ranges []acceptRange, mediaType string) (q float64, matched bool) {
	typ, subtype, found := strings.Cut(mediaType, "/")
	if !found {
		return 0, false
	}
	typ, subtype = strings.ToLower(typ), strings.ToLower(subtype)

	bestSpecificity := -1
	for _, rg := range ranges {
		var specificity int
		switch {
		case rg.typ == typ && rg.subtype == subtype:
			specificity = 2
		case rg.typ == typ && rg.subtype == "*":
			specificity = 1
		case rg.typ == "*" && rg.subtype == "*":
			specificity = 0
		default:
			continue
		}
		if specificity > bestSpecificity {
			bestSpecificity = specificity
			q = rg.q
		}
	}
	if bestSpecificity < 0 {
		return 0, false
	}
	return q, q > 0
}

// selectFormatMediaType picks which of formats to serve for the regular
// (non-signature) download endpoints (docs/security-review-260923.md
// finding #12): an explicit mediaType query parameter always wins
// (matched case-insensitively, TEA 1.0: "case-insensitive for the type
// and subtype"); otherwise the request's Accept header is consulted per
// RFC 9110 section 12 -- the highest-quality, most-specific matching
// format wins. A format excluded via q=0 is never selected even if no
// other format matches: the spec parameter doc's "falls back to a format
// of its choice when Accept does not constrain the result" describes an
// absent/unconstraining Accept, not one that explicitly excludes
// everything offered -- those two cases must not be conflated. usedAccept
// reports whether Accept negotiation was in play (mediaType was absent)
// -- true even when Accept itself is also absent, since a client sending
// a specific Accept next time could still get a different result (the
// spec parameter doc: "the server shall include Accept in Vary... including
// when Accept is absent or contains */*"). ok is false if formats is
// empty or nothing is acceptable.
func selectFormatMediaType(r *http.Request, formats []tea.ArtifactFormat) (mediaType string, usedAccept bool, ok bool) {
	if q := r.URL.Query().Get("mediaType"); q != "" {
		for _, f := range formats {
			if strings.EqualFold(f.MediaType, q) {
				return f.MediaType, false, true
			}
		}
		return "", false, false
	}
	if len(formats) == 0 {
		return "", true, false
	}

	accept := r.Header.Get("Accept")
	if accept == "" {
		return formats[0].MediaType, true, true
	}

	ranges := parseAccept(accept)
	bestIdx := -1
	var bestQ float64
	for i, f := range formats {
		q, matched := bestAcceptQuality(ranges, f.MediaType)
		if !matched {
			continue
		}
		if bestIdx == -1 || q > bestQ {
			bestIdx, bestQ = i, q
		}
	}
	if bestIdx == -1 {
		return "", true, false
	}
	return formats[bestIdx].MediaType, true, true
}

// selectSignatureFormatMediaType picks which of formats a signature
// download applies to: an explicit mediaType query parameter always wins,
// the same matching rule as selectFormatMediaType. When omitted, this
// parameter's own spec text is explicit that Accept plays no role at all
// -- "the server selects a format of its choice" -- unlike the content
// download endpoints, this deliberately does not consult Accept
// (docs/security-review-260923.md finding #12: signature downloads
// previously called selectFormatMediaType directly, incorrectly
// inheriting its Accept-driven selection for a parameter whose spec text
// says otherwise). ok is false if formats is empty or an explicit
// mediaType matches nothing.
func selectSignatureFormatMediaType(r *http.Request, formats []tea.ArtifactFormat) (mediaType string, ok bool) {
	if q := r.URL.Query().Get("mediaType"); q != "" {
		for _, f := range formats {
			if strings.EqualFold(f.MediaType, q) {
				return f.MediaType, true
			}
		}
		return "", false
	}
	if len(formats) == 0 {
		return "", false
	}
	return formats[0].MediaType, true
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

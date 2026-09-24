// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Package files serves uploaded blobs back out over HTTP at GET /files/{sha256}.
package files

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/oej/opentea/internal/authn"
	"github.com/oej/opentea/internal/authz"
	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/internal/storage"
)

// Handler serves stored blobs back out over HTTP.
type Handler struct {
	repo    *repo.Repo
	storage storage.Storage
}

// NewHandler builds the /files/{sha256} mux.
func NewHandler(r *repo.Repo, s storage.Storage) http.Handler {
	h := &Handler{repo: r, storage: s}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /files/{sha256}", h.serve)
	return mux
}

// sha256Pattern matches a blob's only valid path-parameter shape (every
// blob is stored and looked up by its lowercase hex SHA-256, see
// internal/storage). Rejecting anything else before it reaches the
// filesystem-path-building storage layer or an HTTP response header (see
// serve below) closes off both path-traversal and header-injection
// surfaces that an unvalidated path segment would otherwise open.
var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) {
	sha256Hex := r.PathValue("sha256")
	if !sha256Pattern.MatchString(sha256Hex) {
		http.NotFound(w, r)
		return
	}

	mediaType, err := h.repo.GetBlobMediaType(r.Context(), sha256Hex)
	if errors.Is(err, repo.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}

	if !h.authorized(w, r, sha256Hex) {
		return
	}

	f, err := h.storage.Open(r.Context(), sha256Hex)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()

	if mediaType != "" {
		w.Header().Set("Content-Type", mediaType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	// mediaType came from an upload's Content-Type header or, for an
	// imported bundle, a manifest field declared by whatever remote TEA
	// server it was imported from -- either way, it's not something this
	// server controls or validates. Serving it inline would let a crafted
	// value like "text/html" or "image/svg+xml" execute as a stored XSS in
	// this same origin (shared with /admin/ui and /admin/v1's session
	// cookie). Content-Disposition: attachment makes the browser download
	// rather than render/execute the response regardless of Content-Type;
	// nosniff additionally stops the browser from guessing its own,
	// possibly more dangerous, type for the small window before a download
	// prompt would apply anyway.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", `attachment; filename="`+sha256Hex+`"`)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable") // content-addressed, safe to cache forever
	if _, err := io.Copy(w, f); err != nil {
		// Response headers/status are already sent; nothing more we can do
		// but log for operators.
		slog.Error("stream blob failed", "sha256", sha256Hex, "error", err)
	}
}

// authorized enforces the same artifact.download capability the versioned
// download endpoints do (internal/api/artifactdownload.go), resolved
// through whichever artifact(s) reference sha256Hex as content or
// signature (repo.FindBlobArtifactOwners) -- a blob has no identity of its
// own to check authz against, so this reaches it through its owning
// resources instead, the same "any one authorized relationship" principle
// collection/artifact sharing already relies on. A blob with no owning
// artifact at all (for example a release distribution's file -- TEA
// distributions aren't Artifacts and have no download capability defined
// for them, see TODO.md) is denied by default rather than served: this
// handler's job is to gate access to artifact content, not to be a
// general-purpose anonymous file host.
//
// Principal resolution mirrors internal/api's resolvePrincipal exactly
// (same present/valid split, same 401 on an invalid token) -- duplicated
// rather than shared because /files/ is a separate top-level route, not
// part of the cfg.APIBasePath-scoped /tea/v1 router that middleware wraps.
//
// Returns true if the caller should continue serving the blob, false if
// authorized has already written a response and the caller must return
// immediately.
func (h *Handler) authorized(w http.ResponseWriter, r *http.Request, sha256Hex string) bool {
	user, present, valid := authn.BearerUser(r.Context(), r, h.repo)
	if present && !valid {
		httpx.UnauthorizedBearer(w, "invalid_token", "invalid or expired bearer token")
		return false
	}
	principal := authz.Principal{}
	if present && valid {
		principal = authz.Principal{UserUUID: user.UUID}
	}

	owners, err := h.repo.FindBlobArtifactOwners(r.Context(), sha256Hex)
	if err != nil {
		httpx.InternalError(w, r, err)
		return false
	}

	for _, owner := range owners {
		decision, err := authz.Decide(r.Context(), h.repo, principal, authz.CapArtifactDownload,
			authz.Resource{ArtifactUUID: owner.ArtifactUUID, ArtifactType: authz.ArtifactType(owner.ArtifactType)})
		if err != nil {
			httpx.InternalError(w, r, err)
			return false
		}
		if decision.Allowed {
			return true
		}
	}

	// TEA 1.0's own 401-unauthorized text: "a protected object shall not
	// answer 404 solely because the client is unauthenticated" -- same
	// split internal/api's writeAuthzDenial applies.
	if !principal.IsAuthenticated() {
		httpx.UnauthorizedBearer(w, "", "authentication required")
		return false
	}
	http.NotFound(w, r)
	return false
}

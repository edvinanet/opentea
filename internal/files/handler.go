// Package files serves uploaded blobs back out over HTTP at GET /files/{sha256}.
package files

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"

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

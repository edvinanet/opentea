// Package files serves uploaded blobs back out over HTTP at GET /files/{sha256}.
package files

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

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

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) {
	sha256Hex := r.PathValue("sha256")

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
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable") // content-addressed, safe to cache forever
	if _, err := io.Copy(w, f); err != nil {
		// Response headers/status are already sent; nothing more we can do
		// but log for operators.
		slog.Error("stream blob failed", "sha256", sha256Hex, "error", err)
	}
}

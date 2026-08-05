// Package admin implements the unofficial internal ingestion API
// (/admin/v1/...) used to load data into the server until a real TEA
// publisher API exists. It is not part of the spec.
package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/oej/opentea/internal/config"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/internal/storage"
)

// Server holds the dependencies for the /admin/v1 API handlers.
type Server struct {
	repo      *repo.Repo
	storage   storage.Storage
	cfg       config.Config
	startedAt time.Time
}

// NewRouter builds the /admin/v1/... mux. startedAt is the server process's
// own start time (captured once in main, not re-derived here), surfaced by
// GET /admin/v1/stats for monitoring.
func NewRouter(r *repo.Repo, s storage.Storage, cfg config.Config, startedAt time.Time) http.Handler {
	srv := &Server{repo: r, storage: s, cfg: cfg, startedAt: startedAt}
	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	return mux
}

const maxJSONBody = 10 << 20 // 10 MiB, generous for metadata payloads

func decodeJSON(r *http.Request, v any) error {
	defer func() { _ = r.Body.Close() }()
	return json.NewDecoder(io.LimitReader(r.Body, maxJSONBody)).Decode(v)
}

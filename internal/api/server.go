// Package api implements the spec-conformant TEA consumer read API
// (mounted at /tea/v1, matching spec/openapi.yaml v0.4.0 exactly).
package api

import (
	"net/http"

	"github.com/oej/opentea/internal/config"
	"github.com/oej/opentea/internal/repo"
)

// Server holds the dependencies for the /tea/v1 consumer read API handlers.
type Server struct {
	repo *repo.Repo
	cfg  config.Config
}

// NewRouter builds the /tea/v1/... mux.
func NewRouter(r *repo.Repo, cfg config.Config) http.Handler {
	srv := &Server{repo: r, cfg: cfg}
	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	return optionalBearerAuth(r, mux)
}

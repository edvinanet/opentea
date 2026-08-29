// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Package api implements the spec-conformant TEA consumer read API
// (matching spec/openapi.yaml v0.4.0 exactly), mounted at
// config.Config.APIBasePath -- "/tea/v1" by default, configurable via
// TEA_API_BASE_PATH.
package api

import (
	"net/http"

	"github.com/oej/opentea/internal/config"
	"github.com/oej/opentea/internal/repo"
)

// Server holds the dependencies for the consumer read API handlers.
type Server struct {
	repo *repo.Repo
	cfg  config.Config
}

// NewRouter builds the consumer API's mux, under cfg.APIBasePath.
func NewRouter(r *repo.Repo, cfg config.Config) http.Handler {
	srv := &Server{repo: r, cfg: cfg}
	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	return resolvePrincipal(r, mux)
}

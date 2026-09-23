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
	"github.com/oej/opentea/internal/storage"
)

// Server holds the dependencies for the consumer read API handlers.
type Server struct {
	repo    *repo.Repo
	storage storage.Storage
	cfg     config.Config
}

// NewRouter builds the consumer API's mux, under cfg.APIBasePath. storage
// is the same blob store /files/{sha256} (internal/files) uses -- the
// artifact download endpoints (artifactdownload.go) serve self-hosted
// content from it directly, not through that other, non-standard handler.
func NewRouter(r *repo.Repo, s storage.Storage, cfg config.Config) http.Handler {
	srv := &Server{repo: r, storage: s, cfg: cfg}
	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	return resolvePrincipal(r, cfg.APIBasePath+"/token", mux)
}

// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Package publisher implements opentea's server-side half of the draft
// standard TEA Publisher API: /publisher/v1, as sketched in
// design/publisher-openapi.yaml and design/publisher-service.md. Unlike
// internal/admin (this project's own, non-standard ingestion surface),
// this is meant to be implementable by any conformant TEA server -- the
// same relationship /tea/v1 (internal/api) has to the official TEA spec.
//
// Authenticated by a publisher_credential bearer token (see
// auth_middleware.go), not the admin session cookie internal/admin uses --
// a /publisher/v1 caller is a publisher platform (a GUI service or a
// reference CLI client embedded in CI/CD), not a logged-in human operator.
// Two credential scopes: "full" (every operation) and "cicd" (artifact
// create/upload/evidence, and collection-draft assembly/prepare/commit --
// deliberately excludes product/component/release/CLE creation and
// approve/reject, design/publisher-service.md §10.4/§14.3).
//
// Mounted on the management plane alongside /admin/v1 and /admin/ui
// (design/opentea-server.md §6/§7.2/§8.4) -- it shares the optional
// TEA_ADMIN_LISTEN_ADDR listener split, not the public consumer listener.
package publisher

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/oej/opentea/internal/config"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/internal/storage"
)

// Server holds the dependencies for the /publisher/v1 API handlers.
type Server struct {
	repo    *repo.Repo
	storage storage.Storage
	cfg     config.Config
}

// NewRouter builds the /publisher/v1/... mux.
func NewRouter(r *repo.Repo, s storage.Storage, cfg config.Config) http.Handler {
	srv := &Server{repo: r, storage: s, cfg: cfg}
	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	return mux
}

const maxJSONBody = 10 << 20 // 10 MiB, matches internal/admin's own limit

func decodeJSON(r *http.Request, v any) error {
	defer func() { _ = r.Body.Close() }()
	return json.NewDecoder(io.LimitReader(r.Body, maxJSONBody)).Decode(v)
}

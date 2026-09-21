// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Package admin implements the unofficial internal ingestion API
// (/admin/v1/...) used to load data into the server until a real TEA
// publisher API exists. It is not part of the spec.
package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/oej/opentea/internal/config"
	"github.com/oej/opentea/internal/httpx"
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

// writeIdentifierValidationError writes a 400 for the identifier
// validation errors insertIdentifiers/insertCLEEventIdentifiers may
// return (TEA 1.0's COMPLIANCE_DOCUMENT rules, internal/repo/identifier.go),
// reporting whether it wrote one -- every handler that creates/imports
// identifiers checks this before falling through to httpx.InternalError.
func writeIdentifierValidationError(w http.ResponseWriter, err error) bool {
	if errors.Is(err, repo.ErrComplianceDocumentWrongOwner) || errors.Is(err, repo.ErrInvalidComplianceDocumentType) {
		httpx.BadRequest(w, err.Error())
		return true
	}
	return false
}

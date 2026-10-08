// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package publisher

import (
	"log/slog"
	"net/http"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/pkg/teapublisher"
)

// writeError writes status with a structured teapublisher.ErrorResponse
// body -- every /publisher/v1 error path in this package goes through this
// (or one of the status-specific wrappers below) instead of calling
// internal/httpx's generic, untyped helpers directly (external security
// review, docs/security-review-publisher-design-260828.md finding 15).
// requestId is pulled from r's context (httpx.RequestID) -- already set for
// every request this server handles, since /publisher/v1 shares
// cmd/opentea/main.go's registerAdminRoutes, which wraps its whole mux in
// httpx.WithRequestID.
func writeError(w http.ResponseWriter, r *http.Request, status int, code string, retryable bool, message string, fields []teapublisher.FieldError) {
	httpx.WriteJSON(w, status, teapublisher.ErrorResponse{
		Code:      code,
		Message:   message,
		RequestID: httpx.RequestID(r.Context()),
		Retryable: retryable,
		Fields:    fields,
	})
}

// badRequest writes a 400 with the given stable code -- never retryable, a
// resubmission of the identical request fails identically until the
// caller changes something.
func badRequest(w http.ResponseWriter, r *http.Request, code, message string, fields ...teapublisher.FieldError) {
	writeError(w, r, http.StatusBadRequest, code, false, message, fields)
}

// notFoundErr writes a 404. /publisher/v1 has no existence-hiding
// requirement (unlike /tea/v1's anonymous-reader threat model -- every
// caller here already holds an authenticated publisher credential), so
// this carries the structured body too, not a bare status.
func notFoundErr(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, http.StatusNotFound, teapublisher.ErrorNotFound, false, "no such resource", nil)
}

// conflictErr writes a 409 with the given stable code.
func conflictErr(w http.ResponseWriter, r *http.Request, code, message string) {
	writeError(w, r, http.StatusConflict, code, false, message, nil)
}

// forbiddenErr writes a 403 with the given stable code -- ErrorForbidden
// for an insufficient-scope credential, ErrorSelfApproval for the
// maker-checker case, kept distinguishable per finding 15 rather than both
// collapsing into one generic code.
func forbiddenErr(w http.ResponseWriter, r *http.Request, code, message string) {
	writeError(w, r, http.StatusForbidden, code, false, message, nil)
}

// unauthorizedErr writes a 401.
func unauthorizedErr(w http.ResponseWriter, r *http.Request, message string) {
	writeError(w, r, http.StatusUnauthorized, teapublisher.ErrorUnauthorized, false, message, nil)
}

// internalErr logs err server-side (mirrors internal/httpx.InternalError's
// own log line exactly) and writes a structured 500 with no internal detail
// leaked -- the only error this package ever marks Retryable: a transient
// server-side failure is the one case where resubmitting the identical
// request might simply work the second time.
func internalErr(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("internal server error", "method", r.Method, "path", r.URL.Path, "error", err)
	writeError(w, r, http.StatusInternalServerError, teapublisher.ErrorInternal, true, "internal server error", nil)
}

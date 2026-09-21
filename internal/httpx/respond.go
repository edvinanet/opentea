// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Package httpx holds small HTTP response, request-parsing, and request
// helpers shared across this project's handler packages (api, admin,
// webadmin) and cmd/opentea's server wiring.
package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/oej/opentea/pkg/tea"
)

// WriteJSON writes v as a JSON response body with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json response failed", "error", err)
	}
}

type messageBody struct {
	Message string `json:"message"`
}

// BadRequest writes a 400 response with a plain {"message": ...} body --
// internal/admin's and internal/publisher's own convention, and
// internal/webadmin's; neither is bound to spec/openapi.yaml's
// error-response schema (internal/admin/internal/publisher aren't part of
// the spec at all). internal/api (the literal /tea/v1 surface the spec
// does govern) uses BadRequestTyped instead, below.
func BadRequest(w http.ResponseWriter, message string) {
	WriteJSON(w, http.StatusBadRequest, messageBody{Message: message})
}

// BadRequestTyped writes a 400 response matching the spec's error-response
// schema (TEA 1.0, spec/openapi.yaml): errType must be one of the
// tea.Error* unknown-error-type constants. For internal/api's use --
// admin/publisher/webadmin keep calling the untyped BadRequest above.
func BadRequestTyped(w http.ResponseWriter, errType, message string) {
	WriteJSON(w, http.StatusBadRequest, tea.ErrorResponse{Error: errType, Message: message})
}

// NotFound writes a 404 response matching the spec's error-response schema.
func NotFound(w http.ResponseWriter) {
	WriteJSON(w, http.StatusNotFound, tea.ErrorResponse{Error: tea.ErrorObjectUnknown})
}

// Unauthorized writes a 401 response -- used both when a session/bearer
// credential is missing where required, and when one was supplied but
// didn't resolve to a valid identity. TEA 1.0's error-response schema does
// apply to 401 responses ("any 4xx from a resource endpoint may carry an
// error-response body"), but its unknown-error-type enum has no value for
// "missing/invalid credential" -- deliberately left untyped rather than
// forcing an ill-fitting one; a body is optional here regardless
// ("clients shall not require a body").
func Unauthorized(w http.ResponseWriter, message string) {
	WriteJSON(w, http.StatusUnauthorized, messageBody{Message: message})
}

// Forbidden writes a 403 response -- used when the caller is authenticated
// but their role doesn't grant the required access.
func Forbidden(w http.ResponseWriter, message string) {
	WriteJSON(w, http.StatusForbidden, messageBody{Message: message})
}

// Conflict writes a 409 response -- used when a request is individually
// well-formed but rejected because of the resource's current state (e.g. a
// locked collection draft, a stale approval).
func Conflict(w http.ResponseWriter, message string) {
	WriteJSON(w, http.StatusConflict, messageBody{Message: message})
}

// InternalError logs err server-side (with request context) and writes a
// generic 500 response with no internal detail leaked to the client.
func InternalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("internal server error", "method", r.Method, "path", r.URL.Path, "error", err)
	WriteJSON(w, http.StatusInternalServerError, messageBody{Message: "internal server error"})
}

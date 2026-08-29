// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package httpx

import (
	"context"
	"net/http"

	"github.com/oej/opentea/internal/idgen"
)

// RequestIDHeader is the header a request-id travels on, both inbound (a
// caller-supplied correlation id, honored if present) and outbound (always
// set on the response, so a caller who didn't supply one can still
// correlate against server-side logs/audit records afterward).
const RequestIDHeader = "X-Request-Id"

// maxRequestIDLen bounds an inbound caller-supplied request id -- generous
// enough for any real correlation-id scheme (UUIDs, trace ids), but not
// unbounded, since this value gets echoed back in the response header and
// written into admin_audit_log/authorization decision log lines.
const maxRequestIDLen = 200

type requestIDContextKey struct{}

// WithRequestID wraps next so every request has a request id available via
// RequestID: the caller's own X-Request-Id header value if it supplied one
// (letting a client's own trace id thread through), otherwise a freshly
// generated one. Always echoes the resolved id back on the response header,
// and stashes it in context for downstream handlers (admin audit records,
// authorization decision logging) to pick up.
func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if id == "" || len(id) > maxRequestIDLen {
			id = idgen.New()
		}
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDContextKey{}, id)))
	})
}

// RequestID returns the current request's id, or "" if ctx didn't pass
// through WithRequestID (shouldn't happen for a real request, but callers
// outside the HTTP path -- tests, background jobs -- have no request to
// correlate, so an empty string is a valid, harmless value here rather than
// a panic).
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey{}).(string)
	return id
}

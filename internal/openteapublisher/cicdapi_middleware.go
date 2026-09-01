// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"errors"
	"net/http"
	"strings"

	"github.com/oej/opentea/internal/httpx"
)

// cicdHandler is like http.HandlerFunc but also receives the resolved
// credential's target UUID -- every /cicdapi/v1 handler needs it to build
// a teapublisherclient.Client against that target.
type cicdHandler func(w http.ResponseWriter, r *http.Request, targetUUID string)

// requireCICDCredential wraps next so it only runs for a request bearing
// a valid, unrevoked cicd_credential -- otherwise it writes a 401 JSON
// response. No session cookie, no SameOrigin CSRF check: this is a
// service bearer credential, not a browser session -- mirrors
// internal/publisher/auth_middleware.go's own requireScope reasoning
// exactly (bearer requests aren't CSRF targets).
func (s *Server) requireCICDCredential(next cicdHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := cicdBearerToken(r)
		if !ok {
			httpx.Unauthorized(w, "missing or malformed Authorization header")
			return
		}
		cred, err := s.repo.GetCICDCredentialByToken(r.Context(), token)
		if errors.Is(err, ErrNotFound) {
			httpx.Unauthorized(w, "invalid or revoked bearer token")
			return
		}
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		next(w, r, cred.TargetUUID)
	}
}

func cicdBearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(h, prefix))
	if token == "" {
		return "", false
	}
	return token, true
}

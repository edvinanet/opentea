// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package publisher

import (
	"errors"
	"net/http"
	"strings"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/repo"
)

// scopeSatisfies reports whether a credential issued with credScope may
// call an operation that requires minScope. "full" satisfies both scopes;
// "cicd" only satisfies "cicd" -- mirrors internal/authn.RoleSatisfies'
// shape for a different, two-value vocabulary
// (model.PublisherScopeFull/PublisherScopeCICD).
func scopeSatisfies(credScope, minScope string) bool {
	if credScope == model.PublisherScopeFull {
		return true
	}
	return credScope == minScope
}

// requireScope wraps next so it only runs for a request bearing a valid,
// unrevoked publisher credential whose scope satisfies minScope --
// otherwise it writes a 401 (missing/invalid/revoked token) or 403
// (valid token, insufficient scope) JSON response. No session cookie, no
// SameOrigin check: this is a service bearer credential, not a browser
// session (internal/authn.SameOrigin's own doc comment: bearer requests
// aren't CSRF targets).
func (s *Server) requireScope(minScope string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			httpx.Unauthorized(w, "missing or malformed Authorization header")
			return
		}
		cred, err := s.repo.GetPublisherCredentialByToken(r.Context(), token)
		if errors.Is(err, repo.ErrNotFound) {
			httpx.Unauthorized(w, "invalid or revoked bearer token")
			return
		}
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		if !scopeSatisfies(cred.Scope, minScope) {
			httpx.Forbidden(w, "credential scope does not permit this operation")
			return
		}
		next(w, r)
	}
}

func bearerToken(r *http.Request) (string, bool) {
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

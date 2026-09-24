// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package publisher

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/repo"
)

// credentialContextKey is an unexported type so this package's context key
// can't collide with any key set by another package -- mirrors
// internal/api/principal.go's withPrincipal/principalFromContext pattern.
type credentialContextKey struct{}

// withCredential returns a context carrying cred, resolved once by
// requireScope and read by handlers that need to know which credential
// actually authenticated the request -- not just what scope it holds.
// Collection-draft maker-checker enforcement is the one place this
// matters today (docs/security-review-260923.md finding #4): a
// caller-supplied JSON "actor" string alone never established two
// independent credentials, only two arbitrary strings the same caller
// typed.
func withCredential(ctx context.Context, cred model.PublisherCredential) context.Context {
	return context.WithValue(ctx, credentialContextKey{}, cred)
}

// credentialFromContext returns the resolved credential, or the zero value
// if none was set (shouldn't happen for a request that passed through
// requireScope, but the zero value's empty UUID is safe: it can never
// equal a real drafted-by-credential UUID, so it can't accidentally
// satisfy or defeat a self-approval check).
func credentialFromContext(ctx context.Context) model.PublisherCredential {
	cred, _ := ctx.Value(credentialContextKey{}).(model.PublisherCredential)
	return cred
}

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
		next(w, r.WithContext(withCredential(r.Context(), cred)))
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

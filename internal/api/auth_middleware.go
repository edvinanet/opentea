// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package api

import (
	"net/http"

	"github.com/oej/opentea/internal/authn"
	"github.com/oej/opentea/internal/authz"
	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
)

// resolvePrincipal implements the spec's default "unauthenticated read
// access" model with one addition: if a request supplies an Authorization
// header, it must resolve to a valid access token or the request is
// rejected. Absence of the header is always fine (anonymous, per spec).
// Unlike the optionalBearerAuth this replaces, the resolved user is no
// longer discarded -- it's stashed in request context as an
// authz.Principal via withPrincipal, for every handler's authz.Decide call
// to read back via principalFromContext.
//
// tokenPath (cfg.APIBasePath+"/token") is exempted entirely: that
// operation authenticates its caller via HTTP Basic (a client's API key),
// a wholly different scheme from the Bearer access tokens this middleware
// checks -- treating a Basic header there as a malformed bearer attempt
// would reject every /token request with a bad Authorization header
// before requestToken ever saw it. /token performs its own auth
// (internal/api/token.go) and needs no principal.
func resolvePrincipal(r *repo.Repo, tokenPath string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == tokenPath {
			next.ServeHTTP(w, req.WithContext(withPrincipal(req.Context(), authz.Principal{})))
			return
		}
		user, present, valid := authn.BearerUser(req.Context(), req, r)
		if present && !valid {
			httpx.UnauthorizedBearer(w, "invalid_token", "invalid or expired bearer token")
			return
		}
		principal := authz.Principal{}
		if present && valid {
			principal = authz.Principal{UserUUID: user.UUID}
		}
		next.ServeHTTP(w, req.WithContext(withPrincipal(req.Context(), principal)))
	})
}

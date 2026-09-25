// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package api

import (
	"net/http"

	"github.com/oej/opentea/internal/authn"
	"github.com/oej/opentea/internal/authz"
	"github.com/oej/opentea/internal/repo"
)

// resolvePrincipal implements the spec's default "unauthenticated read
// access" model: an absent Authorization header is always fine (anonymous,
// per spec), and -- per the updated auth text, "a public endpoint ignores
// a presented token, valid or not" -- so is a present-but-invalid/expired
// one; it resolves to the anonymous principal rather than rejecting the
// request outright, since whether an invalid token should actually block
// access depends on whether the specific resource being requested turns
// out to require authentication at all, which isn't known yet at this
// middleware layer (docs/security-review-260923.md finding #15: this
// previously rejected with 401 unconditionally, breaking access to a
// resource that would have been public to an anonymous caller with no
// token at all). withInvalidBearer records that an invalid token was
// presented so writeAuthzDenial can still surface that specifically
// (rather than a generic "authentication required") if the resource does
// turn out to require authentication -- see its own doc comment. The
// resolved user is stashed in request context as an authz.Principal via
// withPrincipal, for every handler's authz.Decide call to read back via
// principalFromContext.
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
		ctx := req.Context()
		principal := authz.Principal{}
		switch {
		case present && valid:
			principal = authz.Principal{UserUUID: user.UUID}
		case present && !valid:
			ctx = withInvalidBearer(ctx)
		}
		next.ServeHTTP(w, req.WithContext(withPrincipal(ctx, principal)))
	})
}

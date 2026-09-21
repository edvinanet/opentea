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
// header, it must resolve to a valid API token or the request is rejected.
// Absence of the header is always fine (anonymous, per spec). Unlike the
// optionalBearerAuth this replaces, the resolved user is no longer
// discarded -- it's stashed in request context as an authz.Principal via
// withPrincipal, for every handler's authz.Decide call to read back via
// principalFromContext.
func resolvePrincipal(r *repo.Repo, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
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

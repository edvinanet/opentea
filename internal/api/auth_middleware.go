package api

import (
	"net/http"

	"github.com/oej/opentea/internal/authn"
	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
)

// optionalBearerAuth implements the spec's default "unauthenticated read
// access" model with one addition: if a request supplies an Authorization
// header, it must resolve to a valid API token or the request is rejected.
// Absence of the header is always fine (anonymous, per spec).
func optionalBearerAuth(r *repo.Repo, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, present, valid := authn.BearerUser(req.Context(), req, r)
		if present && !valid {
			httpx.Unauthorized(w, "invalid or expired bearer token")
			return
		}
		next.ServeHTTP(w, req)
	})
}

// Package authn centralizes session-cookie and bearer-token resolution
// shared by the JSON admin API (internal/admin), the HTML admin GUI
// (internal/webadmin), and the spec-conformant read API (internal/api).
// Response-format-specific enforcement (JSON 401/403 vs HTML redirect,
// required vs optional) stays in each of those packages.
package authn

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/repo"
)

const (
	// SessionCookieName is the cookie name used for /admin/ui browser sessions.
	SessionCookieName = "opentea_session"
	// SessionTTL is how long a session stays valid after login.
	SessionTTL = 24 * time.Hour
)

// RoleSatisfies reports whether userRole grants access requiring minRole.
// "admin" satisfies both "admin" and "consumer"; "consumer" only satisfies "consumer".
func RoleSatisfies(userRole, minRole string) bool {
	if userRole == model.RoleAdmin {
		return true
	}
	return userRole == minRole
}

// SessionUser resolves the logged-in user from the session cookie on r, if
// there is a valid, unexpired session.
func SessionUser(ctx context.Context, r *http.Request, store *repo.Repo) (model.User, bool) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return model.User{}, false
	}
	user, err := store.GetSessionUser(ctx, cookie.Value)
	if err != nil {
		return model.User{}, false
	}
	return user, true
}

// BearerUser resolves a user from an `Authorization: Bearer <token>` header.
//
// present=false means no such header was sent at all -- the caller should
// treat the request as anonymous (allowed, per the TEA spec's default
// unauthenticated-read model).
//
// present=true, valid=false means a header was sent but didn't resolve to a
// real token -- the caller must reject the request.
func BearerUser(ctx context.Context, r *http.Request, store *repo.Repo) (user model.User, present, valid bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return model.User{}, false, false
	}
	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || token == "" {
		return model.User{}, true, false
	}
	u, err := store.GetUserByAPIToken(ctx, token)
	if errors.Is(err, repo.ErrNotFound) {
		return model.User{}, true, false
	}
	if err != nil {
		return model.User{}, true, false
	}
	return u, true, true
}

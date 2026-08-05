// Package authn centralizes session-cookie and bearer-token resolution
// shared by the JSON admin API (internal/admin), the HTML admin GUI
// (internal/webadmin), and the spec-conformant read API (internal/api).
// Response-format-specific enforcement (JSON 401/403 vs HTML redirect,
// required vs optional) stays in each of those packages.
package authn

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
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
// there is a valid, unexpired session. A missing/unknown/expired cookie is
// silently treated as anonymous (expected, high-volume, not worth logging);
// any other repo error (DB unavailable, corrupted row, etc.) still resolves
// to "no user" -- callers must fail closed either way -- but is logged, so
// an outage doesn't get silently misdiagnosed as a wave of client mistakes.
func SessionUser(ctx context.Context, r *http.Request, store *repo.Repo) (model.User, bool) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return model.User{}, false
	}
	user, err := store.GetSessionUser(ctx, cookie.Value)
	if errors.Is(err, repo.ErrNotFound) {
		return model.User{}, false
	}
	if err != nil {
		slog.Error("session lookup failed", "method", r.Method, "path", r.URL.Path, "error", err)
		return model.User{}, false
	}
	return user, true
}

// SameOrigin reports whether r's declared origin matches rootURL's -- a
// CSRF defense-in-depth check for state-changing requests authenticated by
// an ambient session cookie (the only auth mechanism internal/admin and
// internal/webadmin's requireRole support; SameSite=Lax on that cookie
// already blocks most cross-site POSTs, but not same-site/subdomain
// attacks, so callers should still run this check before acting on a
// non-safe-method request). Bearer-token requests aren't in scope: a
// forged cross-site request can't attach a header the victim's browser
// wouldn't send on its own, so they're not CSRF targets and don't need
// this check.
//
// GET/HEAD/OPTIONS are always allowed (assumed side-effect-free, so not a
// CSRF target); other methods require an Origin header (falling back to
// Referer, since some non-browser or older-browser requests only send
// that) whose scheme+host match rootURL's -- absent or mismatched is
// rejected.
func SameOrigin(r *http.Request, rootURL string) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	if origin == "" {
		return false
	}

	want, err := url.Parse(rootURL)
	if err != nil {
		return false
	}
	got, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return got.Scheme == want.Scheme && got.Host == want.Host
}

// BearerUser resolves a user from an `Authorization: Bearer <token>` header.
//
// present=false means no such header was sent at all -- the caller should
// treat the request as anonymous (allowed, per the TEA spec's default
// unauthenticated-read model).
//
// present=true, valid=false means a header was sent but didn't resolve to a
// real token -- the caller must reject the request. An unknown token is
// silently treated this way (expected, high-volume, not worth logging); any
// other repo error (DB unavailable, corrupted row, etc.) still resolves to
// valid=false -- callers must fail closed either way -- but is logged, so
// an outage doesn't get silently misdiagnosed as a wave of bad tokens.
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
		slog.Error("bearer token lookup failed", "method", r.Method, "path", r.URL.Path, "error", err)
		return model.User{}, true, false
	}
	return u, true, true
}

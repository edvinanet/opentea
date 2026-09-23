// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package authn

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oej/opentea/internal/db"
	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/repo"
)

// captureLogs redirects the default slog logger to a buffer for the
// duration of the test, restoring the previous logger via t.Cleanup.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func newTestRepo(t *testing.T) *repo.Repo {
	t.Helper()
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return repo.New(sqlDB)
}

func TestRoleSatisfies(t *testing.T) {
	cases := []struct {
		userRole, minRole string
		want              bool
	}{
		{model.RoleAdmin, model.RoleAdmin, true},
		{model.RoleAdmin, model.RoleConsumer, true},
		{model.RoleConsumer, model.RoleConsumer, true},
		{model.RoleConsumer, model.RoleAdmin, false},
	}
	for _, c := range cases {
		if got := RoleSatisfies(c.userRole, c.minRole); got != c.want {
			t.Errorf("RoleSatisfies(%q, %q) = %v, want %v", c.userRole, c.minRole, got, c.want)
		}
	}
}

func TestSessionUser(t *testing.T) {
	ctx := context.Background()
	store := newTestRepo(t)

	user, err := store.CreateUser(ctx, "alice", "password1", model.RoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, _, err := store.CreateSession(ctx, user.UUID, SessionTTL)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// No cookie at all.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, ok := SessionUser(ctx, req, store); ok {
		t.Fatal("expected no user without a cookie")
	}

	// Valid cookie.
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	got, ok := SessionUser(ctx, req, store)
	if !ok || got.UUID != user.UUID {
		t.Fatalf("SessionUser = %+v, ok=%v, want user %s", got, ok, user.UUID)
	}

	// Garbage cookie value.
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "not-a-real-token"})
	if _, ok := SessionUser(ctx, req, store); ok {
		t.Fatal("expected no user for an invalid session token")
	}
}

func TestBearerUser(t *testing.T) {
	ctx := context.Background()
	store := newTestRepo(t)

	user, err := store.CreateUser(ctx, "alice", "password1", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, _, err := store.CreateAccessToken(ctx, user.UUID, time.Hour)
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}

	// No Authorization header: anonymous, allowed.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, present, _ := BearerUser(ctx, req, store); present {
		t.Fatal("expected present=false with no Authorization header")
	}

	// Malformed header (no "Bearer " prefix).
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "garbage")
	if _, present, valid := BearerUser(ctx, req, store); !present || valid {
		t.Fatalf("present=%v valid=%v, want present=true valid=false", present, valid)
	}

	// Valid-looking but unknown token.
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	if _, present, valid := BearerUser(ctx, req, store); !present || valid {
		t.Fatalf("present=%v valid=%v, want present=true valid=false", present, valid)
	}

	// Real token.
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	got, present, valid := BearerUser(ctx, req, store)
	if !present || !valid || got.UUID != user.UUID {
		t.Fatalf("BearerUser = %+v present=%v valid=%v, want user %s present=true valid=true", got, present, valid, user.UUID)
	}
}

// TestSessionUserUnknownCookieIsSilent is the counterpart to
// TestSessionUserLogsUnexpectedError: the expected, high-volume case (an
// invalid/expired session -- repo.ErrNotFound) must NOT be logged, or the
// fix below would just turn every anonymous request into log noise instead
// of surfacing genuine outages.
func TestSessionUserUnknownCookieIsSilent(t *testing.T) {
	ctx := context.Background()
	store := newTestRepo(t)
	logs := captureLogs(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "not-a-real-token"})
	if _, ok := SessionUser(ctx, req, store); ok {
		t.Fatal("expected no user for an invalid session token")
	}
	if logs.Len() > 0 {
		t.Fatalf("expected no log output for an ordinary invalid token, got: %s", logs.String())
	}
}

// TestSessionUserLogsUnexpectedError is the regression test for the finding
// that SessionUser collapsed every repo error -- not just "no such
// session" -- into a plain unauthenticated result with no diagnostic trail,
// which could misdiagnose a DB outage as a wave of client mistakes. A
// closed DB connection triggers a real (non-ErrNotFound) error from
// GetSessionUser; SessionUser must still fail closed (ok=false) but now
// also log it.
func TestSessionUserLogsUnexpectedError(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	store := repo.New(sqlDB)

	user, err := store.CreateUser(ctx, "alice", "password1", model.RoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, _, err := store.CreateSession(ctx, user.UUID, SessionTTL)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	logs := captureLogs(t)
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	if _, ok := SessionUser(ctx, req, store); ok {
		t.Fatal("expected no user once the DB connection is closed")
	}
	if !strings.Contains(logs.String(), "session lookup failed") {
		t.Fatalf("expected an error log for the unexpected DB failure, got: %s", logs.String())
	}
}

// TestBearerUserLogsUnexpectedError mirrors
// TestSessionUserLogsUnexpectedError for BearerUser.
func TestBearerUserLogsUnexpectedError(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	store := repo.New(sqlDB)

	user, err := store.CreateUser(ctx, "alice", "password1", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, _, err := store.CreateAccessToken(ctx, user.UUID, time.Hour)
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}

	logs := captureLogs(t)
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if _, present, valid := BearerUser(ctx, req, store); !present || valid {
		t.Fatalf("present=%v valid=%v, want present=true valid=false once the DB connection is closed", present, valid)
	}
	if !strings.Contains(logs.String(), "bearer token lookup failed") {
		t.Fatalf("expected an error log for the unexpected DB failure, got: %s", logs.String())
	}
}

func TestSameOrigin(t *testing.T) {
	const rootURL = "https://tea.example:8443"

	cases := []struct {
		name          string
		method        string
		origin, refer string
		want          bool
	}{
		{"GET ignores origin entirely", http.MethodGet, "", "", true},
		{"HEAD ignores origin entirely", http.MethodHead, "https://evil.example", "", true},
		{"OPTIONS ignores origin entirely", http.MethodOptions, "https://evil.example", "", true},
		{"POST with matching Origin", http.MethodPost, rootURL, "", true},
		{"POST with mismatched Origin", http.MethodPost, "https://evil.example", "", false},
		{"POST with matching scheme+host but different path in Origin", http.MethodPost, rootURL + "/whatever", "", true},
		{"POST with no Origin or Referer", http.MethodPost, "", "", false},
		{"POST falls back to matching Referer when Origin absent", http.MethodPost, "", rootURL + "/admin/ui/users", true},
		{"POST falls back to mismatched Referer when Origin absent", http.MethodPost, "", "https://evil.example/x", false},
		{"POST with unparseable Origin", http.MethodPost, "not a url\x7f", "", false},
		{"DELETE with mismatched Origin", http.MethodDelete, "https://evil.example", "", false},
		{"DELETE with matching Origin", http.MethodDelete, rootURL, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(c.method, "/", nil)
			if c.origin != "" {
				req.Header.Set("Origin", c.origin)
			}
			if c.refer != "" {
				req.Header.Set("Referer", c.refer)
			}
			if got := SameOrigin(req, rootURL); got != c.want {
				t.Errorf("SameOrigin(method=%s, Origin=%q, Referer=%q) = %v, want %v", c.method, c.origin, c.refer, got, c.want)
			}
		})
	}
}

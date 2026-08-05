package authn

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/oej/opentea/internal/db"
	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/repo"
)

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

	user, err := store.CreateUser(ctx, "alice", "pw", model.RoleAdmin)
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

	user, err := store.CreateUser(ctx, "alice", "pw", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := store.SetAPIToken(ctx, user.UUID)
	if err != nil {
		t.Fatalf("SetAPIToken: %v", err)
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

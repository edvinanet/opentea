package webadmin

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oej/opentea/internal/authn"
	"github.com/oej/opentea/internal/config"
	"github.com/oej/opentea/internal/db"
	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/repo"
)

func newTestServer(t *testing.T) (*httptest.Server, *repo.Repo) {
	t.Helper()
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	r := repo.New(sqlDB)
	srv := httptest.NewServer(NewRouter(r, config.Config{RootURL: "http://example.test"}))
	t.Cleanup(srv.Close)
	return srv, r
}

// TestPagesRenderWithoutError exercises every template (login, dashboard,
// users, token, plus the "new token" and "form error" branches) end to end,
// to catch template parse/execute errors that only surface once a handler
// actually runs (html/template.Must only catches parse errors, not
// execute-time field-name typos).
func TestPagesRenderWithoutError(t *testing.T) {
	ctx := context.Background()
	srv, r := newTestServer(t)

	admin, err := r.CreateUser(ctx, "admin", "adminpw", model.RoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, _, err := r.CreateSession(ctx, admin.UUID, authn.SessionTTL)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	client := srv.Client()
	get := func(path string, cookie string) (int, string) {
		req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: authn.SessionCookieName, Value: cookie})
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}
	post := func(path, cookie string, form map[string]string) (int, string) {
		vals := url.Values{}
		for k, v := range form {
			vals.Set(k, v)
		}
		req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(vals.Encode()))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: authn.SessionCookieName, Value: cookie})
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}

	if status, body := get("/admin/ui/login", ""); status != http.StatusOK {
		t.Fatalf("GET /admin/ui/login: status=%d body=%s", status, body)
	}

	if status, body := get("/admin/ui/", token); status != http.StatusOK {
		t.Fatalf("GET /admin/ui/: status=%d body=%s", status, body)
	}

	if status, body := get("/admin/ui/users", token); status != http.StatusOK {
		t.Fatalf("GET /admin/ui/users: status=%d body=%s", status, body)
	}

	// Trigger the "users" page error re-render branch (duplicate username).
	if status, body := post("/admin/ui/users", token, map[string]string{"username": "admin", "password": "x", "role": "consumer"}); status != http.StatusOK {
		t.Fatalf("POST /admin/ui/users (duplicate): status=%d body=%s", status, body)
	}

	if status, body := get("/admin/ui/token", token); status != http.StatusOK {
		t.Fatalf("GET /admin/ui/token: status=%d body=%s", status, body)
	}
	if status, body := post("/admin/ui/token/generate", token, nil); status != http.StatusOK {
		t.Fatalf("POST /admin/ui/token/generate: status=%d body=%s", status, body)
	}
	// Re-fetch the token page: exercises the "TokenInfo set, NewToken empty" branch.
	if status, body := get("/admin/ui/token", token); status != http.StatusOK {
		t.Fatalf("GET /admin/ui/token (after generating): status=%d body=%s", status, body)
	}

	// Login with a bad password renders the error branch of login.html.
	if status, body := post("/admin/ui/login", "", map[string]string{"username": "admin", "password": "wrong"}); status != http.StatusOK {
		t.Fatalf("POST /admin/ui/login (wrong password): status=%d body=%s", status, body)
	}
}

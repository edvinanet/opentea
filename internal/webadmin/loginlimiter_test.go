// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package webadmin

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/oej/opentea/internal/config"
	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/model"
)

// TestLoginSubmitThrottlesAfterLimit is the regression test for the "login
// has no throttling" finding: repeated login attempts from the same source
// IP eventually get rejected without ever reaching VerifyLogin/bcrypt,
// regardless of whether the submitted credentials are actually correct.
// httpx.LoginLimiter itself is unit-tested in internal/httpx -- this
// exercises it wired into the real handler.
func TestLoginSubmitThrottlesAfterLimit(t *testing.T) {
	srv, r := newTestServer(t)
	ctx := context.Background()

	if _, err := r.CreateUser(ctx, "admin", "correct-password", model.RoleAdmin); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	login := func(password string) (int, string) {
		form := url.Values{"username": {"admin"}, "password": {password}}
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/admin/ui/login", strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		// Matches newTestServer's fixed cfg.RootURL -- requireSameOrigin
		// rejects a mismatched/missing Origin before this request ever
		// reaches the rate limiter or VerifyLogin.
		req.Header.Set("Origin", "http://example.test")
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("POST /admin/ui/login: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read response body: %v", err)
		}
		return resp.StatusCode, string(body)
	}

	// httpx.DefaultLoginRateLimit wrong-password attempts: all get the
	// normal "invalid credentials" response, still within budget.
	for i := 0; i < httpx.DefaultLoginRateLimit; i++ {
		status, body := login("wrong")
		if status != http.StatusOK || !strings.Contains(body, "invalid username or password") {
			t.Fatalf("attempt %d: status=%d body=%q, want 200 with the invalid-credentials message", i+1, status, body)
		}
	}

	// One more attempt, this time with the CORRECT password -- must still
	// be throttled, proving the limiter runs before credential checking,
	// not just after repeated failures.
	status, body := login("correct-password")
	if !strings.Contains(body, "too many login attempts") {
		t.Fatalf("attempt over the limit (correct password): status=%d body=%q, want the throttled message", status, body)
	}
	if strings.Contains(body, "invalid username or password") {
		t.Fatal("throttled response mentions invalid credentials -- suggests VerifyLogin ran despite being over the limit")
	}
}

// TestLoginSubmitProxyAwareRateLimiting confirms the fix end-to-end through
// loginSubmit: with TrustProxyHeaders on, two distinct clients sharing one
// RemoteAddr (as they would behind a real reverse proxy) get independent
// rate-limit budgets, keyed by their own X-Forwarded-For entries rather
// than being collapsed into the proxy's single shared one.
func TestLoginSubmitProxyAwareRateLimiting(t *testing.T) {
	srv, r := newTestServerWithConfig(t, config.Config{RootURL: "http://example.test", APIBasePath: "/tea/v1", TrustProxyHeaders: true})
	ctx := context.Background()
	if _, err := r.CreateUser(ctx, "admin", "correct-password", model.RoleAdmin); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	login := func(forwardedFor string) (int, string) {
		form := url.Values{"username": {"admin"}, "password": {"wrong"}}
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/admin/ui/login", strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://example.test")
		req.Header.Set("X-Forwarded-For", forwardedFor)
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("POST /admin/ui/login: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read response body: %v", err)
		}
		return resp.StatusCode, string(body)
	}

	// Client A uses up its whole budget.
	for i := 0; i < httpx.DefaultLoginRateLimit; i++ {
		if status, body := login("198.51.100.1"); status != http.StatusOK || !strings.Contains(body, "invalid username or password") {
			t.Fatalf("client A attempt %d: status=%d body=%q, want 200 with the invalid-credentials message", i+1, status, body)
		}
	}
	if _, body := login("198.51.100.1"); !strings.Contains(body, "too many login attempts") {
		t.Fatal("client A: expected to be throttled after using its whole budget")
	}

	// Client B, sharing the same RemoteAddr (simulating the same reverse
	// proxy) but a distinct X-Forwarded-For, must not be affected.
	if status, body := login("198.51.100.2"); status != http.StatusOK || !strings.Contains(body, "invalid username or password") {
		t.Fatalf("client B: status=%d body=%q, want 200 with the invalid-credentials message (own, unused budget)", status, body)
	}
}

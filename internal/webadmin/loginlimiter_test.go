package webadmin

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/oej/opentea/internal/config"
	"github.com/oej/opentea/internal/model"
)

func TestLoginLimiterAllow(t *testing.T) {
	l := newLoginLimiterWithLimits(3, time.Minute)

	for i := 0; i < 3; i++ {
		if !l.allow("1.2.3.4") {
			t.Fatalf("attempt %d: allow = false, want true (under the limit)", i+1)
		}
	}
	if l.allow("1.2.3.4") {
		t.Fatal("4th attempt: allow = true, want false (over the limit)")
	}

	// A different IP has its own independent budget.
	if !l.allow("5.6.7.8") {
		t.Fatal("different IP: allow = false, want true")
	}
}

func TestLoginLimiterAllowResetsAfterWindow(t *testing.T) {
	l := newLoginLimiterWithLimits(1, 10*time.Millisecond)

	if !l.allow("1.2.3.4") {
		t.Fatal("1st attempt: allow = false, want true")
	}
	if l.allow("1.2.3.4") {
		t.Fatal("2nd attempt (within window): allow = true, want false")
	}

	time.Sleep(20 * time.Millisecond)
	if !l.allow("1.2.3.4") {
		t.Fatal("attempt after window elapsed: allow = false, want true")
	}
}

// TestLoginSubmitThrottlesAfterLimit is the regression test for the "login
// has no throttling" finding: repeated login attempts from the same source
// IP eventually get rejected without ever reaching VerifyLogin/bcrypt,
// regardless of whether the submitted credentials are actually correct.
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

	// defaultLoginRateLimit wrong-password attempts: all get the normal
	// "invalid credentials" response, still within budget.
	for i := 0; i < defaultLoginRateLimit; i++ {
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

func TestClientIP(t *testing.T) {
	cases := []struct {
		remoteAddr string
		want       string
	}{
		{"192.0.2.1:54321", "192.0.2.1"},
		{"[2001:db8::1]:54321", "2001:db8::1"},
		{"no-port-here", "no-port-here"},
	}
	for _, c := range cases {
		req := httptest.NewRequest("POST", "/", nil)
		req.RemoteAddr = c.remoteAddr
		if got := clientIP(req, false); got != c.want {
			t.Errorf("clientIP(RemoteAddr=%q, trustProxyHeaders=false) = %q, want %q", c.remoteAddr, got, c.want)
		}
	}
}

// TestClientIPTrustProxyHeaders is the regression test for the "rate
// limiter collapses every client behind a reverse proxy into one shared
// budget" finding: with trustProxyHeaders false (direct-deployment
// default), a spoofed X-Forwarded-For must be ignored entirely -- it's as
// untrustworthy as any other client-supplied header. With it true, the
// right-most X-Forwarded-For entry (the one a single trusted proxy hop
// itself appends) is used instead of RemoteAddr.
func TestClientIPTrustProxyHeaders(t *testing.T) {
	req := httptest.NewRequest("POST", "/", nil)
	req.RemoteAddr = "203.0.113.9:54321" // stands in for the proxy's own address
	req.Header.Set("X-Forwarded-For", "198.51.100.1, 198.51.100.2")

	if got := clientIP(req, false); got != "203.0.113.9" {
		t.Errorf("clientIP(trustProxyHeaders=false) = %q, want RemoteAddr (203.0.113.9), X-Forwarded-For ignored", got)
	}
	if got := clientIP(req, true); got != "198.51.100.2" {
		t.Errorf("clientIP(trustProxyHeaders=true) = %q, want the right-most X-Forwarded-For entry (198.51.100.2)", got)
	}

	// No X-Forwarded-For header at all -- falls back to RemoteAddr even
	// with trustProxyHeaders true.
	reqNoHeader := httptest.NewRequest("POST", "/", nil)
	reqNoHeader.RemoteAddr = "203.0.113.9:54321"
	if got := clientIP(reqNoHeader, true); got != "203.0.113.9" {
		t.Errorf("clientIP(trustProxyHeaders=true, no X-Forwarded-For) = %q, want RemoteAddr fallback (203.0.113.9)", got)
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
	for i := 0; i < defaultLoginRateLimit; i++ {
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

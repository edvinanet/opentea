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
		resp, err := srv.Client().PostForm(srv.URL+"/admin/ui/login", url.Values{
			"username": {"admin"}, "password": {password},
		})
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
		if got := clientIP(req); got != c.want {
			t.Errorf("clientIP(RemoteAddr=%q) = %q, want %q", c.remoteAddr, got, c.want)
		}
	}
}

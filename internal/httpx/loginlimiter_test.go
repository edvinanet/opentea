// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package httpx

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoginLimiterAllow(t *testing.T) {
	l := NewLoginLimiterWithLimits(3, time.Minute)

	for i := 0; i < 3; i++ {
		if !l.Allow("1.2.3.4") {
			t.Fatalf("attempt %d: Allow = false, want true (under the limit)", i+1)
		}
	}
	if l.Allow("1.2.3.4") {
		t.Fatal("4th attempt: Allow = true, want false (over the limit)")
	}

	// A different IP has its own independent budget.
	if !l.Allow("5.6.7.8") {
		t.Fatal("different IP: Allow = false, want true")
	}
}

func TestLoginLimiterAllowResetsAfterWindow(t *testing.T) {
	l := NewLoginLimiterWithLimits(1, 10*time.Millisecond)

	if !l.Allow("1.2.3.4") {
		t.Fatal("1st attempt: Allow = false, want true")
	}
	if l.Allow("1.2.3.4") {
		t.Fatal("2nd attempt (within window): Allow = true, want false")
	}

	time.Sleep(20 * time.Millisecond)
	if !l.Allow("1.2.3.4") {
		t.Fatal("attempt after window elapsed: Allow = false, want true")
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
		if got := ClientIP(req, false); got != c.want {
			t.Errorf("ClientIP(RemoteAddr=%q, trustProxyHeaders=false) = %q, want %q", c.remoteAddr, got, c.want)
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

	if got := ClientIP(req, false); got != "203.0.113.9" {
		t.Errorf("ClientIP(trustProxyHeaders=false) = %q, want RemoteAddr (203.0.113.9), X-Forwarded-For ignored", got)
	}
	if got := ClientIP(req, true); got != "198.51.100.2" {
		t.Errorf("ClientIP(trustProxyHeaders=true) = %q, want the right-most X-Forwarded-For entry (198.51.100.2)", got)
	}

	// No X-Forwarded-For header at all -- falls back to RemoteAddr even
	// with trustProxyHeaders true.
	reqNoHeader := httptest.NewRequest("POST", "/", nil)
	reqNoHeader.RemoteAddr = "203.0.113.9:54321"
	if got := ClientIP(reqNoHeader, true); got != "203.0.113.9" {
		t.Errorf("ClientIP(trustProxyHeaders=true, no X-Forwarded-For) = %q, want RemoteAddr fallback (203.0.113.9)", got)
	}
}

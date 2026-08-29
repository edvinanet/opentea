// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package webadmin

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// defaultLoginRateLimit and defaultLoginRateWindow bound login attempts per
// source IP: no more than defaultLoginRateLimit attempts (successful or
// not) within any defaultLoginRateWindow.
const (
	defaultLoginRateLimit  = 5
	defaultLoginRateWindow = time.Minute
	// loginLimiterMaxTrackedIPs is a safety valve against unbounded memory
	// growth from many distinct source IPs (e.g. a distributed guessing
	// attempt) -- if exceeded, the whole map is cleared, which briefly
	// resets everyone's window rather than growing forever. Generous
	// relative to this tool's realistic traffic (an internal admin login
	// page, not a public endpoint).
	loginLimiterMaxTrackedIPs = 10_000
)

// loginLimiter throttles login attempts per source IP, closing two gaps in
// an otherwise-unthrottled bcrypt.CompareHashAndPassword call (see
// repo.VerifyLogin): unbounded password-guessing throughput, and unbounded
// CPU spent on bcrypt itself -- each attempt is deliberately expensive, so
// letting a client trigger unlimited attempts turns that cost into its own
// CPU-exhaustion vector. Keyed by IP rather than by username, so this can't
// be used to lock a legitimate user out of their own account -- it only
// throttles whoever is making the requests.
//
// Behind a reverse proxy, every client shares the proxy's one RemoteAddr
// unless config.Config.TrustProxyHeaders is set, in which case clientIP
// reads the real client IP from X-Forwarded-For instead -- see clientIP's
// doc comment for the trust model. With that flag off (the default),
// r.RemoteAddr is used directly, correct for a standalone deployment but
// collapsing every client behind an untrusted/unconfigured proxy into one
// shared budget.
//
// State is in-memory only and resets on restart -- an accepted tradeoff for
// a simple, self-hosted admin tool, not a durable security control.
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	limit    int
	window   time.Duration
}

func newLoginLimiter() *loginLimiter {
	return newLoginLimiterWithLimits(defaultLoginRateLimit, defaultLoginRateWindow)
}

// newLoginLimiterWithLimits builds a loginLimiter with a caller-chosen
// limit/window instead of the defaults, so tests can exercise throttling
// without a real time.Minute wait.
func newLoginLimiterWithLimits(limit int, window time.Duration) *loginLimiter {
	return &loginLimiter{attempts: map[string][]time.Time{}, limit: limit, window: window}
}

// allow reports whether ip is currently under the attempt limit. Callers
// should check this before verifying credentials at all (not just before
// granting a session), so a burst of attempts is capped regardless of
// whether each individual attempt would have succeeded.
func (l *loginLimiter) allow(ip string) bool {
	now := time.Now()
	cutoff := now.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.attempts) > loginLimiterMaxTrackedIPs {
		l.attempts = map[string][]time.Time{}
	}

	kept := l.attempts[ip][:0]
	for _, t := range l.attempts[ip] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.limit {
		l.attempts[ip] = kept
		return false
	}
	l.attempts[ip] = append(kept, now)
	return true
}

// clientIP extracts the request's client IP for rate-limiting purposes:
// RemoteAddr's host by default, or (when trustProxyHeaders is set) the
// right-most entry of X-Forwarded-For if present -- the standard safe
// pattern for exactly one trusted reverse-proxy hop that appends (not
// replaces) the real client IP as it forwards, matching the same
// single-hop-trust model as httpx.IsSecure's X-Forwarded-Proto handling.
// See config.Config.TrustProxyHeaders's doc comment for why this is opt-in:
// the header is exactly as spoofable as any other client-supplied header
// otherwise, so with trustProxyHeaders false (the default), this falls
// back to RemoteAddr, which net/http guarantees is set (in "IP:port" form)
// for every request it hands to a handler.
func clientIP(r *http.Request, trustProxyHeaders bool) string {
	if trustProxyHeaders {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if candidate := strings.TrimSpace(parts[len(parts)-1]); candidate != "" {
				return candidate
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package httpx

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DefaultLoginRateLimit and DefaultLoginRateWindow bound login attempts per
// source IP: no more than DefaultLoginRateLimit attempts (successful or
// not) within any DefaultLoginRateWindow.
const (
	DefaultLoginRateLimit  = 5
	DefaultLoginRateWindow = time.Minute
	// LoginLimiterMaxTrackedIPs is a safety valve against unbounded memory
	// growth from many distinct source IPs (e.g. a distributed guessing
	// attempt) -- if exceeded, the whole map is cleared, which briefly
	// resets everyone's window rather than growing forever. Generous
	// relative to a self-hosted admin/staff login page's realistic
	// traffic, not a public endpoint.
	LoginLimiterMaxTrackedIPs = 10_000
)

// LoginLimiter throttles login attempts per source IP, closing two gaps in
// an otherwise-unthrottled bcrypt.CompareHashAndPassword call: unbounded
// password-guessing throughput, and unbounded CPU spent on bcrypt itself --
// each attempt is deliberately expensive, so letting a client trigger
// unlimited attempts turns that cost into its own CPU-exhaustion vector.
// Keyed by IP rather than by username, so this can't be used to lock a
// legitimate user out of their own account -- it only throttles whoever is
// making the requests.
//
// Behind a reverse proxy, every client shares the proxy's one RemoteAddr
// unless the caller opts into trusting X-Forwarded-For (see ClientIP's doc
// comment for the trust model).
//
// State is in-memory only and resets on restart -- an accepted tradeoff for
// a simple, self-hosted login page, not a durable security control.
//
// Shared by internal/webadmin (opentea's own admin GUI) and
// internal/openteapublisher (its own staff login) -- both need the exact
// same throttling, with zero coupling to either app's own config/repo
// types, hence living here rather than duplicated.
type LoginLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	limit    int
	window   time.Duration
}

// NewLoginLimiter builds a LoginLimiter using DefaultLoginRateLimit/DefaultLoginRateWindow.
func NewLoginLimiter() *LoginLimiter {
	return NewLoginLimiterWithLimits(DefaultLoginRateLimit, DefaultLoginRateWindow)
}

// NewLoginLimiterWithLimits builds a LoginLimiter with a caller-chosen
// limit/window instead of the defaults, so tests can exercise throttling
// without a real time.Minute wait.
func NewLoginLimiterWithLimits(limit int, window time.Duration) *LoginLimiter {
	return &LoginLimiter{attempts: map[string][]time.Time{}, limit: limit, window: window}
}

// Allow reports whether ip is currently under the attempt limit. Callers
// should check this before verifying credentials at all (not just before
// granting a session), so a burst of attempts is capped regardless of
// whether each individual attempt would have succeeded.
func (l *LoginLimiter) Allow(ip string) bool {
	now := time.Now()
	cutoff := now.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.attempts) > LoginLimiterMaxTrackedIPs {
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

// ClientIP extracts the request's client IP for rate-limiting purposes:
// RemoteAddr's host by default, or (when trustProxyHeaders is set) the
// right-most entry of X-Forwarded-For if present -- the standard safe
// pattern for exactly one trusted reverse-proxy hop that appends (not
// replaces) the real client IP as it forwards, matching the same
// single-hop-trust model IsSecure's X-Forwarded-Proto handling uses.
// trustProxyHeaders should stay off by default: the header is exactly as
// spoofable as any other client-supplied header otherwise, so with it
// false, this falls back to RemoteAddr, which net/http guarantees is set
// (in "IP:port" form) for every request it hands to a handler.
func ClientIP(r *http.Request, trustProxyHeaders bool) string {
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

package webadmin

import (
	"net"
	"net/http"
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
// This only helps the standalone deployment path (no reverse proxy in
// front, so r.RemoteAddr is the real client IP). A rate-limiting proxy
// should handle this upstream instead, where it can see the real client IP
// through whatever proxy layers sit in front of it -- this makes no attempt
// to parse X-Forwarded-For or similar, since those headers are
// attacker-controlled unless a specific trusted-proxy configuration strips
// and re-sets them, which this project doesn't have.
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

// clientIP extracts the host portion of r.RemoteAddr (which net/http
// guarantees is set, in "IP:port" form, for every request it hands to a
// handler) -- see loginLimiter's doc comment for why this deliberately
// doesn't look at any client-supplied header.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

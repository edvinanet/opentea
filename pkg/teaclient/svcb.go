package teaclient

import (
	"context"
	"math/rand"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// defaultWellKnownPort is what a TEA discovery authority is queried on
// when no SVCB/HTTPS record redirects the client elsewhere (TEA discovery
// spec's "Port resolution": "the server that is part of the first step of
// discovery will by default be running on the default HTTPS port 443").
const defaultWellKnownPort = 443

// publicFallbackResolver is used only when the system resolver config
// (/etc/resolv.conf) can't be read -- e.g. non-Linux, or a minimal
// container without one. Cloudflare's public resolver, chosen for
// availability. Best-effort: SVCB lookup is an entirely optional
// enhancement, never required for the bootstrap flow to work at all, so
// falling back to a fixed public resolver here (rather than failing) is
// an acceptable simplification, not a trust decision about that resolver.
const publicFallbackResolver = "1.1.1.1"

// resolveWellKnownTarget determines the host:port to actually connect to
// for fetching authority's .well-known/tea document, per RFC 9460
// HTTPS-record-based failover/load-balancing (TEA discovery spec's "Port
// resolution": "if clients are known to support HTTPS/SVCB DNS records...
// these may be used to redirect to other ports and servers").
//
// Any failure at any point (resolver config unavailable, query timeout,
// NXDOMAIN, no usable records) falls back to (authority, 443, false) --
// SVCB is a pure optional enhancement; a lookup failure must never block
// the .well-known fetch itself.
//
// Deliberately out of scope: ECH config (the "ech" SvcParam), ipv4hint/
// ipv6hint (ordinary A/AAAA resolution of the chosen target still happens
// normally via the HTTP transport's own dialer), ALPN enforcement beyond
// what Go's TLS stack already negotiates, and AliasForm chains longer than
// one hop (avoids a pathological redirect loop from a misconfigured or
// hostile zone). No native Windows/macOS system-resolver-config
// integration -- /etc/resolv.conf or the fixed public fallback only.
func resolveWellKnownTarget(ctx context.Context, authority string) (host string, port int, usedSVCB bool) {
	cfg, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil || len(cfg.Servers) == 0 {
		cfg = &dns.ClientConfig{Servers: []string{publicFallbackResolver}, Port: "53", Timeout: 5}
	}
	return resolveWellKnownTargetWithConfig(ctx, cfg, authority)
}

// resolveWellKnownTargetWithConfig is resolveWellKnownTarget with an
// injectable resolver config, so tests can point it at a local test DNS
// server instead of the real system resolver.
func resolveWellKnownTargetWithConfig(ctx context.Context, cfg *dns.ClientConfig, authority string) (host string, port int, usedSVCB bool) {
	server := net.JoinHostPort(cfg.Servers[0], cfg.Port)

	// SVCB/HTTPS records are queried against the bare hostname -- a TEI
	// authority may include an explicit port for direct connection, but
	// DNS queries never do. fallbackHost/fallbackPort split that same
	// explicit port back out for the no-SVCB-record case below, so the
	// return contract (host is always bare, port is always numeric) holds
	// on every path, not just the SVCB-success one -- the caller
	// (fetchWellKnown) always does net.JoinHostPort(host, port) itself and
	// must never receive a host that already contains a port.
	fallbackHost, fallbackPort := authority, defaultWellKnownPort
	if h, p, err := net.SplitHostPort(authority); err == nil {
		fallbackHost = h
		if n, err := strconv.Atoi(p); err == nil {
			fallbackPort = n
		}
	}

	rr, ok := queryHTTPS(ctx, cfg, server, dns.Fqdn(fallbackHost))
	if !ok {
		return fallbackHost, fallbackPort, false
	}

	if rr.Priority == 0 {
		// AliasForm: re-query once at Target, don't chase further.
		if rr.Target == "." || rr.Target == "" {
			return fallbackHost, fallbackPort, false
		}
		next, ok := queryHTTPS(ctx, cfg, server, dns.Fqdn(strings.TrimSuffix(rr.Target, ".")))
		if !ok || next.Priority == 0 {
			return fallbackHost, fallbackPort, false
		}
		rr = next
	}

	// Target "." means "same as this record's own owner name" (RFC 9460),
	// which is rr.Hdr.Name -- NOT necessarily the name this function
	// itself queried. A real DNS resolver answering a query for a CNAME'd
	// authority returns the actual record under the CNAME's *target*
	// (canonical) name, with Hdr.Name set accordingly; using the
	// originally-queried alias name here instead would silently resolve
	// "." to the wrong host whenever the authority itself is a CNAME (see
	// TestResolveWellKnownTargetCNAMEOnAuthority).
	targetHost := strings.TrimSuffix(rr.Hdr.Name, ".")
	if rr.Target != "." && rr.Target != "" {
		targetHost = strings.TrimSuffix(rr.Target, ".")
	}
	targetPort := defaultWellKnownPort
	for _, v := range rr.Value {
		if p, ok := v.(*dns.SVCBPort); ok {
			targetPort = int(p.Port)
		}
	}
	return targetHost, targetPort, true
}

// queryHTTPS queries name's HTTPS (RFC 9460) records against server and
// returns the highest-priority record found (lowest SvcPriority value
// wins; ties broken by random shuffle before a stable sort, per RFC 9460
// §2.4.1's SHOULD-level load-balancing guidance). ok is false for any
// failure, non-success Rcode, or an answer with no HTTPS records.
func queryHTTPS(ctx context.Context, cfg *dns.ClientConfig, server, name string) (*dns.HTTPS, bool) {
	m := new(dns.Msg)
	m.SetQuestion(name, dns.TypeHTTPS)

	c := new(dns.Client)
	if cfg.Timeout > 0 {
		c.Timeout = time.Duration(cfg.Timeout) * time.Second
	}

	r, _, err := c.ExchangeContext(ctx, m, server)
	if err != nil || r == nil || r.Rcode != dns.RcodeSuccess {
		return nil, false
	}

	var candidates []*dns.HTTPS
	for _, ans := range r.Answer {
		if https, ok := ans.(*dns.HTTPS); ok {
			candidates = append(candidates, https)
		}
	}
	if len(candidates) == 0 {
		return nil, false
	}
	rand.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Priority < candidates[j].Priority })
	return candidates[0], true
}

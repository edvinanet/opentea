package teaclient

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// startTestDNSServer starts a real local UDP DNS server (miekg/dns's own
// Server type) serving handler, and returns a *dns.ClientConfig pointing
// at it plus a cleanup func. Exercises the real query/exchange/parse path
// end to end rather than mocking dns.Client -- more realistic, and this
// package has no injectable-interface seam to mock through anyway.
func startTestDNSServer(t *testing.T, handler dns.HandlerFunc) *dns.ClientConfig {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket: %v", err)
	}
	srv := &dns.Server{PacketConn: pc, Handler: handler}
	started := make(chan struct{})
	srv.NotifyStartedFunc = func() { close(started) }
	go func() { _ = srv.ActivateAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown() })

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("test DNS server did not start in time")
	}

	host, port, err := net.SplitHostPort(pc.LocalAddr().String())
	if err != nil {
		t.Fatalf("split local addr: %v", err)
	}
	return &dns.ClientConfig{Servers: []string{host}, Port: port, Timeout: 2}
}

func httpsAnswer(name string, priority uint16, target string, port uint16) *dns.HTTPS {
	rr := &dns.HTTPS{
		SVCB: dns.SVCB{
			Hdr:      dns.RR_Header{Name: dns.Fqdn(name), Rrtype: dns.TypeHTTPS, Class: dns.ClassINET, Ttl: 300},
			Priority: priority,
			Target:   dns.Fqdn(target),
		},
	}
	if priority > 0 && port > 0 {
		rr.Value = []dns.SVCBKeyValue{&dns.SVCBPort{Port: port}}
	}
	return rr
}

func TestResolveWellKnownTargetServiceForm(t *testing.T) {
	cfg := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Answer = []dns.RR{httpsAnswer("products.example.com.", 1, "api.products.example.com.", 8443)}
		_ = w.WriteMsg(m)
	})

	host, port, usedSVCB := resolveWellKnownTargetWithConfig(context.Background(), cfg, "products.example.com")
	if !usedSVCB {
		t.Fatal("usedSVCB = false, want true")
	}
	if host != "api.products.example.com" {
		t.Fatalf("host = %q, want api.products.example.com", host)
	}
	if port != 8443 {
		t.Fatalf("port = %d, want 8443", port)
	}
}

func TestResolveWellKnownTargetAliasFormOneHop(t *testing.T) {
	cfg := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		name := r.Question[0].Name
		switch name {
		case dns.Fqdn("products.example.com"):
			m.Answer = []dns.RR{httpsAnswer(name, 0, "real.example.com.", 0)}
		case dns.Fqdn("real.example.com"):
			m.Answer = []dns.RR{httpsAnswer(name, 1, ".", 9443)}
		}
		_ = w.WriteMsg(m)
	})

	host, port, usedSVCB := resolveWellKnownTargetWithConfig(context.Background(), cfg, "products.example.com")
	if !usedSVCB {
		t.Fatal("usedSVCB = false, want true")
	}
	if host != "real.example.com" {
		t.Fatalf("host = %q, want real.example.com", host)
	}
	if port != 9443 {
		t.Fatalf("port = %d, want 9443", port)
	}
}

func TestResolveWellKnownTargetAliasChainLongerThanOneHopFallsBack(t *testing.T) {
	// Every AliasForm response, never terminating in a ServiceForm record
	// -- must fall back rather than loop or chase indefinitely.
	cfg := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Answer = []dns.RR{httpsAnswer(r.Question[0].Name, 0, "still-an-alias.example.com.", 0)}
		_ = w.WriteMsg(m)
	})

	host, port, usedSVCB := resolveWellKnownTargetWithConfig(context.Background(), cfg, "products.example.com")
	if usedSVCB {
		t.Fatal("usedSVCB = true, want false (alias chain longer than one hop)")
	}
	if host != "products.example.com" || port != defaultWellKnownPort {
		t.Fatalf("got (%q, %d), want (products.example.com, %d)", host, port, defaultWellKnownPort)
	}
}

func TestResolveWellKnownTargetNoRecordsFallsBack(t *testing.T) {
	cfg := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r) // empty answer, NOERROR
		_ = w.WriteMsg(m)
	})

	host, port, usedSVCB := resolveWellKnownTargetWithConfig(context.Background(), cfg, "products.example.com")
	if usedSVCB {
		t.Fatal("usedSVCB = true, want false")
	}
	if host != "products.example.com" || port != defaultWellKnownPort {
		t.Fatalf("got (%q, %d), want (products.example.com, %d)", host, port, defaultWellKnownPort)
	}
}

func TestResolveWellKnownTargetNXDOMAINFallsBack(t *testing.T) {
	cfg := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetRcode(r, dns.RcodeNameError)
		_ = w.WriteMsg(m)
	})

	host, port, usedSVCB := resolveWellKnownTargetWithConfig(context.Background(), cfg, "products.example.com")
	if usedSVCB {
		t.Fatal("usedSVCB = true, want false")
	}
	if host != "products.example.com" || port != defaultWellKnownPort {
		t.Fatalf("got (%q, %d), want (products.example.com, %d)", host, port, defaultWellKnownPort)
	}
}

// TestResolveWellKnownTargetFallbackPreservesExplicitPort is the
// regression test for a real bug caught while writing wellknown_test.go:
// on the no-SVCB-record fallback path, host must be the bare hostname
// with any port from authority split back out into port, not the whole
// original authority string (which may already contain ":port") returned
// verbatim as host -- fetchWellKnown always does
// net.JoinHostPort(host, port) itself, which would double up into a
// malformed address like "[127.0.0.1:8443]:443" otherwise.
func TestResolveWellKnownTargetFallbackPreservesExplicitPort(t *testing.T) {
	cfg := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r) // empty answer -- forces the fallback path
		_ = w.WriteMsg(m)
	})

	host, port, usedSVCB := resolveWellKnownTargetWithConfig(context.Background(), cfg, "127.0.0.1:8443")
	if usedSVCB {
		t.Fatal("usedSVCB = true, want false")
	}
	if host != "127.0.0.1" {
		t.Fatalf("host = %q, want the bare host 127.0.0.1 (not the whole authority)", host)
	}
	if port != 8443 {
		t.Fatalf("port = %d, want 8443 (the explicit port from authority, not the 443 default)", port)
	}
}

func TestResolveWellKnownTargetResolverUnreachableFallsBack(t *testing.T) {
	// Port 0 on loopback with nothing listening -- Exchange must fail
	// (connection refused / timeout), and that failure must fall back
	// rather than propagate as an error from resolveWellKnownTargetWithConfig.
	cfg := &dns.ClientConfig{Servers: []string{"127.0.0.1"}, Port: "1", Timeout: 1}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	host, port, usedSVCB := resolveWellKnownTargetWithConfig(ctx, cfg, "products.example.com")
	if usedSVCB {
		t.Fatal("usedSVCB = true, want false")
	}
	if host != "products.example.com" || port != defaultWellKnownPort {
		t.Fatalf("got (%q, %d), want (products.example.com, %d)", host, port, defaultWellKnownPort)
	}
}

func TestResolveWellKnownTargetStripsPortFromAuthorityForQuery(t *testing.T) {
	var queriedName string
	cfg := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		queriedName = r.Question[0].Name
		m := new(dns.Msg)
		m.SetReply(r)
		_ = w.WriteMsg(m)
	})

	_, _, _ = resolveWellKnownTargetWithConfig(context.Background(), cfg, "products.example.com:8443")
	if queriedName != dns.Fqdn("products.example.com") {
		t.Fatalf("queried name = %q, want %q (port must not be part of the DNS query name)", queriedName, dns.Fqdn("products.example.com"))
	}
}

// TestResolveWellKnownTargetCNAMEOnAuthority is a probe, not (yet) an
// assertion of correct behavior: a real DNS resolver answering an HTTPS
// query for a CNAME'd name returns the CNAME record AND the HTTPS record
// from the CNAME's target in one answer set, with the HTTPS record's own
// Hdr.Name set to the *canonical* name, not the originally-queried alias.
// A record's Target of "." means "same name as this record's own owner"
// (RFC 9460), which after a CNAME is the canonical name -- NOT the
// original queried name. This checks whether resolveWellKnownTargetWithConfig
// gets that right.
func TestResolveWellKnownTargetCNAMEOnAuthority(t *testing.T) {
	cfg := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Answer = []dns.RR{
			&dns.CNAME{
				Hdr:    dns.RR_Header{Name: dns.Fqdn("tea15.example.com"), Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 300},
				Target: dns.Fqdn("tea15-canonical.example.com"),
			},
			httpsAnswer("tea15-canonical.example.com.", 1, ".", 7443),
		}
		_ = w.WriteMsg(m)
	})

	host, port, usedSVCB := resolveWellKnownTargetWithConfig(context.Background(), cfg, "tea15.example.com")
	if !usedSVCB {
		t.Fatal("usedSVCB = false, want true")
	}
	if host != "tea15-canonical.example.com" {
		t.Fatalf("host = %q, want tea15-canonical.example.com (the record's own owner name after the CNAME, not the original alias)", host)
	}
	if port != 7443 {
		t.Fatalf("port = %d, want 7443", port)
	}
}

// TestQueryHTTPSPicksLowestPriority confirms selection picks the lowest
// SvcPriority value (RFC 9460: lower number = higher priority) among
// several ServiceForm candidates, regardless of answer order.
func TestQueryHTTPSPicksLowestPriority(t *testing.T) {
	cfg := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		name := r.Question[0].Name
		m.Answer = []dns.RR{
			httpsAnswer(name, 5, "low-priority.example.com.", 443),
			httpsAnswer(name, 1, "high-priority.example.com.", 443),
			httpsAnswer(name, 3, "mid-priority.example.com.", 443),
		}
		_ = w.WriteMsg(m)
	})

	rr, ok := queryHTTPS(context.Background(), cfg, net.JoinHostPort(cfg.Servers[0], cfg.Port), dns.Fqdn("products.example.com"))
	if !ok {
		t.Fatal("queryHTTPS returned ok=false")
	}
	if rr.Priority != 1 || rr.Target != dns.Fqdn("high-priority.example.com") {
		t.Fatalf("got priority=%d target=%q, want priority=1 target=%s", rr.Priority, rr.Target, dns.Fqdn("high-priority.example.com"))
	}
}

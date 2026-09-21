// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teaclient

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oej/opentea/pkg/tea"
)

// newFakeWellKnownAuthority starts an httptest.NewTLSServer whose host:port
// is used directly as a test "authority" -- since resolveWellKnownTarget's
// real (non-test-injectable) entry point does a genuine SVCB lookup
// against the actual system resolver, and a bogus test hostname like
// "products.example.com" won't have any HTTPS records in real DNS, it
// falls through to the plain fallback path (bare host, its own explicit
// port) exactly as intended -- no DNS mocking needed for these tests,
// which are about the fetch/retry/bootstrap logic layered on top, not SVCB
// itself (already covered directly in svcb_test.go).
//
// The returned authority carries a port, which a real TEI's authority never
// can (ExtractTEIAuthority rejects one -- see TestBootstrapDiscoverRejectsPortInTEI).
// Callers here pass it to bootstrapDiscoverWithAuthority, not the public
// BootstrapDiscover, precisely to bypass that check and reach a directly-
// dialable local server without a fake DNS server in every test.
//
// srv.Client() is NOT used here on purpose: fetchWellKnown builds its own
// http.Client with an overridden DialContext, so what actually matters for
// "does this pass TLS validation" is whether the *Option-supplied*
// WithHTTPClient's Transport trusts srv's certificate -- tested explicitly
// by TestBootstrapDiscoverTLSFailureNoRetry using an untrusting client.
func newFakeWellKnownAuthority(t *testing.T, handler http.HandlerFunc) (authority string, srv *httptest.Server) {
	t.Helper()
	srv = httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	u, err := neturl(srv.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	return u, srv
}

// neturl extracts host:port from a "https://host:port" URL string.
func neturl(rawURL string) (string, error) {
	rawURL = strings.TrimPrefix(rawURL, "https://")
	return rawURL, nil
}

// trustingOption returns an Option configuring a Client's http.Client to
// trust srv's TLS certificate (httptest.NewTLSServer's own self-signed
// cert), via srv.Client()'s pre-configured Transport.
func trustingOption(srv *httptest.Server) Option {
	return WithHTTPClient(srv.Client())
}

func wellKnownHandler(t *testing.T, endpoints []tea.WellKnownEndpoint) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/tea" {
			t.Errorf(".well-known fetch path = %q, want /.well-known/tea", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(tea.WellKnownDocument{SchemaVersion: 1, Endpoints: endpoints})
	}
}

func priorityPtr(p float64) *float64 { return &p }

func TestBootstrapDiscoverSuccessWithVersionNegotiation(t *testing.T) {
	var discoverPath string
	apiSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		discoverPath = r.URL.Path
		if r.URL.Query().Get("tei") == "" {
			t.Error("discover request missing tei query param")
		}
		_ = json.NewEncoder(w).Encode([]tea.DiscoveryInfo{{ProductReleaseUUID: "pr-1"}})
	}))
	t.Cleanup(apiSrv.Close)

	authority, wkSrv := newFakeWellKnownAuthority(t, wellKnownHandler(t, []tea.WellKnownEndpoint{
		{URL: apiSrv.URL, Versions: []string{"0.2.0-beta.2", "0.4.0", "1.0.0"}},
	}))

	oldSupported := SupportedVersions
	SupportedVersions = []string{"0.4.0"}
	t.Cleanup(func() { SupportedVersions = oldSupported })

	tei := "tei://" + authority + "/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1"
	result, err := bootstrapDiscoverWithAuthority(context.Background(), authority, tei, trustingOption(wkSrv), trustingOption(apiSrv))
	if err != nil {
		t.Fatalf("BootstrapDiscover: %v", err)
	}
	if !strings.HasSuffix(result.ServerURL, "/v0.4.0") {
		t.Fatalf("ServerURL = %q, want a /v0.4.0 suffix (highest mutual version)", result.ServerURL)
	}
	if len(result.Info) != 1 || result.Info[0].ProductReleaseUUID != "pr-1" {
		t.Fatalf("Info = %+v", result.Info)
	}
	if discoverPath != "/v0.4.0/discovery" {
		t.Fatalf("discover request path = %q, want /v0.4.0/discovery", discoverPath)
	}
}

func TestBootstrapDiscoverPriorityOrdering(t *testing.T) {
	var triedHighPriority bool
	highSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		triedHighPriority = true
		_ = json.NewEncoder(w).Encode([]tea.DiscoveryInfo{{ProductReleaseUUID: "from-high-priority"}})
	}))
	t.Cleanup(highSrv.Close)
	lowSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("low-priority endpoint was tried even though the high-priority one succeeded")
		_ = json.NewEncoder(w).Encode([]tea.DiscoveryInfo{{ProductReleaseUUID: "from-low-priority"}})
	}))
	t.Cleanup(lowSrv.Close)

	authority, wkSrv := newFakeWellKnownAuthority(t, wellKnownHandler(t, []tea.WellKnownEndpoint{
		{URL: lowSrv.URL, Versions: []string{"0.4.0"}, Priority: priorityPtr(0.2)},
		{URL: highSrv.URL, Versions: []string{"0.4.0"}, Priority: priorityPtr(0.9)},
	}))

	oldSupported := SupportedVersions
	SupportedVersions = []string{"0.4.0"}
	t.Cleanup(func() { SupportedVersions = oldSupported })

	tei := "tei://" + authority + "/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1"
	result, err := bootstrapDiscoverWithAuthority(context.Background(), authority, tei, trustingOption(wkSrv), trustingOption(highSrv), trustingOption(lowSrv))
	if err != nil {
		t.Fatalf("BootstrapDiscover: %v", err)
	}
	if !triedHighPriority {
		t.Fatal("the higher-priority endpoint was never tried")
	}
	if len(result.Info) != 1 || result.Info[0].ProductReleaseUUID != "from-high-priority" {
		t.Fatalf("Info = %+v, want the high-priority endpoint's result", result.Info)
	}
}

func TestBootstrapDiscoverFailoverOnServerError(t *testing.T) {
	failingSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(failingSrv.Close)
	workingSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]tea.DiscoveryInfo{{ProductReleaseUUID: "from-working-endpoint"}})
	}))
	t.Cleanup(workingSrv.Close)

	authority, wkSrv := newFakeWellKnownAuthority(t, wellKnownHandler(t, []tea.WellKnownEndpoint{
		{URL: failingSrv.URL, Versions: []string{"0.4.0"}, Priority: priorityPtr(1.0)},
		{URL: workingSrv.URL, Versions: []string{"0.4.0"}, Priority: priorityPtr(0.5)},
	}))

	oldSupported := SupportedVersions
	SupportedVersions = []string{"0.4.0"}
	t.Cleanup(func() { SupportedVersions = oldSupported })

	tei := "tei://" + authority + "/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1"
	result, err := bootstrapDiscoverWithAuthority(context.Background(), authority, tei, trustingOption(wkSrv), trustingOption(failingSrv), trustingOption(workingSrv))
	if err != nil {
		t.Fatalf("BootstrapDiscover: %v", err)
	}
	if len(result.Info) != 1 || result.Info[0].ProductReleaseUUID != "from-working-endpoint" {
		t.Fatalf("Info = %+v, want failover to the working endpoint", result.Info)
	}
}

// TestBootstrapDiscoverFailoverOn503 is TestBootstrapDiscoverFailoverOnServerError's
// sibling using exactly 503 -- the TEA discovery spec's own "Common
// errors" section names 503 Service Unavailable specifically as the
// canonical transient-failure example ("temporary failure"), distinct from
// a generic 500. isRetryableError treats every 5xx alike, so this and the
// 500-based test are expected to behave identically -- kept as two tests
// because the spec's own wording calls out 503 by number, worth a direct,
// literal check rather than only ever exercising 500.
func TestBootstrapDiscoverFailoverOn503(t *testing.T) {
	unavailableSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(unavailableSrv.Close)
	workingSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]tea.DiscoveryInfo{{ProductReleaseUUID: "from-working-endpoint"}})
	}))
	t.Cleanup(workingSrv.Close)

	authority, wkSrv := newFakeWellKnownAuthority(t, wellKnownHandler(t, []tea.WellKnownEndpoint{
		{URL: unavailableSrv.URL, Versions: []string{"0.4.0"}, Priority: priorityPtr(1.0)},
		{URL: workingSrv.URL, Versions: []string{"0.4.0"}, Priority: priorityPtr(0.5)},
	}))

	oldSupported := SupportedVersions
	SupportedVersions = []string{"0.4.0"}
	t.Cleanup(func() { SupportedVersions = oldSupported })

	tei := "tei://" + authority + "/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1"
	result, err := bootstrapDiscoverWithAuthority(context.Background(), authority, tei, trustingOption(wkSrv), trustingOption(unavailableSrv), trustingOption(workingSrv))
	if err != nil {
		t.Fatalf("BootstrapDiscover: %v", err)
	}
	if len(result.Info) != 1 || result.Info[0].ProductReleaseUUID != "from-working-endpoint" {
		t.Fatalf("Info = %+v, want failover to the working endpoint", result.Info)
	}
}

// TestBootstrapDiscoverFailoverOnNoMatch is the regression test for a real
// bug this fix uncovered: upstream TEA 1.0 changed GET /discovery's
// no-match response from 200+[] to 404+OBJECT_UNKNOWN
// (spec/openapi.yaml). Before that, Discover returning ([], nil) for "this
// endpoint doesn't have the TEI" was indistinguishable from a genuine
// match with zero results, so bootstrapDiscoverWithAuthority's `err == nil`
// success check stopped at the very first endpoint every time, regardless
// of whether a later, lower-priority endpoint actually had the TEI --
// never previously caught because no existing failover test exercised a
// no-match response, only 5xx/401/403. A 404 is correctly non-retryable
// (isRetryableError) and now correctly triggers failover to the next
// endpoint, exactly like a 5xx does.
func TestBootstrapDiscoverFailoverOnNoMatch(t *testing.T) {
	noMatchSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(tea.ErrorResponse{Error: tea.ErrorObjectUnknown})
	}))
	t.Cleanup(noMatchSrv.Close)
	workingSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]tea.DiscoveryInfo{{ProductReleaseUUID: "from-working-endpoint"}})
	}))
	t.Cleanup(workingSrv.Close)

	authority, wkSrv := newFakeWellKnownAuthority(t, wellKnownHandler(t, []tea.WellKnownEndpoint{
		{URL: noMatchSrv.URL, Versions: []string{"0.4.0"}, Priority: priorityPtr(1.0)},
		{URL: workingSrv.URL, Versions: []string{"0.4.0"}, Priority: priorityPtr(0.5)},
	}))

	oldSupported := SupportedVersions
	SupportedVersions = []string{"0.4.0"}
	t.Cleanup(func() { SupportedVersions = oldSupported })

	tei := "tei://" + authority + "/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1"
	result, err := bootstrapDiscoverWithAuthority(context.Background(), authority, tei, trustingOption(wkSrv), trustingOption(noMatchSrv), trustingOption(workingSrv))
	if err != nil {
		t.Fatalf("BootstrapDiscover: %v", err)
	}
	if len(result.Info) != 1 || result.Info[0].ProductReleaseUUID != "from-working-endpoint" {
		t.Fatalf("Info = %+v, want failover past the no-match endpoint to the working one", result.Info)
	}
}

// TestBootstrapDiscoverNoFailoverOnForbidden is the regression test for the
// TEA discovery spec's explicit rule: "Authentication error codes (401,
// 403) should not lead to failover to the next endpoint in the list."
func TestBootstrapDiscoverNoFailoverOnForbidden(t *testing.T) {
	forbiddenSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(forbiddenSrv.Close)
	neverTriedSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("a lower-priority endpoint was tried after a 403 -- must not fail over on auth errors")
	}))
	t.Cleanup(neverTriedSrv.Close)

	authority, wkSrv := newFakeWellKnownAuthority(t, wellKnownHandler(t, []tea.WellKnownEndpoint{
		{URL: forbiddenSrv.URL, Versions: []string{"0.4.0"}, Priority: priorityPtr(1.0)},
		{URL: neverTriedSrv.URL, Versions: []string{"0.4.0"}, Priority: priorityPtr(0.5)},
	}))

	oldSupported := SupportedVersions
	SupportedVersions = []string{"0.4.0"}
	t.Cleanup(func() { SupportedVersions = oldSupported })

	tei := "tei://" + authority + "/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1"
	_, err := bootstrapDiscoverWithAuthority(context.Background(), authority, tei, trustingOption(wkSrv), trustingOption(forbiddenSrv), trustingOption(neverTriedSrv))
	if err == nil {
		t.Fatal("expected BootstrapDiscover to fail after a 403 rather than fail over")
	}
	if !IsForbidden(err) {
		t.Fatalf("err = %v, want IsForbidden", err)
	}
}

func TestBootstrapDiscoverEndpointWithNoMutualVersionSkipped(t *testing.T) {
	incompatibleSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("an endpoint with no mutually supported version must never actually be queried")
	}))
	t.Cleanup(incompatibleSrv.Close)
	compatibleSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]tea.DiscoveryInfo{{ProductReleaseUUID: "from-compatible-endpoint"}})
	}))
	t.Cleanup(compatibleSrv.Close)

	authority, wkSrv := newFakeWellKnownAuthority(t, wellKnownHandler(t, []tea.WellKnownEndpoint{
		{URL: incompatibleSrv.URL, Versions: []string{"9.9.9"}, Priority: priorityPtr(1.0)},
		{URL: compatibleSrv.URL, Versions: []string{"0.4.0"}, Priority: priorityPtr(0.5)},
	}))

	oldSupported := SupportedVersions
	SupportedVersions = []string{"0.4.0"}
	t.Cleanup(func() { SupportedVersions = oldSupported })

	tei := "tei://" + authority + "/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1"
	result, err := bootstrapDiscoverWithAuthority(context.Background(), authority, tei, trustingOption(wkSrv), trustingOption(incompatibleSrv), trustingOption(compatibleSrv))
	if err != nil {
		t.Fatalf("BootstrapDiscover: %v", err)
	}
	if len(result.Info) != 1 || result.Info[0].ProductReleaseUUID != "from-compatible-endpoint" {
		t.Fatalf("Info = %+v", result.Info)
	}
}

// TestBootstrapDiscoverTLSFailureNoRetry confirms an untrusted certificate
// on the .well-known fetch fails fast (not treated as a generically
// retryable transient error) with a clear error.
func TestBootstrapDiscoverTLSFailureNoRetry(t *testing.T) {
	wkSrv := httptest.NewTLSServer(wellKnownHandler(t, nil))
	t.Cleanup(wkSrv.Close)

	authority, err := neturl(wkSrv.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	tei := "tei://" + authority + "/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1"

	// Deliberately NOT using trustingOption(wkSrv) here -- the default
	// http.Client won't trust httptest's self-signed certificate, so this
	// must fail as a TLS validation error, not hang retrying.
	_, err = bootstrapDiscoverWithAuthority(context.Background(), authority, tei)
	if err == nil {
		t.Fatal("expected an error for an untrusted certificate")
	}
}

// TestBootstrapDiscoverRejectsPortInTEI is the regression test for the
// public BootstrapDiscover entry point specifically: a TEI whose authority
// carries a port must be rejected immediately, before any network I/O --
// the TEA discovery spec is explicit that a port is never part of the TEI
// (see pkg/tea.ExtractTEIAuthority's doc comment). No server is started
// here; a real network attempt on failure would itself be a test bug.
func TestBootstrapDiscoverRejectsPortInTEI(t *testing.T) {
	_, err := BootstrapDiscover(context.Background(), "tei://products.example.com:8443/uuid/d4d9f54a-abcf-11ee-ac79-1a52914d44b1")
	if err == nil {
		t.Fatal("expected an error for a TEI authority with a port")
	}
	var teiErr *tea.TEIError
	if !errors.As(err, &teiErr) {
		t.Fatalf("error = %v (%T), want a *tea.TEIError", err, err)
	}
}

// certificateSANForTests is the only SAN on the self-signed certificate
// newTLSServerWithWrongHostnameCert builds -- deliberately not
// "127.0.0.1"/"::1" (unlike httptest.NewTLSServer's own auto-generated
// cert, which does cover those) and deliberately not the hostname any test
// actually requests either, so a test using this cert is a genuine SAN
// mismatch against whatever hostname it does request -- not an artifact of
// which name happened to be picked.
const certificateSANForTests = "actual-server.invalid"

// newTLSServerWithWrongHostnameCert starts an httptest server presenting a
// self-signed certificate whose only SAN is certificateSANForTests, and
// returns the server plus a *x509.CertPool trusting that cert's issuer --
// i.e. a client using this pool has a fully trusted chain, but will still
// fail hostname verification against any request hostname other than
// certificateSANForTests itself. Isolates "wrong hostname" from "untrusted
// CA" (TestBootstrapDiscoverTLSFailureNoRetry, above): different failure
// modes inside crypto/tls, even though both surface from the same
// certificate-verification step.
func newTLSServerWithWrongHostnameCert(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *x509.CertPool) {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("generate serial: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: certificateSANForTests},
		DNSNames:     []string{certificateSANForTests},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, priv)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	cert := tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  priv,
	}

	pool := x509.NewCertPool()
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	pool.AddCert(leaf)

	srv := httptest.NewUnstartedServer(handler)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	return srv, pool
}

// TestFetchWellKnownTrustedCertWrongHostnameNoRetry confirms a certificate
// that chain-validates fine (its issuer is trusted) but whose SAN doesn't
// match the hostname actually being requested is still rejected -- and,
// like the untrusted-CA case, treated as permanent (no retries), not a
// generic transient failure. This is the case resolveWellKnownTarget's
// SVCB redirection makes newly relevant: the TCP connection can go to a
// different host:port than the request's own Host/SNI (authority), so
// hostname verification genuinely has something to check here, not just
// "does any cert happen to be presented."
//
// This calls doFetchWellKnown directly (like the sibling
// TestFetchWellKnown* tests below) rather than going through
// BootstrapDiscover, and requests a real DNS-shaped hostname
// ("requested-name.invalid") that differs from the certificate's own SAN
// (certificateSANForTests) -- not a literal IP like "127.0.0.1". That
// distinction matters and isn't just style: Go's TLS client omits SNI
// entirely for a literal IP authority (SNI is defined for DNS names, RFC
// 6066), and tls.Config.GetCertificate is only invoked when the client
// either sends SNI or Certificates is empty -- so an IP-literal request
// would silently skip GetCertificate, serve the certificate via the plain
// Certificates fallback instead, and fail with "doesn't contain any IP
// SANs" -- a real, but different, failure from the DNS-hostname-mismatch
// this test is actually meant to exercise. (Caught by writing this test
// against 127.0.0.1 first and finding handshakeAttempts came back 0.)
func TestFetchWellKnownTrustedCertWrongHostnameNoRetry(t *testing.T) {
	srv, pool := newTLSServerWithWrongHostnameCert(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("HTTP handler reached -- the TLS handshake should have been rejected before any request was sent")
	})

	// TLS handshake failures never reach the HTTP handler above (the
	// connection never gets that far), so "was this retried" has to be
	// counted at the handshake layer instead, via GetCertificate.
	var handshakeAttempts int
	presentedCert := srv.TLS.Certificates[0]
	srv.TLS.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		handshakeAttempts++
		return &presentedCert, nil
	}

	realAddr := srv.Listener.Addr().String() // the httptest server's real 127.0.0.1:port
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, realAddr)
			},
		},
	}

	_, err := doFetchWellKnown(context.Background(), client, "https://requested-name.invalid/.well-known/tea")
	if err == nil {
		t.Fatal("expected an error: the certificate's SAN doesn't match the requested hostname")
	}
	var hostErr x509.HostnameError
	if !errors.As(err, &hostErr) {
		t.Fatalf("err = %v, want it to wrap x509.HostnameError (proving this is really a hostname mismatch, not some other failure)", err)
	}
	if handshakeAttempts != 1 {
		t.Fatalf("handshake attempts = %d, want 1 (a hostname mismatch must not be retried, same as an untrusted CA)", handshakeAttempts)
	}
}

// TestFetchWellKnownRejectsRedirect is the regression test for a real bug:
// the .well-known fetch's DialContext (redirected per
// resolveWellKnownTarget's SVCB lookup) always connects to the one
// host:port already resolved, regardless of what host a followed HTTP
// redirect would actually name. Before this was fixed, a redirect to a
// second server was never actually reached -- the client just re-sent the
// request to the *original* server repeatedly (which kept re-issuing the
// same redirect) until hitting net/http's default 10-redirect cap, wasting
// 10 round trips before failing with a generic, unhelpful error. Confirmed
// directly with a standalone probe before this test existed. Redirects are
// now rejected immediately instead of followed at all.
func TestFetchWellKnownRejectsRedirect(t *testing.T) {
	var secondServerHit bool
	secondSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondServerHit = true
	}))
	t.Cleanup(secondSrv.Close)

	var attempts int
	authority, srv := newFakeWellKnownAuthority(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		http.Redirect(w, r, secondSrv.URL+"/.well-known/tea", http.StatusFound)
	})

	_, err := fetchWellKnown(context.Background(), srv.Client(), authority)
	if err == nil {
		t.Fatal("expected an error: redirects must be rejected, not followed")
	}
	if !errors.Is(err, errWellKnownRedirect) {
		t.Fatalf("err = %v, want it to wrap errWellKnownRedirect", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts against the redirecting server = %d, want 1 (a redirect must not be retried, and must not be re-requested repeatedly the way following it used to)", attempts)
	}
	if secondServerHit {
		t.Fatal("the redirect target was contacted -- redirects must not be followed at all")
	}
}

// TestFetchWellKnownRejectsUnparseableJSON confirms a response body that
// isn't valid JSON at all (distinct from valid-JSON-wrong-content, covered
// by TestFetchWellKnownRejectsBadSchemaVersion/RejectsEmptyEndpoints below)
// is rejected cleanly, with no retries -- the server answered successfully
// (200 OK), so retrying the identical request won't produce different
// content.
func TestFetchWellKnownRejectsUnparseableJSON(t *testing.T) {
	var attempts int
	authority, srv := newFakeWellKnownAuthority(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		_, _ = w.Write([]byte("this is not json at all {{{"))
	})

	_, err := fetchWellKnown(context.Background(), srv.Client(), authority)
	if err == nil {
		t.Fatal("expected an error for an unparseable response body")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 (a content-parse error must not be retried)", attempts)
	}
}

func TestFetchWellKnownRejectsBadSchemaVersion(t *testing.T) {
	authority, srv := newFakeWellKnownAuthority(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"schemaVersion": 2, "endpoints": []any{}})
	})

	_, err := fetchWellKnown(context.Background(), srv.Client(), authority)
	if err == nil {
		t.Fatal("expected an error for schemaVersion != 1")
	}
}

func TestFetchWellKnownRejectsEmptyEndpoints(t *testing.T) {
	authority, srv := newFakeWellKnownAuthority(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(tea.WellKnownDocument{SchemaVersion: 1, Endpoints: nil})
	})

	_, err := fetchWellKnown(context.Background(), srv.Client(), authority)
	if err == nil {
		t.Fatal("expected an error for an empty endpoints list")
	}
}

func TestFetchWellKnownRetriesOnTransientFailureThenSucceeds(t *testing.T) {
	var attempts int
	authority, srv := newFakeWellKnownAuthority(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(tea.WellKnownDocument{SchemaVersion: 1, Endpoints: []tea.WellKnownEndpoint{{URL: "https://x", Versions: []string{"1.0.0"}}}})
	})

	doc, err := fetchWellKnown(context.Background(), srv.Client(), authority)
	if err != nil {
		t.Fatalf("fetchWellKnown: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3 (2 failures then success)", attempts)
	}
	if len(doc.Endpoints) != 1 {
		t.Fatalf("Endpoints = %+v", doc.Endpoints)
	}
}

func TestFetchWellKnownDoesNotRetryOn404(t *testing.T) {
	var attempts int
	authority, srv := newFakeWellKnownAuthority(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := fetchWellKnown(context.Background(), srv.Client(), authority)
	if err == nil {
		t.Fatal("expected an error for a 404")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 (404 must not be retried)", attempts)
	}
}

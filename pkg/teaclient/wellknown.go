package teaclient

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/oej/opentea/pkg/tea"
)

// retryBackoffDelays are the waits between retryWithBackoff's attempts
// (three retries after the initial try, so four attempts total). Small,
// fixed, local to this feature -- not a general exported/configurable
// utility, just enough for the two retry call sites in this file.
var retryBackoffDelays = []time.Duration{200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond}

// retryWithBackoff calls fn, retrying with exponential backoff
// (retryBackoffDelays) as long as shouldRetry(err) is true, up to
// len(retryBackoffDelays) retries. Stops early (no more retries) once
// shouldRetry returns false, or if ctx is done while waiting.
func retryWithBackoff[T any](ctx context.Context, fn func() (T, error), shouldRetry func(error) bool) (T, error) {
	var zero T
	var lastErr error
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			if !shouldRetry(lastErr) {
				return zero, lastErr
			}
			delay := retryBackoffDelays[min(attempt-1, len(retryBackoffDelays)-1)]
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return zero, ctx.Err()
			}
		}
		result, err := fn()
		if err == nil {
			return result, nil
		}
		lastErr = err
		if attempt >= len(retryBackoffDelays) {
			return zero, lastErr
		}
	}
}

// errWellKnownRedirect is returned when the .well-known/tea endpoint
// responds with an HTTP redirect. Discovery bootstrap deliberately does
// NOT follow redirects for this fetch, for two reasons: (1) it combines
// badly with resolveWellKnownTarget's SVCB-driven dial redirection --
// fetchWellKnown's DialContext always connects to the one host:port that
// was already resolved, regardless of what a followed redirect would
// actually name, so a redirect to a different origin can never actually
// be reached this way. Confirmed by testing directly: given a redirect to
// a second server, the client never reaches it at all -- it just
// re-requests the *original* server repeatedly (which keeps re-issuing
// the same redirect) until hitting net/http's default 10-redirect limit,
// then fails with a generic "stopped after 10 redirects" error. (2) Even
// setting the dial issue aside, a `.well-known/tea` response redirecting
// to a different origin is exactly the kind of response a
// security-sensitive trust-bootstrap step should treat with suspicion by
// default, not follow transparently -- so this is a deliberate policy,
// not only a workaround for the dial limitation.
var errWellKnownRedirect = errors.New("teaclient: .well-known/tea response was a redirect, which is not followed")

// isRetryableError reports whether err represents a transient failure
// worth retrying: a 5xx APIError, or a genuine transport-level failure
// (dial error, timeout, connection reset, DNS failure at the HTTP layer --
// anything http.Client.Do itself failed on, which net/http always wraps in
// *url.Error, a stable stdlib contract) -- except a TLS certificate
// verification failure or a rejected redirect specifically, both
// permanent (retrying won't fix a bad certificate, and a server's
// redirect behavior won't change either). Everything else is treated as
// permanent too, deliberately including any error that isn't one of the
// above: a non-5xx APIError (4xx, including 404 -- TEA discovery spec's
// "Common errors" section: 404 means no discovery endpoint present, not a
// transient condition), and -- importantly -- a *content*-level error on
// an otherwise-successful response (malformed JSON, wrong schemaVersion,
// an empty endpoints list): the server answered fine, retrying the same
// request won't change what it sent.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	var tlsErr *tls.CertificateVerificationError
	if errors.As(err, &tlsErr) {
		return false
	}
	if errors.Is(err, errWellKnownRedirect) {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode >= 500
	}
	var urlErr *url.Error
	return errors.As(err, &urlErr)
}

// fetchWellKnown fetches and parses authority's .well-known/tea document
// -- always HTTPS (the spec forbids plain HTTP for this endpoint), with
// the TCP connection target optionally redirected by
// resolveWellKnownTarget's SVCB/HTTPS-record lookup while the request
// URL/Host/TLS ServerName stay authority itself (so certificate validation
// is against the authority the caller actually asked about, per RFC 9460's
// target-name handling for the common case). Retries on transient
// failures per isRetryableError.
func fetchWellKnown(ctx context.Context, hc *http.Client, authority string) (tea.WellKnownDocument, error) {
	host, port, _ := resolveWellKnownTarget(ctx, authority)
	dialAddr := net.JoinHostPort(host, strconv.Itoa(port))

	// Clone (not replace) hc's own Transport so its TLS/proxy/etc config
	// (e.g. a caller-supplied WithHTTPClient for custom cert trust, as
	// tests do) is preserved -- only DialContext is overridden, to redirect
	// the TCP connection per resolveWellKnownTarget's result while leaving
	// everything else (including how TLS gets configured/verified) as the
	// caller already set it up.
	transport := baseTransport(hc).Clone()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, dialAddr)
	}
	client := &http.Client{
		Timeout:   hc.Timeout,
		Transport: transport,
		// Redirects are deliberately not followed -- see errWellKnownRedirect's
		// doc comment for why.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return errWellKnownRedirect
		},
	}
	reqURL := "https://" + authority + "/.well-known/tea"

	return retryWithBackoff(ctx, func() (tea.WellKnownDocument, error) {
		return doFetchWellKnown(ctx, client, reqURL)
	}, isRetryableError)
}

// baseTransport returns hc's own *http.Transport to build on, if it has
// one; otherwise falls back to (a copy of) http.DefaultTransport. A
// WithHTTPClient caller using some other http.RoundTripper implementation
// entirely (not *http.Transport) falls back to the default too -- a known,
// narrow limitation of only this one internal dial-redirection use, not of
// Client generally (every other method still calls hc.Do(req) unmodified).
func baseTransport(hc *http.Client) *http.Transport {
	if t, ok := hc.Transport.(*http.Transport); ok && t != nil {
		return t
	}
	if t, ok := http.DefaultTransport.(*http.Transport); ok {
		return t
	}
	return &http.Transport{}
}

func doFetchWellKnown(ctx context.Context, client *http.Client, reqURL string) (tea.WellKnownDocument, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return tea.WellKnownDocument{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return tea.WellKnownDocument{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	if err != nil {
		return tea.WellKnownDocument{}, err
	}
	if len(body) > maxResponseBody {
		return tea.WellKnownDocument{}, fmt.Errorf("teaclient: .well-known/tea response exceeds %d byte limit", maxResponseBody)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return tea.WellKnownDocument{}, &APIError{StatusCode: resp.StatusCode, Body: body}
	}

	var doc tea.WellKnownDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return tea.WellKnownDocument{}, fmt.Errorf("teaclient: parse .well-known/tea response: %w", err)
	}
	if doc.SchemaVersion != 1 {
		return tea.WellKnownDocument{}, fmt.Errorf("teaclient: .well-known/tea schemaVersion = %d, want 1", doc.SchemaVersion)
	}
	if len(doc.Endpoints) == 0 {
		return tea.WellKnownDocument{}, fmt.Errorf("teaclient: .well-known/tea response has no endpoints")
	}
	return doc, nil
}

// BootstrapDiscoverResult is the outcome of BootstrapDiscover: which
// server ultimately answered, and its own discovery result for the TEI.
type BootstrapDiscoverResult struct {
	// ServerURL is the versioned base URL that actually answered, e.g.
	// "https://api.teaexample.com/v1.0.0".
	ServerURL string              `json:"serverUrl"`
	Info      []tea.DiscoveryInfo `json:"info"`
}

// BootstrapDiscover implements TEA's full bootstrap discovery flow (TEA
// discovery spec's "From product identifier to API endpoint" and
// "Connecting to the API" sections): parse tei's authority, fetch its
// .well-known/tea document, then for each listed endpoint (sorted by
// priority, highest first) that shares at least one version with
// SupportedVersions, construct "<endpoint.url>/v<highest-mutual-version>"
// and try its Discover. A 401/403 from an endpoint stops immediately
// without trying the next one (spec: authentication errors must not
// trigger failover); any other failure -- including no mutually supported
// version -- moves on to the next endpoint. Returns an aggregated error if
// every endpoint fails.
func BootstrapDiscover(ctx context.Context, tei string, opts ...Option) (BootstrapDiscoverResult, error) {
	authority, err := tea.ExtractTEIAuthority(tei)
	if err != nil {
		return BootstrapDiscoverResult{}, err
	}

	base := NewClient("", opts...)
	doc, err := fetchWellKnown(ctx, base.httpClient, authority)
	if err != nil {
		return BootstrapDiscoverResult{}, fmt.Errorf("teaclient: fetch .well-known/tea for %s: %w", authority, err)
	}

	var errs []error
	for _, ep := range sortEndpointsByPriority(doc.Endpoints) {
		version, ok := highestMutualVersion(SupportedVersions, ep.Versions)
		if !ok {
			errs = append(errs, fmt.Errorf("%s: no version supported by both this client (%v) and the endpoint (%v)", ep.URL, SupportedVersions, ep.Versions))
			continue
		}
		serverURL := strings.TrimRight(ep.URL, "/") + "/v" + version
		client := NewClient(serverURL, opts...)

		info, err := retryWithBackoff(ctx, func() ([]tea.DiscoveryInfo, error) {
			return client.Discover(ctx, tei)
		}, func(e error) bool {
			return !IsUnauthorized(e) && !IsForbidden(e) && isRetryableError(e)
		})
		if err == nil {
			return BootstrapDiscoverResult{ServerURL: serverURL, Info: info}, nil
		}
		if IsUnauthorized(err) || IsForbidden(err) {
			return BootstrapDiscoverResult{}, fmt.Errorf("teaclient: %s: authentication error, not trying further endpoints: %w", serverURL, err)
		}
		errs = append(errs, fmt.Errorf("%s: %w", serverURL, err))
	}

	return BootstrapDiscoverResult{}, fmt.Errorf("teaclient: bootstrap discovery for %s failed against every candidate endpoint: %w", tei, errors.Join(errs...))
}

// sortEndpointsByPriority returns a copy of endpoints ordered by Priority
// descending (highest tried first; absent Priority defaults to 1, per the
// well-known schema). Ties keep their original list order -- unlike SVCB
// records (RFC 9460 §2.4.1's SHOULD-level randomization among equal
// priority, see queryHTTPS), the well-known schema itself has no such
// guidance for its endpoints[] list.
func sortEndpointsByPriority(endpoints []tea.WellKnownEndpoint) []tea.WellKnownEndpoint {
	out := make([]tea.WellKnownEndpoint, len(endpoints))
	copy(out, endpoints)
	sort.SliceStable(out, func(i, j int) bool {
		return endpointPriority(out[i]) > endpointPriority(out[j])
	})
	return out
}

func endpointPriority(ep tea.WellKnownEndpoint) float64 {
	if ep.Priority == nil {
		return 1
	}
	return *ep.Priority
}

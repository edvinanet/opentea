// Package teaclient is a reference client for the CycloneDX Transparency
// Exchange API consumer read API (spec/openapi.yaml v0.4.0). It's built for
// interoperability testing: point it at any conformant TEA server, not just
// this repo's own reference server.
package teaclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to a single TEA server's consumer read API.
type Client struct {
	baseURL     string
	httpClient  *http.Client
	bearerToken string
}

type Option func(*Client)

// WithHTTPClient overrides the default *http.Client (e.g. for custom TLS config).
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithBearerToken attaches an Authorization: Bearer header to every request.
// The spec's consumer API works fine without one; this is for identifying
// yourself to servers that support (optional) bearer-token auth.
func WithBearerToken(token string) Option {
	return func(c *Client) { c.bearerToken = token }
}

// WithTimeout sets the underlying http.Client's timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.httpClient.Timeout = d }
}

// NewClient builds a Client for the TEA server at baseURL (its "rootUrl",
// e.g. from a discovery response -- the path prefix such as /tea/v1 is part
// of baseURL, this package doesn't assume one).
func NewClient(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// do issues a request against path (relative to baseURL), decodes a 2xx
// JSON response into out (if non-nil), and returns *APIError for any other
// status code.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, out any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return err
	}
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Body: body}
	}
	if out == nil || len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, out)
}

// ListAll repeatedly calls fetch (each call should perform one page request
// using the given pageToken, starting with "") until hasNext is false,
// concatenating every page's results.
func ListAll[T any](fetch func(pageToken string) (results []T, hasNext bool, nextPageToken string, err error)) ([]T, error) {
	var all []T
	token := ""
	for {
		results, hasNext, next, err := fetch(token)
		if err != nil {
			return nil, err
		}
		all = append(all, results...)
		if !hasNext {
			return all, nil
		}
		token = next
	}
}

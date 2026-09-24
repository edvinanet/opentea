// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Package teaclient is a reference client for the CycloneDX Transparency
// Exchange API consumer read API (spec/openapi.yaml v0.4.0). It's built for
// interoperability testing: point it at any conformant TEA server, not just
// this repo's own reference server.
package teaclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxResponseBody bounds how much of any single HTTP response body this
// package reads into memory (a JSON API response in do, or an error body in
// DownloadAndVerifyTo) -- guards a CLI or embedding application against a
// malicious or malfunctioning remote TEA server sending an unbounded
// response and exhausting memory. Matches internal/admin/server.go's
// maxJSONBody on this project's own server side; other TEA servers this
// client talks to aren't bound by that, hence checking here too.
const maxResponseBody = 10 << 20 // 10 MiB

// Client talks to a single TEA server's consumer read API.
type Client struct {
	baseURL     string
	httpClient  *http.Client
	bearerToken string
}

// Option configures a Client at construction time; see WithHTTPClient and WithBearerToken.
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
	// Installed after opts so a WithHTTPClient caller's own CheckRedirect
	// (none exist in this codebase today) can't accidentally disable this --
	// the credential-leak protection below is meant to hold unconditionally,
	// not be opt-out. See crossOriginCheckRedirect's own doc comment for why.
	c.httpClient.CheckRedirect = crossOriginCheckRedirect
	return c
}

// crossOriginCheckRedirect is every Client's http.Client.CheckRedirect: it
// strips the Authorization header before following any redirect whose
// target has a different origin (scheme+host, host including port) than
// the original request. net/http's own default redirect policy already
// does this when the hostname changes, but not when only the port or
// scheme differs on the same host -- confirmed empirically (a same-host,
// different-port redirect still received the bearer token under the
// default policy) during the investigation that led to this fix
// (docs/security-review-260923.md finding #2). TEA's own auth text is
// explicit that a TEA access token is sent only to the TEA server's own
// API base URL, so any origin change on a redirect is exactly the boundary
// that must not carry it further. Replicates net/http's own 10-redirect
// cap, since installing a CheckRedirect at all replaces that default
// entirely, not just the credential-stripping.
func crossOriginCheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("teaclient: stopped after 10 redirects")
	}
	orig := via[0].URL
	if !strings.EqualFold(orig.Scheme, req.URL.Scheme) || !strings.EqualFold(orig.Host, req.URL.Host) {
		req.Header.Del("Authorization")
	}
	return nil
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
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	if err != nil {
		return err
	}
	if len(body) > maxResponseBody {
		return fmt.Errorf("teaclient: response body for %s %s exceeds %d byte limit", method, path, maxResponseBody)
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

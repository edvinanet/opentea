// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Package teapublisherclient is a reference client for the draft standard
// TEA Publisher API (/publisher/v1, design/publisher-openapi.yaml). Built
// for interoperability: point it at any conformant target implementing the
// protocol, not just opentea's own internal/publisher. Its primary intended
// consumer is opentea-publisher's own backend (design/publisher-service.md
// §17, §14.1) -- a service that signs locally (via internal/trust) and
// calls a target's /publisher/v1 as a credentialed client, mirroring the
// read side's existing pkg/teaclient shape exactly.
//
// Unlike pkg/teaclient (whose bearer token is optional -- /tea/v1 works
// unauthenticated), a bearer credential is required here: /publisher/v1
// has no anonymous operations (design/publisher-openapi.yaml's
// bearerAuth security scheme applies to every path).
package teapublisherclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// maxResponseBody bounds how much of any single HTTP response body this
// package reads into memory -- matches pkg/teaclient's own limit and
// internal/publisher/server.go's maxJSONBody; other targets this client
// talks to aren't bound by that, hence checking here too.
const maxResponseBody = 10 << 20 // 10 MiB

// Client talks to a single TEA server's /publisher/v1 as one credentialed
// publisher platform (design/publisher-service.md §10.2/§10.4's Layer B/D).
type Client struct {
	baseURL     string
	bearerToken string
	httpClient  *http.Client
}

// Option configures a Client at construction time; see WithHTTPClient and WithTimeout.
type Option func(*Client)

// WithHTTPClient overrides the default *http.Client (e.g. for custom TLS config).
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithTimeout sets the underlying http.Client's timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.httpClient.Timeout = d }
}

// NewClient builds a Client for the /publisher/v1 API at baseURL (e.g.
// "https://tea.example.com/publisher/v1" -- the path prefix is part of
// baseURL, this package doesn't assume one), authenticating every request
// with bearerToken (a publisher_credential-style token issued by that
// target, scoped "full" or "cicd" -- design/publisher-service.md §10.4;
// which operations a given token may call is enforced by the target, not
// checked client-side).
func NewClient(baseURL, bearerToken string, opts ...Option) *Client {
	c := &Client{
		baseURL:     strings.TrimRight(baseURL, "/"),
		bearerToken: bearerToken,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// do issues a request against path (relative to baseURL), JSON-encoding
// body if non-nil (nil for GET/DELETE and the parameterless POST
// operations -- prepareCommit, cancelPrepare), decodes a 2xx JSON response
// into out (if non-nil), and returns *APIError for any other status code.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.bearerToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	if err != nil {
		return err
	}
	if len(respBody) > maxResponseBody {
		return fmt.Errorf("teapublisherclient: response body for %s %s exceeds %d byte limit", method, path, maxResponseBody)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Body: respBody}
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	return json.Unmarshal(respBody, out)
}

// doUpload issues a multipart/form-data POST carrying a "mediaType" field
// and a "file" field (UploadArtifactFile's shape -- addressed by mediaType,
// not a positional index, matching internal/publisher/artifact.go's
// mediaType-based addressing, design/publisher-openapi.yaml v0.10). r is
// read to completion and not closed by this method -- callers own its
// lifecycle, matching os.File/io.Reader convention.
func (c *Client) doUpload(ctx context.Context, path, mediaType, filename string, r io.Reader) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("mediaType", mediaType); err != nil {
		return err
	}
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, r); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.bearerToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Body: respBody}
	}
	return nil
}

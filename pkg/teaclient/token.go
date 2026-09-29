// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teaclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/oej/opentea/pkg/tea"
)

// ExchangeToken performs TEA 1.0's client_credentials token exchange (spec/
// openapi.yaml's POST /token, upstream's fuller auth/readme.md): presents
// keyID/secret via HTTP Basic to this Client's own baseURL, and returns the
// short-lived access token the server issues in return. That access token
// -- not the API key itself -- is what a conformant TEA server accepts as
// "Authorization: Bearer" on every other endpoint (see WithBearerToken);
// presenting a long-lived API key directly, the way this client used to
// treat any -token value, is exactly what auth/readme.md forbids
// (docs/security-review-260923.md finding #10).
//
// The exchange is bound to this Client's own baseURL: nothing here submits
// keyID/secret, or forwards the resulting access token, to any other
// origin -- the same service-scoping boundary crossOriginCheckRedirect
// already enforces for a token attached via WithBearerToken.
//
// ExchangeToken does not mutate c or attach the result to future requests
// by itself -- combine its result with WithBearerToken when constructing
// the Client callers actually want to use (see cmd/teaclient's
// resolveBearerToken for the typical pattern: exchange first with a bare
// Client, then build the real one).
func (c *Client) ExchangeToken(ctx context.Context, keyID, secret string) (tea.TokenResponse, error) {
	form := url.Values{"grant_type": {"client_credentials"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/token", strings.NewReader(form.Encode()))
	if err != nil {
		return tea.TokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// RFC 6749 section 2.3.1: each Basic-auth component is
	// application/x-www-form-urlencoded encoded before being joined and
	// base64'd. opentea's own POST /token unwinds this the same way
	// (internal/api/token.go's basicAuthUnescaped); a plain keyID/secret
	// with no reserved characters round-trips either way.
	req.SetBasicAuth(url.QueryEscape(keyID), url.QueryEscape(secret))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return tea.TokenResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	if err != nil {
		return tea.TokenResponse{}, err
	}
	if len(body) > maxResponseBody {
		return tea.TokenResponse{}, fmt.Errorf("teaclient: token response exceeds %d byte limit", maxResponseBody)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// A failed token exchange still returns a *APIError, decodable via
		// TEAErrorCode -- token-error-response and TEA's ordinary
		// error-response both key their error code under the same JSON
		// "error" property (pkg/tea.TokenErrorResponse/ErrorResponse), so
		// no separate error type is needed here.
		return tea.TokenResponse{}, &APIError{StatusCode: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), Body: body}
	}

	var out tea.TokenResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return tea.TokenResponse{}, err
	}
	return out, nil
}

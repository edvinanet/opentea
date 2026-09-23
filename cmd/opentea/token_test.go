// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/oej/opentea/internal/model"
)

// tokenRequest POSTs application/x-www-form-urlencoded form values to
// POST /tea/v1/token with the given HTTP Basic credentials (basicUser=""
// sends no Authorization header at all).
func tokenRequest(t *testing.T, srv *testServer, basicUser, basicPass string, form url.Values) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/tea/v1/token", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if basicUser != "" || basicPass != "" {
		req.SetBasicAuth(basicUser, basicPass)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /tea/v1/token: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp.StatusCode, resp.Header, body
}

// TestTokenExchangeFullFlow is the end-to-end proof for TEA 1.0's /token
// (spec/openapi.yaml, auth/readme.md): generate an API key exactly the way
// the GUI does (Repo.SetAPIKey), exchange it at /token, and use the
// resulting access token against a real /tea/v1 resource endpoint --
// confirming the whole chain actually works, not just that each piece
// compiles.
func TestTokenExchangeFullFlow(t *testing.T) {
	srv := newTestServer(t)
	ctx := t.Context()

	user, err := srv.repo.CreateUser(ctx, "alice", "password123", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	keyID, secret, err := srv.repo.SetAPIKey(ctx, user.UUID)
	if err != nil {
		t.Fatalf("SetAPIKey: %v", err)
	}

	status, headers, raw := tokenRequest(t, srv, keyID, secret, url.Values{"grant_type": {"client_credentials"}})
	if status != http.StatusOK {
		t.Fatalf("POST /tea/v1/token: status=%d body=%s", status, raw)
	}
	if cc := headers.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store (RFC 6749 section 5.1)", cc)
	}
	var resp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("decode token response: %v (body: %s)", err, raw)
	}
	if resp.AccessToken == "" {
		t.Fatal("expected a non-empty access_token")
	}
	if resp.TokenType != "Bearer" {
		t.Fatalf("token_type = %q, want Bearer", resp.TokenType)
	}
	if resp.ExpiresIn <= 0 {
		t.Fatalf("expires_in = %d, want > 0", resp.ExpiresIn)
	}

	// The issued access token works against a real /tea/v1 resource.
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/tea/v1/products", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+resp.AccessToken)
	getResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /tea/v1/products with access token: %v", err)
	}
	defer func() { _ = getResp.Body.Close() }()
	if getResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(getResp.Body)
		t.Fatalf("GET /tea/v1/products with access token: status=%d body=%s", getResp.StatusCode, body)
	}
}

// TestTokenExchangeAPIKeyNeverWorksAsBearer is the regression test for the
// conformance gap found this session (auth/readme.md: "A server shall not
// accept an API key directly on the resource endpoints... The API key is
// exchanged for an access token, and the access token is what the
// resource endpoints see"): presenting a raw, valid API key secret
// directly as Authorization: Bearer on /tea/v1 must be rejected, even
// though it's a real credential belonging to a real user -- only a token
// obtained through the /token exchange is accepted there.
func TestTokenExchangeAPIKeyNeverWorksAsBearer(t *testing.T) {
	srv := newTestServer(t)
	ctx := t.Context()

	user, err := srv.repo.CreateUser(ctx, "alice", "password123", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	_, secret, err := srv.repo.SetAPIKey(ctx, user.UUID)
	if err != nil {
		t.Fatalf("SetAPIKey: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/tea/v1/products", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /tea/v1/products with a raw API key secret: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s, want 401 -- a raw API key secret must never work as a /tea/v1 bearer token", resp.StatusCode, body)
	}
}

// TestTokenExchangeWrongSecret covers RFC 6749 section 5.2's 401
// invalid_client case for a client-authentication failure (as opposed to
// a malformed request, which is 400 -- see TestTokenExchangeBadGrantType).
func TestTokenExchangeWrongSecret(t *testing.T) {
	srv := newTestServer(t)
	ctx := t.Context()

	user, err := srv.repo.CreateUser(ctx, "alice", "password123", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	keyID, _, err := srv.repo.SetAPIKey(ctx, user.UUID)
	if err != nil {
		t.Fatalf("SetAPIKey: %v", err)
	}

	status, headers, raw := tokenRequest(t, srv, keyID, "wrong-secret", url.Values{"grant_type": {"client_credentials"}})
	if status != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s, want 401", status, raw)
	}
	if got := headers.Get("WWW-Authenticate"); got != `Basic realm="tea"` {
		t.Fatalf("WWW-Authenticate = %q, want Basic realm=\"tea\" (RFC 6749 section 5.2)", got)
	}
	var errResp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &errResp); err != nil {
		t.Fatalf("decode error response: %v (body: %s)", err, raw)
	}
	if errResp.Error != "invalid_client" {
		t.Fatalf("error = %q, want invalid_client", errResp.Error)
	}
}

// TestTokenExchangeMissingBasicAuth covers the case where no
// Authorization header is presented at all -- also 401 invalid_client per
// the spec's 401-token-error response.
func TestTokenExchangeMissingBasicAuth(t *testing.T) {
	srv := newTestServer(t)

	status, headers, raw := tokenRequest(t, srv, "", "", url.Values{"grant_type": {"client_credentials"}})
	if status != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s, want 401", status, raw)
	}
	if got := headers.Get("WWW-Authenticate"); got != `Basic realm="tea"` {
		t.Fatalf("WWW-Authenticate = %q, want Basic realm=\"tea\"", got)
	}
}

// TestTokenExchangeBadGrantType covers RFC 6749 section 5.2's 400
// unsupported_grant_type case -- client_credentials is the only grant
// type this server supports.
func TestTokenExchangeBadGrantType(t *testing.T) {
	srv := newTestServer(t)
	ctx := t.Context()

	user, err := srv.repo.CreateUser(ctx, "alice", "password123", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	keyID, secret, err := srv.repo.SetAPIKey(ctx, user.UUID)
	if err != nil {
		t.Fatalf("SetAPIKey: %v", err)
	}

	for _, grantType := range []string{"", "authorization_code", "refresh_token"} {
		status, _, raw := tokenRequest(t, srv, keyID, secret, url.Values{"grant_type": {grantType}})
		if status != http.StatusBadRequest {
			t.Fatalf("grant_type=%q: status=%d body=%s, want 400", grantType, status, raw)
		}
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(raw, &errResp); err != nil {
			t.Fatalf("decode error response: %v (body: %s)", err, raw)
		}
		if errResp.Error != "unsupported_grant_type" {
			t.Fatalf("grant_type=%q: error = %q, want unsupported_grant_type", grantType, errResp.Error)
		}
	}
}

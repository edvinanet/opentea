// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teaclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/oej/opentea/pkg/tea"
)

func TestExchangeTokenSuccess(t *testing.T) {
	var gotKeyID, gotSecret, gotGrantType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s, want POST /token", r.Method, r.URL.Path)
		}
		var ok bool
		gotKeyID, gotSecret, ok = r.BasicAuth()
		if !ok {
			t.Fatal("request missing Authorization: Basic")
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		gotGrantType = r.PostForm.Get("grant_type")
		_ = json.NewEncoder(w).Encode(tea.TokenResponse{AccessToken: "at-123", TokenType: "Bearer", ExpiresIn: 3600})
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL)
	resp, err := c.ExchangeToken(context.Background(), "key-1", "secret-1")
	if err != nil {
		t.Fatalf("ExchangeToken: %v", err)
	}
	if resp.AccessToken != "at-123" || resp.TokenType != "Bearer" || resp.ExpiresIn != 3600 {
		t.Fatalf("resp = %+v", resp)
	}
	if gotKeyID != "key-1" || gotSecret != "secret-1" {
		t.Fatalf("server saw keyID=%q secret=%q, want key-1/secret-1", gotKeyID, gotSecret)
	}
	if gotGrantType != "client_credentials" {
		t.Fatalf("grant_type = %q, want client_credentials", gotGrantType)
	}
}

// TestExchangeTokenEscapesReservedCharacters confirms a keyID/secret
// containing a colon -- which would otherwise corrupt the "user:pass"
// Basic-auth encoding -- round-trips correctly, since ExchangeToken
// form-urlencodes each component before joining them (RFC 6749 section
// 2.3.1), the same way opentea's own POST /token unwinds it server-side
// (internal/api/token.go's basicAuthUnescaped).
func TestExchangeTokenEscapesReservedCharacters(t *testing.T) {
	var gotKeyID, gotSecret string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, _ := r.BasicAuth()
		uu, _ := url.QueryUnescape(u)
		up, _ := url.QueryUnescape(p)
		gotKeyID, gotSecret = uu, up
		_ = json.NewEncoder(w).Encode(tea.TokenResponse{AccessToken: "at", TokenType: "Bearer"})
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL)
	if _, err := c.ExchangeToken(context.Background(), "key:with:colons", "secret:too"); err != nil {
		t.Fatalf("ExchangeToken: %v", err)
	}
	if gotKeyID != "key:with:colons" || gotSecret != "secret:too" {
		t.Fatalf("server decoded keyID=%q secret=%q, want the original colon-containing values", gotKeyID, gotSecret)
	}
}

func TestExchangeTokenWrongSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(tea.TokenErrorResponse{Error: tea.TokenErrorInvalidClient})
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL)
	_, err := c.ExchangeToken(context.Background(), "key-1", "wrong-secret")
	if !IsUnauthorized(err) {
		t.Fatalf("ExchangeToken err = %v, want a 401 APIError", err)
	}
	code, ok := TEAErrorCode(err)
	if !ok || code != tea.TokenErrorInvalidClient {
		t.Fatalf("TEAErrorCode = %q, %v, want %q, true", code, ok, tea.TokenErrorInvalidClient)
	}
}

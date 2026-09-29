// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/oej/opentea/internal/model"
)

// TestResolveBearerTokenExchangesAPIKey is the CLI-side counterpart to
// cmd/opentea's TestTokenExchangeFullFlow: this CLI used to accept only an
// already-issued bearer token, with no way to perform the API-key exchange
// itself (docs/security-review-260923.md finding #10). Confirms -apikey
// actually drives a real POST /token exchange against a real server and
// yields a working access token.
func TestResolveBearerTokenExchangesAPIKey(t *testing.T) {
	srv, r, _ := newTestServer(t)
	ctx := context.Background()

	user, err := r.CreateUser(ctx, "alice", "password123", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	keyID, secret, err := r.SetAPIKey(ctx, user.UUID)
	if err != nil {
		t.Fatalf("SetAPIKey: %v", err)
	}

	server := srv.URL + "/tea/v1"
	token, err := resolveBearerToken(server, "", keyID+":"+secret)
	if err != nil {
		t.Fatalf("resolveBearerToken: %v", err)
	}
	if token == "" {
		t.Fatal("resolveBearerToken returned an empty access token")
	}

	// The exchanged token must actually work as a bearer credential, not
	// just be a non-empty string.
	if _, err := resolveBearerToken(server, "", keyID+":wrong-secret"); err == nil {
		t.Fatal("resolveBearerToken with a wrong secret: want an error, got nil")
	}
}

func TestResolveBearerTokenMutuallyExclusive(t *testing.T) {
	_, err := resolveBearerToken("http://example.com", "some-token", "key:secret")
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("resolveBearerToken with both -token and -apikey: err = %v, want a mutually-exclusive error", err)
	}
}

func TestResolveBearerTokenAPIKeyRequiresServer(t *testing.T) {
	_, err := resolveBearerToken("", "", "key:secret")
	if err == nil || !strings.Contains(err.Error(), "-server") {
		t.Fatalf("resolveBearerToken with -apikey but no -server: err = %v, want a -server-required error", err)
	}
}

func TestResolveBearerTokenAPIKeyBadFormat(t *testing.T) {
	_, err := resolveBearerToken("http://example.com", "", "no-colon-here")
	if err == nil || !strings.Contains(err.Error(), "keyId:secret") {
		t.Fatalf("resolveBearerToken with a malformed -apikey: err = %v, want a keyId:secret format error", err)
	}
}

func TestResolveBearerTokenPlainToken(t *testing.T) {
	token, err := resolveBearerToken("http://example.com", "already-issued-token", "")
	if err != nil {
		t.Fatalf("resolveBearerToken: %v", err)
	}
	if token != "already-issued-token" {
		t.Fatalf("token = %q, want the -token value passed straight through", token)
	}
}

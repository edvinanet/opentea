// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// token.go implements TEA 1.0's client_credentials token exchange
// (spec/openapi.yaml's /token, and upstream's fuller auth/readme.md):
// exchanges a user's API key (an identifier + secret pair, presented via
// HTTP Basic) for a short-lived access token, which is the only credential
// /tea/v1 accepts as Authorization: Bearer from here on -- see
// internal/authn.BearerUser and internal/repo's api_key/access_token
// tables (0009_token.sql).
package api

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/pkg/tea"
)

func (s *Server) requestToken(w http.ResponseWriter, r *http.Request) {
	keyID, secret, ok := basicAuthUnescaped(r)
	if !ok {
		httpx.TokenError(w, http.StatusUnauthorized, tea.TokenErrorInvalidClient, "missing or malformed Authorization: Basic header")
		return
	}

	if err := r.ParseForm(); err != nil {
		httpx.TokenError(w, http.StatusBadRequest, tea.TokenErrorInvalidRequest, "invalid application/x-www-form-urlencoded body: "+err.Error())
		return
	}
	if r.PostForm.Get("grant_type") != "client_credentials" {
		httpx.TokenError(w, http.StatusBadRequest, tea.TokenErrorUnsupportedGrantType, "this server supports only the client_credentials grant type")
		return
	}

	user, err := s.repo.VerifyAPIKey(r.Context(), keyID, secret)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.TokenError(w, http.StatusUnauthorized, tea.TokenErrorInvalidClient, "unknown API key or wrong secret")
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}

	token, _, err := s.repo.CreateAccessToken(r.Context(), user.UUID, s.cfg.AccessTokenTTL)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.TokenIssued(w, tea.TokenResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int(s.cfg.AccessTokenTTL.Seconds()),
	})
}

// basicAuthUnescaped is r.BasicAuth() with RFC 6749 section 2.3.1's
// pre-encoding unwound: the client_credentials identifier and secret are
// each application/x-www-form-urlencoded encoded before being joined with
// a colon and Base64 encoded, so any compliant client's credential
// containing a colon or non-ASCII characters round-trips correctly here --
// not only opentea's own generated key/secret values (plain base64url/
// UUID strings, which this unescape is a no-op for).
func basicAuthUnescaped(r *http.Request) (username, password string, ok bool) {
	u, p, ok := r.BasicAuth()
	if !ok {
		return "", "", false
	}
	uu, err := url.QueryUnescape(u)
	if err != nil {
		return "", "", false
	}
	up, err := url.QueryUnescape(p)
	if err != nil {
		return "", "", false
	}
	return uu, up, true
}

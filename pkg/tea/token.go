// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package tea

// TokenResponse is the body of a successful POST /token response (TEA 1.0,
// spec/openapi.yaml's token-response schema; RFC 6749 section 5.1).
type TokenResponse struct {
	// AccessToken is presented on every other TEA endpoint as
	// "Authorization: Bearer <access_token>". Opaque to the client.
	AccessToken string `json:"access_token"`
	// TokenType is always "Bearer" in TEA.
	TokenType string `json:"token_type"`
	// ExpiresIn is the access token's lifetime in seconds.
	ExpiresIn int `json:"expires_in,omitempty"`
	// Scope is the granted scope, when it differs from the scope requested.
	// TEA defines no scope values of its own; servers may ignore the
	// request-side parameter entirely, as opentea's own /token does.
	Scope string `json:"scope,omitempty"`
}

// TokenErrorResponse is the body of a failed POST /token response (TEA
// 1.0's token-error-response schema; RFC 6749 section 5.2). Deliberately a
// separate type from ErrorResponse/the unknown-error-type enum above: RFC
// 6749's error vocabulary (TokenError* below) is unrelated to the
// TEA-specific error-response schema every other /tea/v1 endpoint uses.
type TokenErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
	ErrorURI         string `json:"error_uri,omitempty"`
}

// RFC 6749 section 5.2's token-error-response error codes -- named
// TokenError* (not Error*) to keep them visually and namespace-distinct
// from the unrelated ErrorObjectUnknown/ErrorNoAcceptableFormat/etc.
// vocabulary above, since a caller could otherwise easily reach for the
// wrong constant family.
const (
	TokenErrorInvalidRequest       = "invalid_request"
	TokenErrorInvalidClient        = "invalid_client"
	TokenErrorInvalidGrant         = "invalid_grant"
	TokenErrorUnauthorizedClient   = "unauthorized_client"
	TokenErrorUnsupportedGrantType = "unsupported_grant_type"
	TokenErrorInvalidScope         = "invalid_scope"
)

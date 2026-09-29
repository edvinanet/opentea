// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Package pagination implements an opaque keyset-pagination cursor shared by
// all list endpoints in the read API.
package pagination

import (
	"encoding/base64"
	"encoding/json"
	"errors"
)

// ErrInvalidToken is returned by Decode when the token cannot be parsed.
var ErrInvalidToken = errors.New("pagination: invalid page token")

// Cursor identifies the last row of a page, so the next page can resume
// after it. SortField/SortOrder/Scope are carried along so a request can
// be rejected (400 INVALID_PAGE_TOKEN) if the client changes sort
// parameters, or any other result-affecting query/path parameter, mid-
// pagination. Scope is this cursor's binding to the exact endpoint and
// result-affecting parameters (parent uuid, idType/idValue filters, ...)
// that produced it (TEA 1.0's page-token parameter doc: "The token
// represents continuation state for the original query, including
// sortField, sortOrder, result-affecting filters... and path parameters.
// ... Clients shall not reuse a pageToken across different parent
// resource paths or different path uuid values") -- opaque to clients,
// meaningful only to internal/api's own pageScope, which builds it per
// endpoint (docs/security-review-260923.md finding #11: previously
// absent entirely, so a cursor from one endpoint validated successfully
// against a completely different one).
type Cursor struct {
	SortField string `json:"f"`
	SortOrder string `json:"o"`
	LastValue string `json:"v"`
	LastUUID  string `json:"u"`
	Scope     string `json:"s"`
}

// Encode serializes c into an opaque page token.
func Encode(c Cursor) string {
	b, err := json.Marshal(c)
	if err != nil {
		// Cursor only contains strings; Marshal cannot fail.
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// Decode parses a page token produced by Encode. Returns ErrInvalidToken if
// token is malformed.
func Decode(token string) (Cursor, error) {
	var c Cursor
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return c, ErrInvalidToken
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, ErrInvalidToken
	}
	if c.SortField == "" || c.SortOrder == "" || c.LastUUID == "" || c.Scope == "" {
		return c, ErrInvalidToken
	}
	return c, nil
}

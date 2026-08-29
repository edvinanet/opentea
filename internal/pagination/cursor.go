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
// after it. SortField/SortOrder are carried along so a request can be
// rejected (400) if the client changes sort parameters mid-pagination.
type Cursor struct {
	SortField string `json:"f"`
	SortOrder string `json:"o"`
	LastValue string `json:"v"`
	LastUUID  string `json:"u"`
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
	if c.SortField == "" || c.SortOrder == "" || c.LastUUID == "" {
		return c, ErrInvalidToken
	}
	return c, nil
}

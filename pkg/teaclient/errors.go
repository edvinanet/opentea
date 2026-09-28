// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teaclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"strings"
)

// APIError is returned by any Client method when the server responds with a
// non-2xx status. It carries the raw status, Content-Type, and body so
// callers (e.g. the CLI) can report exactly what the server said, not a
// generic failure -- ContentType specifically lets a caller distinguish a
// genuine TEA error-response from a same-status response that merely
// shares the status code (see TEAErrorCode).
type APIError struct {
	StatusCode  int
	ContentType string
	Body        []byte
}

func (e *APIError) Error() string {
	return fmt.Sprintf("tea API error: status %d: %s", e.StatusCode, e.Body)
}

// IsNotFound reports whether err is an APIError for a 404 response.
func IsNotFound(err error) bool { return hasStatus(err, 404) }

// IsUnauthorized reports whether err is an APIError for a 401 response
// (e.g. an invalid or expired bearer token).
func IsUnauthorized(err error) bool { return hasStatus(err, 401) }

// IsForbidden reports whether err is an APIError for a 403 response.
// BootstrapDiscover treats this the same as IsUnauthorized: an
// authentication/authorization error from one candidate endpoint must not
// trigger failover to the next one (TEA discovery spec).
func IsForbidden(err error) bool { return hasStatus(err, 403) }

// IsBadRequest reports whether err is an APIError for a 400 response.
func IsBadRequest(err error) bool { return hasStatus(err, 400) }

func hasStatus(err error, status int) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == status
	}
	return false
}

// TEAErrorCode reports the "error" value of a conformant TEA error-response,
// if err is one, and whether it is one at all. Per the TEA discovery spec's
// own definition (reused here for every TEA error status, not just
// /discovery's 404): "A response is a TEA error response only when its
// Content-Type is application/json (optionally with parameters such as
// charset) and the body is a JSON object with a string error property."
// ok is false for a non-APIError, a non-JSON Content-Type, or a JSON body
// with no string "error" property -- callers must not treat any of those
// as an authoritative TEA answer just because the status code matches (a
// reverse proxy's own error page, or a non-TEA JSON body, are not TEA
// error responses even at the same status -- docs/security-review-260923.md
// finding #9). An unrecognized error value is still reported, not
// rejected: "Clients shall ignore properties they do not recognize and
// shall not reject the response for an error value they do not know."
func TEAErrorCode(err error) (code string, ok bool) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return "", false
	}
	mediaType, _, parseErr := mime.ParseMediaType(apiErr.ContentType)
	if parseErr != nil || !strings.EqualFold(mediaType, "application/json") {
		return "", false
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(apiErr.Body, &body); err != nil || body.Error == "" {
		return "", false
	}
	return body.Error, true
}

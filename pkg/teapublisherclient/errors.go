// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teapublisherclient

import (
	"errors"
	"fmt"
)

// APIError is returned by any Client method when the target responds with
// a non-2xx status. It carries the raw status and body so callers (e.g.
// opentea-publisher's own handlers) can report exactly what the target
// said, not a generic failure.
type APIError struct {
	StatusCode int
	Body       []byte
}

func (e *APIError) Error() string {
	return fmt.Sprintf("teapublisher API error: status %d: %s", e.StatusCode, e.Body)
}

// IsNotFound reports whether err is an APIError for a 404 response.
func IsNotFound(err error) bool { return hasStatus(err, 404) }

// IsBadRequest reports whether err is an APIError for a 400 response.
func IsBadRequest(err error) bool { return hasStatus(err, 400) }

// IsUnauthorized reports whether err is an APIError for a 401 response
// (missing, invalid, or revoked bearer credential).
func IsUnauthorized(err error) bool { return hasStatus(err, 401) }

// IsForbidden reports whether err is an APIError for a 403 response --
// either the credential's scope doesn't permit the operation
// (design/publisher-service.md §10.4), or a self-approval was rejected
// (§7.9's maker-checker: approve/reject's actor equals the draft's own
// draftedBy).
func IsForbidden(err error) bool { return hasStatus(err, 403) }

// IsConflict reports whether err is an APIError for a 409 response --
// covers several distinct target-side states this package's callers may
// need to branch on: a collection draft locked by an outstanding
// prepareCollectionCommit, no current approval on record for
// prepareCollectionCommit/commit, a component identifier that already
// belongs to another component, or an artifact version that already has
// evidence submitted. Callers that need to distinguish these should
// inspect APIError.Body themselves -- the status code alone doesn't.
func IsConflict(err error) bool { return hasStatus(err, 409) }

func hasStatus(err error, status int) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == status
	}
	return false
}

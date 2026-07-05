package teaclient

import (
	"errors"
	"fmt"
)

// APIError is returned by any Client method when the server responds with a
// non-2xx status. It carries the raw status and body so callers (e.g. the
// CLI) can report exactly what the server said, not a generic failure.
type APIError struct {
	StatusCode int
	Body       []byte
}

func (e *APIError) Error() string {
	return fmt.Sprintf("tea API error: status %d: %s", e.StatusCode, e.Body)
}

// IsNotFound reports whether err is an APIError for a 404 response.
func IsNotFound(err error) bool { return hasStatus(err, 404) }

// IsUnauthorized reports whether err is an APIError for a 401 response
// (e.g. an invalid or expired bearer token).
func IsUnauthorized(err error) bool { return hasStatus(err, 401) }

// IsBadRequest reports whether err is an APIError for a 400 response.
func IsBadRequest(err error) bool { return hasStatus(err, 400) }

func hasStatus(err error, status int) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == status
	}
	return false
}

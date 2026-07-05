package repo

import "errors"

// ErrNotFound is returned by Get-style methods when the requested row
// doesn't exist. Handlers map it to a 404 OBJECT_UNKNOWN response.
var ErrNotFound = errors.New("repo: not found")

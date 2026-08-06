package api

import "github.com/oej/opentea/internal/pagination"

// cursorPart normalizes a page cursor into one ETag component -- "" for the
// first page (nil cursor), otherwise its already-canonical LastValue/
// LastUUID pair (equivalent to the raw pageToken, but doesn't require
// re-decoding it just to normalize).
func cursorPart(c *pagination.Cursor) string {
	if c == nil {
		return ""
	}
	return c.LastValue + "|" + c.LastUUID
}

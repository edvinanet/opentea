package httpx

import (
	"net/http"
	"strings"
)

// BuildETag joins parts with ':' and wraps them in the quoted form HTTP's
// ETag/If-None-Match headers require. Every part must already be safe to
// embed (no quote or comma characters) -- callers pass UUIDs, version
// numbers, and revision counters, none of which can contain either.
func BuildETag(parts ...string) string {
	return `"` + strings.Join(parts, ":") + `"`
}

// MatchesIfNoneMatch reports whether current matches any validator in the
// request's If-None-Match header values (as returned by
// r.Header.Values("If-None-Match")), per RFC 7232 -- multiple validators
// may be comma-separated within one header line or spread across several
// lines, "*" matches any existing resource, and -- per If-None-Match's own
// weak-comparison rule (unlike If-Match, which requires strong comparison)
// -- a leading "W/" is ignored on both sides, so a weak client validator
// can still match this server's strong one. Deliberately not a bare
// equality check against Header.Get: that would miss multi-valued headers
// and the wildcard case.
func MatchesIfNoneMatch(headerValues []string, current string) bool {
	want := strings.TrimPrefix(current, "W/")
	for _, line := range headerValues {
		for _, raw := range strings.Split(line, ",") {
			v := strings.TrimSpace(raw)
			if v == "*" {
				return true
			}
			if strings.TrimPrefix(v, "W/") == want {
				return true
			}
		}
	}
	return false
}

// WriteConditional sets ETag and Cache-Control on w, then reports whether
// the request's If-None-Match already matches etag. If it does, a bodyless
// 304 has already been written and the caller must return immediately
// without building or writing a response body. Otherwise the caller
// proceeds to fetch/serialize and write its normal response as usual --
// the headers set here are preserved through the caller's own WriteHeader
// call (e.g. via WriteJSON), since HTTP response headers must be set
// before the status line and this function runs first.
func WriteConditional(w http.ResponseWriter, r *http.Request, etag, cacheControl string) bool {
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", cacheControl)
	if MatchesIfNoneMatch(r.Header.Values("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	return false
}

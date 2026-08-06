package api

// Cache-Control values for /tea/v1 GET responses, applied alongside their
// ETag (see internal/httpx/etag.go's WriteConditional) -- all "public"
// since nothing in this API is request-dependent today (anonymous-by-default,
// a bearer token only identifies the caller, never restricts what they see).
const (
	// cacheControlImmutable is for representations keyed by an identity that
	// can never point at different content once it exists (a specific
	// artifact/collection version, addressed by (uuid, version) in the URL)
	// -- long-lived and never-revalidate, safe because the content really
	// can't change out from under a cached copy.
	cacheControlImmutable = "public, max-age=31536000, immutable"

	// cacheControlRevalidate is for representations that can change (or
	// disappear) over time -- a short freshness window plus
	// stale-while-revalidate so a cache can serve a slightly-stale copy
	// while it re-checks in the background, rather than blocking every
	// request past max-age on a synchronous revalidation.
	cacheControlRevalidate = "public, max-age=60, stale-while-revalidate=300"
)

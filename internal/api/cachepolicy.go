// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/oej/opentea/internal/httpx"
)

// Cache-Control base values for /tea/v1 GET responses, applied alongside
// their ETag (see internal/httpx/etag.go's WriteConditional) via
// conditional below, which adapts "public" to "private" for authenticated
// requests -- see conditional's doc comment for why.
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

// conditional builds this response's ETag from parts plus a principal-
// scoped suffix, applies the appropriate Cache-Control, and handles the
// conditional-request short-circuit -- the one call every /tea/v1 GET
// handler makes in place of a bare httpx.BuildETag + httpx.WriteConditional
// pair, now that authorization makes responses non-uniform across
// principals (see the two helpers below for why both the ETag and
// Cache-Control need to change, not just one).
//
// Returns true if it already wrote a response (a 304, or a 500 if the
// principal's entitlement watermark couldn't be read) -- the caller must
// return immediately, exactly like a bare httpx.WriteConditional call.
func (s *Server) conditional(w http.ResponseWriter, r *http.Request, cacheControlBase string, parts ...string) bool {
	suffix, err := s.authzCacheSuffix(r)
	if err != nil {
		httpx.InternalError(w, r, err)
		return true
	}
	etag := httpx.BuildETag(append(parts, suffix)...)
	return httpx.WriteConditional(w, r, etag, cacheControlFor(r, cacheControlBase))
}

// cacheControlFor adapts base for the current request's principal:
// anonymous responses stay exactly as declared (public, shared-cache-safe)
// since every anonymous caller sees identical content -- only
// 'everyone'-scoped entitlements can ever apply to them. An authenticated
// response becomes "private": safe for that one client's own cache, never
// safe for a shared/intermediary cache, since two different authenticated
// principals can get different authorized content for the identical URL
// (spec Sec 25.1: "Protected responses SHOULD use private cache
// controls").
func cacheControlFor(r *http.Request, base string) string {
	if !principalFromContext(r.Context()).IsAuthenticated() {
		return base
	}
	return strings.Replace(base, "public", "private", 1)
}

// authzCacheSuffix returns the ETag-partitioning key for the current
// request's principal: "anon" for anonymous callers (they all share one
// key, since their authorized view is identical), or a key derived from
// the principal's *effective entitlement state* for an authenticated one
// -- stable across requests from the same user with the same entitlements
// (so a client's own cache can still validate via If-None-Match), but
// different the instant their entitlements change, without needing to
// enumerate or hash the full candidate-rule set on every request. See
// Repo.GetPrincipalEntitlementWatermark's doc comment for exactly what
// changes this value: any create/update/delete of an entitlement
// applicable to this principal (including 'everyone'/'authenticated'
// ones, which apply to every authenticated caller too).
func (s *Server) authzCacheSuffix(r *http.Request) (string, error) {
	p := principalFromContext(r.Context())
	if !p.IsAuthenticated() {
		return "anon", nil
	}
	watermark, err := s.repo.GetPrincipalEntitlementWatermark(r.Context(), p.UserUUID)
	if err != nil {
		return "", err
	}
	return "u:" + p.UserUUID + ":" + strconv.FormatInt(watermark, 10), nil
}

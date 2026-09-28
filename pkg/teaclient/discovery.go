// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teaclient

import (
	"context"
	"fmt"
	"net/url"

	"github.com/oej/opentea/pkg/tea"
)

// Discover resolves a Transparency Exchange Identifier (TEI) via this
// server's GET /discovery endpoint. Per spec (TEA 1.0, spec/openapi.yaml)
// a match returns one or more candidate servers for the TEI; no match
// returns a 404 *APIError (IsNotFound), not an empty, successful slice.
//
// Note: this only queries the single server baseURL points at (its own
// self-authoritative discovery endpoint) -- for the full TEI-authority
// ".well-known" bootstrap flow (extract authority from the TEI, fetch a
// well-known document from that authority, then query one of the servers
// it lists, per CycloneDX/transparency-exchange-api's discovery/readme.md),
// use BootstrapDiscover instead. BootstrapDiscover itself calls this
// method once it has resolved which server to ask.
func (c *Client) Discover(ctx context.Context, tei string) ([]tea.DiscoveryInfo, error) {
	return c.discover(ctx, url.Values{"tei": {tei}})
}

// DiscoverByPURL resolves a Package URL (PURL) via this server's GET
// /discovery endpoint -- added alongside tei in upstream TEA 1.0
// (spec/openapi.yaml): "Discovery by PURL requires an already-known API
// base URL and resolves within that server's inventory," unlike tei
// discovery there's no ".well-known"-based authority to bootstrap from
// for a bare PURL, so this has no BootstrapDiscover-style counterpart --
// callers must already know which server to ask. Named DiscoverByPURL
// rather than renaming Discover to DiscoverByTEI to avoid breaking every
// existing Discover/BootstrapDiscover call site for a purely cosmetic
// symmetry; same success/no-match contract as Discover (see its own doc
// comment).
func (c *Client) DiscoverByPURL(ctx context.Context, purl string) ([]tea.DiscoveryInfo, error) {
	return c.discover(ctx, url.Values{"purl": {purl}})
}

// discover is Discover/DiscoverByPURL's shared implementation. The
// discovery spec is explicit that a successful lookup "shall return a
// non-empty array" -- an empty one is one of the spec's own named
// "invalid documents" failure classes ("an invalid /discovery success
// body"), not a legitimate zero-result answer; a real no-match is a 404
// OBJECT_UNKNOWN instead. The doc comments on Discover/DiscoverByPURL
// already stated this contract, but nothing enforced it
// (docs/security-review-260923.md finding #9) -- an empty array from a
// real server would previously have been accepted as success. Returns a
// plain error (not *APIError, since the HTTP response itself was a
// successful 2xx) -- isRetryableError's existing catch-all already treats
// this as a permanent, non-retryable-within-candidate content error, and
// BootstrapDiscover's own loop already fails over to the next candidate
// for any non-nil, non-authentication, non-authoritative-404 error, so no
// changes were needed there for this specific case to correctly count as
// one of the spec's failover triggers.
func (c *Client) discover(ctx context.Context, query url.Values) ([]tea.DiscoveryInfo, error) {
	var out []tea.DiscoveryInfo
	if err := c.do(ctx, "GET", "/discovery", query, &out); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("teaclient: /discovery returned a successful but empty result array, which TEA 1.0 treats as invalid (a real no-match answers 404 OBJECT_UNKNOWN instead)")
	}
	return out, nil
}

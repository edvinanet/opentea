// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teaclient

import (
	"context"
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
	var out []tea.DiscoveryInfo
	err := c.do(ctx, "GET", "/discovery", url.Values{"tei": {tei}}, &out)
	return out, err
}

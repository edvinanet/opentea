package teaclient

import (
	"context"
	"net/url"

	"github.com/oej/opentea/pkg/tea"
)

// Discover resolves a Transparency Exchange Identifier (TEI) via this
// server's GET /discovery endpoint. Per spec this may return zero, one, or
// more candidate servers for the TEI.
//
// Note: this only queries the single server baseURL points at (its own
// self-authoritative discovery endpoint). It does not perform the full
// TEI-authority ".well-known" bootstrap discovery described in
// discovery/readme.md -- that's a heavier flow (extract authority from the
// TEI, fetch a well-known document from that authority, then query one of
// the servers it lists) tracked as a follow-up, not implemented yet.
func (c *Client) Discover(ctx context.Context, tei string) ([]tea.DiscoveryInfo, error) {
	var out []tea.DiscoveryInfo
	err := c.do(ctx, "GET", "/discovery", url.Values{"tei": {tei}}, &out)
	return out, err
}

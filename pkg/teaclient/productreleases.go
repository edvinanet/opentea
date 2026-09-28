// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teaclient

import (
	"context"
	"strconv"

	"github.com/oej/opentea/pkg/tea"
)

// GetProductReleaseWithCollection fetches one product release together with
// its latest collection (GET /productRelease/{uuid}) -- named to match
// GetComponentReleaseWithCollection, since the server response always
// includes latestCollection (spec/openapi.yaml's product-release-with-collection),
// not the bare release this method used to decode into
// (docs/security-review-260923.md finding #5: a client type matching the
// old shape would have silently dropped latestCollection from every
// response, hiding the server-side bug from any test built on this
// method).
func (c *Client) GetProductReleaseWithCollection(ctx context.Context, uuid string) (tea.ProductReleaseWithCollection, error) {
	var pr tea.ProductReleaseWithCollection
	err := c.do(ctx, "GET", "/productRelease/"+uuid, nil, &pr)
	return pr, err
}

// QueryProductReleases lists product releases across all products,
// optionally filtered/sorted/paginated via params (GET /productReleases).
func (c *Client) QueryProductReleases(ctx context.Context, params ListParams) (tea.PaginatedProductReleases, error) {
	var resp tea.PaginatedProductReleases
	err := c.do(ctx, "GET", "/productReleases", params.values(), &resp)
	return resp, err
}

// GetLatestCollectionForProductRelease fetches the newest published
// collection for productReleaseUUID (GET /productRelease/{uuid}/collection/latest).
func (c *Client) GetLatestCollectionForProductRelease(ctx context.Context, productReleaseUUID string) (tea.Collection, error) {
	var col tea.Collection
	err := c.do(ctx, "GET", "/productRelease/"+productReleaseUUID+"/collection/latest", nil, &col)
	return col, err
}

// GetCollectionForProductRelease fetches one specific collection version
// for productReleaseUUID (GET /productRelease/{uuid}/collection/{version}).
func (c *Client) GetCollectionForProductRelease(ctx context.Context, productReleaseUUID string, version int) (tea.Collection, error) {
	var col tea.Collection
	err := c.do(ctx, "GET", "/productRelease/"+productReleaseUUID+"/collection/"+strconv.Itoa(version), nil, &col)
	return col, err
}

// ListCollectionsForProductRelease lists every collection version for
// productReleaseUUID (GET /productRelease/{uuid}/collections).
func (c *Client) ListCollectionsForProductRelease(ctx context.Context, productReleaseUUID string, params ListParams) (tea.PaginatedCollections, error) {
	var resp tea.PaginatedCollections
	err := c.do(ctx, "GET", "/productRelease/"+productReleaseUUID+"/collections", params.values(), &resp)
	return resp, err
}

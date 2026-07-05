package teaclient

import (
	"context"
	"strconv"

	"github.com/oej/opentea/pkg/tea"
)

func (c *Client) GetProductRelease(ctx context.Context, uuid string) (tea.ProductRelease, error) {
	var pr tea.ProductRelease
	err := c.do(ctx, "GET", "/productRelease/"+uuid, nil, &pr)
	return pr, err
}

func (c *Client) QueryProductReleases(ctx context.Context, params ListParams) (tea.PaginatedProductReleases, error) {
	var resp tea.PaginatedProductReleases
	err := c.do(ctx, "GET", "/productReleases", params.values(), &resp)
	return resp, err
}

func (c *Client) GetLatestCollectionForProductRelease(ctx context.Context, productReleaseUUID string) (tea.Collection, error) {
	var col tea.Collection
	err := c.do(ctx, "GET", "/productRelease/"+productReleaseUUID+"/collection/latest", nil, &col)
	return col, err
}

func (c *Client) GetCollectionForProductRelease(ctx context.Context, productReleaseUUID string, version int) (tea.Collection, error) {
	var col tea.Collection
	err := c.do(ctx, "GET", "/productRelease/"+productReleaseUUID+"/collection/"+strconv.Itoa(version), nil, &col)
	return col, err
}

func (c *Client) ListCollectionsForProductRelease(ctx context.Context, productReleaseUUID string, params ListParams) (tea.PaginatedCollections, error) {
	var resp tea.PaginatedCollections
	err := c.do(ctx, "GET", "/productRelease/"+productReleaseUUID+"/collections", params.values(), &resp)
	return resp, err
}

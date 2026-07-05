package teaclient

import (
	"context"

	"github.com/oej/opentea/pkg/tea"
)

func (c *Client) GetProduct(ctx context.Context, uuid string) (tea.Product, error) {
	var p tea.Product
	err := c.do(ctx, "GET", "/product/"+uuid, nil, &p)
	return p, err
}

func (c *Client) QueryProducts(ctx context.Context, params ListParams) (tea.PaginatedProducts, error) {
	var resp tea.PaginatedProducts
	err := c.do(ctx, "GET", "/products", params.values(), &resp)
	return resp, err
}

func (c *Client) ListReleasesByProduct(ctx context.Context, productUUID string, params ListParams) (tea.PaginatedProductReleases, error) {
	var resp tea.PaginatedProductReleases
	err := c.do(ctx, "GET", "/product/"+productUUID+"/releases", params.values(), &resp)
	return resp, err
}

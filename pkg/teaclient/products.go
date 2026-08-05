package teaclient

import (
	"context"

	"github.com/oej/opentea/pkg/tea"
)

// GetProduct fetches one product by UUID (GET /product/{uuid}).
func (c *Client) GetProduct(ctx context.Context, uuid string) (tea.Product, error) {
	var p tea.Product
	err := c.do(ctx, "GET", "/product/"+uuid, nil, &p)
	return p, err
}

// QueryProducts lists products, optionally filtered/sorted/paginated via
// params (GET /products).
func (c *Client) QueryProducts(ctx context.Context, params ListParams) (tea.PaginatedProducts, error) {
	var resp tea.PaginatedProducts
	err := c.do(ctx, "GET", "/products", params.values(), &resp)
	return resp, err
}

// ListReleasesByProduct lists productUUID's releases (GET /product/{uuid}/releases).
func (c *Client) ListReleasesByProduct(ctx context.Context, productUUID string, params ListParams) (tea.PaginatedProductReleases, error) {
	var resp tea.PaginatedProductReleases
	err := c.do(ctx, "GET", "/product/"+productUUID+"/releases", params.values(), &resp)
	return resp, err
}

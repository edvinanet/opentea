package teaclient

import (
	"context"

	"github.com/oej/opentea/pkg/tea"
)

// GetCLEByProduct fetches productUUID's lifecycle history (GET /product/{uuid}/cle).
func (c *Client) GetCLEByProduct(ctx context.Context, productUUID string) (tea.CLE, error) {
	var cle tea.CLE
	err := c.do(ctx, "GET", "/product/"+productUUID+"/cle", nil, &cle)
	return cle, err
}

// GetCLEByProductRelease fetches productReleaseUUID's lifecycle history
// (GET /productRelease/{uuid}/cle).
func (c *Client) GetCLEByProductRelease(ctx context.Context, productReleaseUUID string) (tea.CLE, error) {
	var cle tea.CLE
	err := c.do(ctx, "GET", "/productRelease/"+productReleaseUUID+"/cle", nil, &cle)
	return cle, err
}

// GetCLEByComponent fetches componentUUID's lifecycle history (GET /component/{uuid}/cle).
func (c *Client) GetCLEByComponent(ctx context.Context, componentUUID string) (tea.CLE, error) {
	var cle tea.CLE
	err := c.do(ctx, "GET", "/component/"+componentUUID+"/cle", nil, &cle)
	return cle, err
}

// GetCLEByComponentRelease fetches componentReleaseUUID's lifecycle history
// (GET /componentRelease/{uuid}/cle).
func (c *Client) GetCLEByComponentRelease(ctx context.Context, componentReleaseUUID string) (tea.CLE, error) {
	var cle tea.CLE
	err := c.do(ctx, "GET", "/componentRelease/"+componentReleaseUUID+"/cle", nil, &cle)
	return cle, err
}

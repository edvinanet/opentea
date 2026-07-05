package teaclient

import (
	"context"

	"github.com/oej/opentea/pkg/tea"
)

func (c *Client) GetCLEByProduct(ctx context.Context, productUUID string) (tea.CLE, error) {
	var cle tea.CLE
	err := c.do(ctx, "GET", "/product/"+productUUID+"/cle", nil, &cle)
	return cle, err
}

func (c *Client) GetCLEByProductRelease(ctx context.Context, productReleaseUUID string) (tea.CLE, error) {
	var cle tea.CLE
	err := c.do(ctx, "GET", "/productRelease/"+productReleaseUUID+"/cle", nil, &cle)
	return cle, err
}

func (c *Client) GetCLEByComponent(ctx context.Context, componentUUID string) (tea.CLE, error) {
	var cle tea.CLE
	err := c.do(ctx, "GET", "/component/"+componentUUID+"/cle", nil, &cle)
	return cle, err
}

func (c *Client) GetCLEByComponentRelease(ctx context.Context, componentReleaseUUID string) (tea.CLE, error) {
	var cle tea.CLE
	err := c.do(ctx, "GET", "/componentRelease/"+componentReleaseUUID+"/cle", nil, &cle)
	return cle, err
}

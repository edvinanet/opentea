package teaclient

import (
	"context"

	"github.com/oej/opentea/pkg/tea"
)

func (c *Client) GetComponent(ctx context.Context, uuid string) (tea.Component, error) {
	var comp tea.Component
	err := c.do(ctx, "GET", "/component/"+uuid, nil, &comp)
	return comp, err
}

func (c *Client) QueryComponents(ctx context.Context, params ListParams) (tea.PaginatedComponents, error) {
	var resp tea.PaginatedComponents
	err := c.do(ctx, "GET", "/components", params.values(), &resp)
	return resp, err
}

func (c *Client) ListReleasesByComponent(ctx context.Context, componentUUID string, params ListParams) (tea.PaginatedComponentReleases, error) {
	var resp tea.PaginatedComponentReleases
	err := c.do(ctx, "GET", "/component/"+componentUUID+"/releases", params.values(), &resp)
	return resp, err
}

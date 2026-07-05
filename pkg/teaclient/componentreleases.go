package teaclient

import (
	"context"
	"strconv"

	"github.com/oej/opentea/pkg/tea"
)

func (c *Client) GetComponentReleaseWithCollection(ctx context.Context, uuid string) (tea.ComponentReleaseWithCollection, error) {
	var out tea.ComponentReleaseWithCollection
	err := c.do(ctx, "GET", "/componentRelease/"+uuid, nil, &out)
	return out, err
}

func (c *Client) QueryComponentReleases(ctx context.Context, params ListParams) (tea.PaginatedComponentReleases, error) {
	var resp tea.PaginatedComponentReleases
	err := c.do(ctx, "GET", "/componentReleases", params.values(), &resp)
	return resp, err
}

func (c *Client) GetLatestCollectionForComponentRelease(ctx context.Context, componentReleaseUUID string) (tea.Collection, error) {
	var col tea.Collection
	err := c.do(ctx, "GET", "/componentRelease/"+componentReleaseUUID+"/collection/latest", nil, &col)
	return col, err
}

func (c *Client) GetCollectionForComponentRelease(ctx context.Context, componentReleaseUUID string, version int) (tea.Collection, error) {
	var col tea.Collection
	err := c.do(ctx, "GET", "/componentRelease/"+componentReleaseUUID+"/collection/"+strconv.Itoa(version), nil, &col)
	return col, err
}

func (c *Client) ListCollectionsForComponentRelease(ctx context.Context, componentReleaseUUID string, params ListParams) (tea.PaginatedCollections, error) {
	var resp tea.PaginatedCollections
	err := c.do(ctx, "GET", "/componentRelease/"+componentReleaseUUID+"/collections", params.values(), &resp)
	return resp, err
}

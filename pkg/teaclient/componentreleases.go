// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teaclient

import (
	"context"
	"strconv"

	"github.com/oej/opentea/pkg/tea"
)

// GetComponentReleaseWithCollection fetches one component release along
// with its latest collection in a single call (GET /componentRelease/{uuid}).
func (c *Client) GetComponentReleaseWithCollection(ctx context.Context, uuid string) (tea.ComponentReleaseWithCollection, error) {
	var out tea.ComponentReleaseWithCollection
	err := c.do(ctx, "GET", "/componentRelease/"+uuid, nil, &out)
	return out, err
}

// QueryComponentReleases lists component releases across all components,
// optionally filtered/sorted/paginated via params (GET /componentReleases).
func (c *Client) QueryComponentReleases(ctx context.Context, params ListParams) (tea.PaginatedComponentReleases, error) {
	var resp tea.PaginatedComponentReleases
	err := c.do(ctx, "GET", "/componentReleases", params.values(), &resp)
	return resp, err
}

// GetLatestCollectionForComponentRelease fetches the newest published
// collection for componentReleaseUUID (GET /componentRelease/{uuid}/collection/latest).
func (c *Client) GetLatestCollectionForComponentRelease(ctx context.Context, componentReleaseUUID string) (tea.Collection, error) {
	var col tea.Collection
	err := c.do(ctx, "GET", "/componentRelease/"+componentReleaseUUID+"/collection/latest", nil, &col)
	return col, err
}

// GetCollectionForComponentRelease fetches one specific collection version
// for componentReleaseUUID (GET /componentRelease/{uuid}/collection/{version}).
func (c *Client) GetCollectionForComponentRelease(ctx context.Context, componentReleaseUUID string, version int) (tea.Collection, error) {
	var col tea.Collection
	err := c.do(ctx, "GET", "/componentRelease/"+componentReleaseUUID+"/collection/"+strconv.Itoa(version), nil, &col)
	return col, err
}

// ListCollectionsForComponentRelease lists every collection version for
// componentReleaseUUID (GET /componentRelease/{uuid}/collections).
func (c *Client) ListCollectionsForComponentRelease(ctx context.Context, componentReleaseUUID string, params ListParams) (tea.PaginatedCollections, error) {
	var resp tea.PaginatedCollections
	err := c.do(ctx, "GET", "/componentRelease/"+componentReleaseUUID+"/collections", params.values(), &resp)
	return resp, err
}

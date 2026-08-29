// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teaclient

import (
	"context"

	"github.com/oej/opentea/pkg/tea"
)

// GetComponent fetches one component by UUID (GET /component/{uuid}).
func (c *Client) GetComponent(ctx context.Context, uuid string) (tea.Component, error) {
	var comp tea.Component
	err := c.do(ctx, "GET", "/component/"+uuid, nil, &comp)
	return comp, err
}

// QueryComponents lists components, optionally filtered/sorted/paginated
// via params (GET /components).
func (c *Client) QueryComponents(ctx context.Context, params ListParams) (tea.PaginatedComponents, error) {
	var resp tea.PaginatedComponents
	err := c.do(ctx, "GET", "/components", params.values(), &resp)
	return resp, err
}

// ListReleasesByComponent lists componentUUID's releases (GET /component/{uuid}/releases).
func (c *Client) ListReleasesByComponent(ctx context.Context, componentUUID string, params ListParams) (tea.PaginatedComponentReleases, error) {
	var resp tea.PaginatedComponentReleases
	err := c.do(ctx, "GET", "/component/"+componentUUID+"/releases", params.values(), &resp)
	return resp, err
}

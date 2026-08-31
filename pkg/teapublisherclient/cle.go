// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teapublisherclient

import (
	"context"

	"github.com/oej/opentea/pkg/tea"
	"github.com/oej/opentea/pkg/teapublisher"
)

// CreateProductCLEEvent records a lifecycle event against productUUID
// (POST /products/{uuid}/cle/events).
func (c *Client) CreateProductCLEEvent(ctx context.Context, productUUID string, in teapublisher.CLEEventCreate) (tea.CLEEvent, error) {
	var e tea.CLEEvent
	err := c.do(ctx, "POST", "/products/"+productUUID+"/cle/events", in, &e)
	return e, err
}

// CreateProductReleaseCLEEvent records a lifecycle event against
// productReleaseUUID (POST /productReleases/{uuid}/cle/events).
func (c *Client) CreateProductReleaseCLEEvent(ctx context.Context, productReleaseUUID string, in teapublisher.CLEEventCreate) (tea.CLEEvent, error) {
	var e tea.CLEEvent
	err := c.do(ctx, "POST", "/productReleases/"+productReleaseUUID+"/cle/events", in, &e)
	return e, err
}

// CreateComponentCLEEvent records a lifecycle event against componentUUID
// (POST /components/{uuid}/cle/events).
func (c *Client) CreateComponentCLEEvent(ctx context.Context, componentUUID string, in teapublisher.CLEEventCreate) (tea.CLEEvent, error) {
	var e tea.CLEEvent
	err := c.do(ctx, "POST", "/components/"+componentUUID+"/cle/events", in, &e)
	return e, err
}

// CreateComponentReleaseCLEEvent records a lifecycle event against
// componentReleaseUUID (POST /componentReleases/{uuid}/cle/events).
func (c *Client) CreateComponentReleaseCLEEvent(ctx context.Context, componentReleaseUUID string, in teapublisher.CLEEventCreate) (tea.CLEEvent, error) {
	var e tea.CLEEvent
	err := c.do(ctx, "POST", "/componentReleases/"+componentReleaseUUID+"/cle/events", in, &e)
	return e, err
}

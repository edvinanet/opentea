// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teapublisherclient

import (
	"context"

	"github.com/oej/opentea/pkg/tea"
	"github.com/oej/opentea/pkg/teapublisher"
)

// CreateProduct creates a product -- stable identity, no staging
// (POST /products).
func (c *Client) CreateProduct(ctx context.Context, in teapublisher.ProductCreate) (tea.Product, error) {
	var p tea.Product
	err := c.do(ctx, "POST", "/products", in, &p)
	return p, err
}

// CreateProductRelease creates a release of productUUID -- stable
// identity, no staging (POST /products/{uuid}/releases).
func (c *Client) CreateProductRelease(ctx context.Context, productUUID string, in teapublisher.ProductReleaseCreate) (tea.ProductRelease, error) {
	var pr tea.ProductRelease
	err := c.do(ctx, "POST", "/products/"+productUUID+"/releases", in, &pr)
	return pr, err
}

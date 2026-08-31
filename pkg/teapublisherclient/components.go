// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teapublisherclient

import (
	"context"
	"net/url"

	"github.com/oej/opentea/pkg/tea"
	"github.com/oej/opentea/pkg/teapublisher"
)

// FindComponents searches existing components by free-text match against
// name (GET /components?q=...) -- find-before-create, to avoid duplicate
// rows for the same real component. The target additionally enforces
// identifiers uniqueness server-side on CreateComponent (409 on conflict,
// design/publisher-openapi.yaml v0.10) -- this search is a courtesy for
// picking a good match, not what prevents duplicates.
func (c *Client) FindComponents(ctx context.Context, q string) ([]tea.Component, error) {
	path := "/components"
	if q != "" {
		path += "?q=" + url.QueryEscape(q)
	}
	var components []tea.Component
	err := c.do(ctx, "GET", path, nil, &components)
	return components, err
}

// CreateComponent creates a component (POST /components). Returns an
// *APIError with StatusCode 409 (IsConflict) if any of in.Identifiers
// already belongs to another component on the target.
func (c *Client) CreateComponent(ctx context.Context, in teapublisher.ComponentCreate) (tea.Component, error) {
	var comp tea.Component
	err := c.do(ctx, "POST", "/components", in, &comp)
	return comp, err
}

// CreateComponentRelease creates a release of componentUUID
// (POST /components/{uuid}/releases).
func (c *Client) CreateComponentRelease(ctx context.Context, componentUUID string, in teapublisher.ComponentReleaseCreate) (tea.ComponentRelease, error) {
	var cr tea.ComponentRelease
	err := c.do(ctx, "POST", "/components/"+componentUUID+"/releases", in, &cr)
	return cr, err
}

// LinkComponent links a component to a product release, optionally pinned
// to a specific component release (POST /productReleases/{uuid}/components).
// Returns the updated product release.
func (c *Client) LinkComponent(ctx context.Context, productReleaseUUID string, ref tea.ComponentRef) (tea.ProductRelease, error) {
	var pr tea.ProductRelease
	err := c.do(ctx, "POST", "/productReleases/"+productReleaseUUID+"/components", ref, &pr)
	return pr, err
}

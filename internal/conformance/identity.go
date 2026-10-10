// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package conformance

import (
	"context"
	"fmt"

	"github.com/oej/opentea/pkg/tea"
	"github.com/oej/opentea/pkg/teaclient"
)

// Resolver implements this suite's primary identity-resolution mechanism
// (see docs/consumer-api-conformance-test-rig.md's "Identifier-based
// resolution"): every reference-dataset entity carries a stable,
// deterministic identifier (a PURL, for every entity kind the datasets
// use one for), and the suite resolves that entity's real server-assigned
// identity via the specification's own identifier-filtered list
// operations, rather than ever assuming a bundle's authored UUID survived
// import unchanged. Calling the underlying list operation at all is
// itself part of endpoint coverage -- Resolver makes ordinary
// teaclient.Client calls, so CoverageTracker records them like any other.
type Resolver struct {
	client *teaclient.Client
}

// NewResolver wraps client for identity resolution.
func NewResolver(client *teaclient.Client) *Resolver {
	return &Resolver{client: client}
}

// errAmbiguousOrMissing is shared by every Resolve* method below: each
// expects its identifier-filtered query to return exactly one result --
// zero means the dataset wasn't loaded (or the server mishandled the
// filter), more than one means either a genuine data problem or the
// filter wasn't applied as a filter at all.
func errAmbiguousOrMissing(kind, idType, idValue string, got int) error {
	return fmt.Errorf("conformance: %s lookup by %s=%q returned %d results, want exactly 1", kind, idType, idValue, got)
}

// ResolveProductByPURL resolves a Product by one of its PURL identifiers
// (GET /products?idType=PURL&idValue=...).
func (r *Resolver) ResolveProductByPURL(ctx context.Context, purl string) (tea.Product, error) {
	resp, err := r.client.QueryProducts(ctx, teaclient.ListParams{IDType: tea.IdentifierTypePURL, IDValue: purl})
	if err != nil {
		return tea.Product{}, err
	}
	if len(resp.Results) != 1 {
		return tea.Product{}, errAmbiguousOrMissing("product", tea.IdentifierTypePURL, purl, len(resp.Results))
	}
	return resp.Results[0], nil
}

// ResolveProductReleaseByPURL resolves a ProductRelease by one of its PURL
// identifiers (GET /productReleases?idType=PURL&idValue=...).
func (r *Resolver) ResolveProductReleaseByPURL(ctx context.Context, purl string) (tea.ProductRelease, error) {
	resp, err := r.client.QueryProductReleases(ctx, teaclient.ListParams{IDType: tea.IdentifierTypePURL, IDValue: purl})
	if err != nil {
		return tea.ProductRelease{}, err
	}
	if len(resp.Results) != 1 {
		return tea.ProductRelease{}, errAmbiguousOrMissing("productRelease", tea.IdentifierTypePURL, purl, len(resp.Results))
	}
	return resp.Results[0], nil
}

// ResolveComponentByPURL resolves a Component by one of its PURL
// identifiers (GET /components?idType=PURL&idValue=...).
func (r *Resolver) ResolveComponentByPURL(ctx context.Context, purl string) (tea.Component, error) {
	resp, err := r.client.QueryComponents(ctx, teaclient.ListParams{IDType: tea.IdentifierTypePURL, IDValue: purl})
	if err != nil {
		return tea.Component{}, err
	}
	if len(resp.Results) != 1 {
		return tea.Component{}, errAmbiguousOrMissing("component", tea.IdentifierTypePURL, purl, len(resp.Results))
	}
	return resp.Results[0], nil
}

// ResolveComponentReleaseByPURL resolves a ComponentRelease by one of its
// PURL identifiers (GET /componentReleases?idType=PURL&idValue=...).
func (r *Resolver) ResolveComponentReleaseByPURL(ctx context.Context, purl string) (tea.ComponentRelease, error) {
	resp, err := r.client.QueryComponentReleases(ctx, teaclient.ListParams{IDType: tea.IdentifierTypePURL, IDValue: purl})
	if err != nil {
		return tea.ComponentRelease{}, err
	}
	if len(resp.Results) != 1 {
		return tea.ComponentRelease{}, errAmbiguousOrMissing("componentRelease", tea.IdentifierTypePURL, purl, len(resp.Results))
	}
	return resp.Results[0], nil
}

// purlOf returns the first PURL identifier's value among ids, and whether
// one was found -- every reference-dataset entity carries exactly one, so
// callers can treat ok=false as "this dataset entity has no PURL," a
// dataset-construction bug the suite should fail loudly on rather than
// resolve against the wrong identifier.
func purlOf(ids []tea.Identifier) (purl string, ok bool) {
	for _, id := range ids {
		if id.IDType == tea.IdentifierTypePURL {
			return id.IDValue, true
		}
	}
	return "", false
}

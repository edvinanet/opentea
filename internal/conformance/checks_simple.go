// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package conformance

import (
	"context"

	"github.com/oej/opentea/internal/testdataset"
	"github.com/oej/opentea/pkg/tea"
	"github.com/oej/opentea/pkg/teaclient"
)

// CheckSimple runs every data-correctness check the Simple reference
// dataset (internal/testdataset.Simple, see
// docs/consumer-api-conformance-test-rig.md's "Simple" row) supports
// against a live server reachable through client, recording one Check
// call per assertion on cor. The dataset must already be loaded on the
// server under test -- how it got there (opentea's own bundle import, or
// any other mechanism) is outside this package's concern.
func CheckSimple(ctx context.Context, client *teaclient.Client, cor *CorrectnessTracker) {
	m := testdataset.Simple()
	resolver := NewResolver(client)

	productPURL, _ := purlOf(m.Product.Identifiers)
	product, err := resolver.ResolveProductByPURL(ctx, productPURL)
	cor.Check(err == nil, "Simple: product resolves by PURL")
	if err != nil {
		return
	}
	checkMarkerUUID(cor, "Simple: product name marker", product.Name, m.Product.UUID)

	gotProduct, err := client.GetProduct(ctx, product.UUID)
	cor.Check(err == nil && gotProduct.UUID == product.UUID, "Simple: GET /product/{uuid} agrees with the identifier-filtered list")

	releases, err := client.ListReleasesByProduct(ctx, product.UUID, teaclient.ListParams{})
	cor.Check(err == nil && len(releases.Results) == 1, "Simple: product has exactly 1 release")

	_, err = client.GetCLEByProduct(ctx, product.UUID)
	cor.Check(err == nil, "Simple: product CLE fetch succeeds (even if empty)")

	prPURL, _ := purlOf(m.ProductReleases[0].Identifiers)
	pr, err := resolver.ResolveProductReleaseByPURL(ctx, prPURL)
	cor.Check(err == nil, "Simple: product release resolves by PURL")
	if err == nil {
		discovered, err := client.DiscoverByPURL(ctx, prPURL)
		cor.Check(err == nil && len(discovered) == 1 && discovered[0].ProductReleaseUUID == pr.UUID,
			"Simple: GET /discovery resolves the product release's own PURL")

		checkCollectionHistory(ctx, cor, "Simple: product release", m.ProductReleases[0].UUID,
			func(ctx context.Context) (tea.Collection, error) {
				return client.GetLatestCollectionForProductRelease(ctx, pr.UUID)
			},
			func(ctx context.Context) (tea.PaginatedCollections, error) {
				return client.ListCollectionsForProductRelease(ctx, pr.UUID, teaclient.ListParams{})
			},
			func(ctx context.Context, version int) (tea.Collection, error) {
				return client.GetCollectionForProductRelease(ctx, pr.UUID, version)
			},
		)
		_, err = client.GetCLEByProductRelease(ctx, pr.UUID)
		cor.Check(err == nil, "Simple: product release CLE fetch succeeds (even if empty)")
	}

	for i, entry := range m.Components {
		compPURL, _ := purlOf(entry.Identifiers)
		comp, err := resolver.ResolveComponentByPURL(ctx, compPURL)
		cor.Check(err == nil, "Simple: component resolves by PURL")
		if err != nil {
			continue
		}
		checkMarkerUUID(cor, "Simple: component name marker", comp.Name, entry.UUID)

		gotComponent, err := client.GetComponent(ctx, comp.UUID)
		cor.Check(err == nil && gotComponent.UUID == comp.UUID, "Simple: GET /component/{uuid} agrees with the identifier-filtered list")

		compReleases, err := client.ListReleasesByComponent(ctx, comp.UUID, teaclient.ListParams{})
		cor.Check(err == nil && len(compReleases.Results) == 2, "Simple: component has exactly 2 releases")

		_, err = client.GetCLEByComponent(ctx, comp.UUID)
		cor.Check(err == nil, "Simple: component CLE fetch succeeds (even if empty)")

		for v := 0; v < 2; v++ {
			crIdx := i*2 + v
			crEntry := m.ComponentReleases[crIdx]
			crPURL, _ := purlOf(crEntry.Identifiers)
			cr, err := resolver.ResolveComponentReleaseByPURL(ctx, crPURL)
			cor.Check(err == nil, "Simple: component release resolves by PURL")
			if err != nil {
				continue
			}
			checkCollectionHistory(ctx, cor, "Simple: component release", crEntry.UUID,
				func(ctx context.Context) (tea.Collection, error) {
					return client.GetLatestCollectionForComponentRelease(ctx, cr.UUID)
				},
				func(ctx context.Context) (tea.PaginatedCollections, error) {
					return client.ListCollectionsForComponentRelease(ctx, cr.UUID, teaclient.ListParams{})
				},
				func(ctx context.Context, version int) (tea.Collection, error) {
					return client.GetCollectionForComponentRelease(ctx, cr.UUID, version)
				},
			)
			_, err = client.GetCLEByComponentRelease(ctx, cr.UUID)
			cor.Check(err == nil, "Simple: component release CLE fetch succeeds (even if empty)")
		}
	}
}

// checkMarkerUUID is the common "does this live free-text field carry the
// marker we authored for this exact entity" check -- the embedded-marker
// cross-check docs/consumer-api-conformance-test-rig.md describes,
// catching an identifier-resolution bug that silently returned the wrong
// entity (same idType/idValue collision, a server-side filtering bug)
// rather than just a missing one.
func checkMarkerUUID(cor *CorrectnessTracker, label, liveName, wantUUID string) {
	got, ok := testdataset.ExtractMarkerUUID(liveName)
	cor.Check(ok && got == wantUUID, label)
}

// checkCollectionHistory asserts the 2-version collection history
// (version 1 -> 1 artifact, version 2 -> 1 artifact, ARTIFACT_UPDATED)
// every owner in the Simple dataset has, via whichever of the three
// collection-reading operations (latest/list/by-version) the caller's
// closures use -- shared between the product-release and
// component-release cases below, which differ only in which client
// methods (and hence which /tea/v1 paths) those closures call.
func checkCollectionHistory(ctx context.Context, cor *CorrectnessTracker, label, ownerAuthoredUUID string,
	getLatest func(ctx context.Context) (tea.Collection, error),
	list func(ctx context.Context) (tea.PaginatedCollections, error),
	getVersion func(ctx context.Context, version int) (tea.Collection, error),
) {
	latest, err := getLatest(ctx)
	cor.Check(err == nil, label+": latest collection fetch succeeds")
	if err == nil {
		cor.Check(latest.Version == 2, label+": latest collection is version 2")
		cor.Check(len(latest.Artifacts) == 1, label+": latest collection has exactly 1 artifact")
		checkCollectionMarker(cor, label+": latest collection marker matches version 2", latest, ownerAuthoredUUID, 2)
	}

	all, err := list(ctx)
	cor.Check(err == nil && len(all.Results) == 2, label+": list collections returns exactly 2 versions")

	v1, err := getVersion(ctx, 1)
	cor.Check(err == nil, label+": version 1 collection fetch succeeds")
	if err == nil {
		cor.Check(len(v1.Artifacts) == 1, label+": version 1 collection has exactly 1 artifact")
		checkCollectionMarker(cor, label+": version 1 collection marker matches version 1", v1, ownerAuthoredUUID, 1)
	}
}

// checkCollectionMarker is checkCollectionHistory's own marker cross-check
// (see checkMarkerUUID's doc comment for why this matters), specialized
// for a Collection's UpdateReason.Comment rather than a Name field.
func checkCollectionMarker(cor *CorrectnessTracker, label string, col tea.Collection, wantOwnerUUID string, wantVersion int) {
	if col.UpdateReason == nil {
		cor.Check(false, label)
		return
	}
	gotUUID, gotVersion, ok := testdataset.ExtractCollectionMarker(col.UpdateReason.Comment)
	cor.Check(ok && gotUUID == wantOwnerUUID && gotVersion == wantVersion, label)
}

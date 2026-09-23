// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"testing"
	"time"

	"github.com/oej/opentea/pkg/tea"
)

func TestProductReleaseCreateGetLinkComponent(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	product, err := r.CreateProduct(ctx, "Acme Widget", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	component, err := r.CreateComponent(ctx, "acme-widget-core", nil)
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	componentRelease, err := r.CreateComponentRelease(ctx, component.UUID, ComponentReleaseInput{
		Version:     "1.0.0",
		CreatedDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateComponentRelease: %v", err)
	}

	pr, err := r.CreateProductRelease(ctx, product.UUID, ProductReleaseInput{
		Version:     "1.0.0",
		CreatedDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		Identifiers: []tea.Identifier{{IDType: "TEI", IDValue: "urn:tea:acme:widget:1.0.0"}},
	})
	if err != nil {
		t.Fatalf("CreateProductRelease: %v", err)
	}
	if pr.Product != product.UUID {
		t.Fatalf("Product = %v, want %s", pr.Product, product.UUID)
	}
	if pr.ProductName != "Acme Widget" {
		t.Fatalf("ProductName = %q", pr.ProductName)
	}
	if len(pr.Components) != 0 {
		t.Fatalf("expected no components yet, got %+v", pr.Components)
	}

	updated, err := r.LinkComponent(ctx, pr.UUID, tea.ComponentRef{UUID: component.UUID, Release: &componentRelease.UUID})
	if err != nil {
		t.Fatalf("LinkComponent: %v", err)
	}
	if len(updated.Components) != 1 || updated.Components[0].UUID != component.UUID {
		t.Fatalf("Components = %+v", updated.Components)
	}
	if updated.Components[0].Release == nil || *updated.Components[0].Release != componentRelease.UUID {
		t.Fatalf("Components[0].Release = %v, want %s", updated.Components[0].Release, componentRelease.UUID)
	}

	// Re-linking the same component replaces the pin rather than duplicating the row.
	updated2, err := r.LinkComponent(ctx, pr.UUID, tea.ComponentRef{UUID: component.UUID})
	if err != nil {
		t.Fatalf("LinkComponent (re-link): %v", err)
	}
	if len(updated2.Components) != 1 {
		t.Fatalf("expected re-link to replace, got %+v", updated2.Components)
	}
	if updated2.Components[0].Release != nil {
		t.Fatalf("expected pin cleared, got %v", updated2.Components[0].Release)
	}
}

// TestProductReleaseRevisionBumpsOnLinkComponent is the regression test for
// product release ETag support: LinkComponent changes what
// GetProductRelease's .Components embeds, so it must bump the release's
// revision every time it's called -- both for a genuinely new link and for
// a re-pin -- so a client's cached ETag stops matching.
func TestProductReleaseRevisionBumpsOnLinkComponent(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	product, err := r.CreateProduct(ctx, "Acme Widget", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	component, err := r.CreateComponent(ctx, "acme-widget-core", nil)
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	pr, err := r.CreateProductRelease(ctx, product.UUID, ProductReleaseInput{
		Version: "1.0.0", CreatedDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateProductRelease: %v", err)
	}

	rev, err := r.GetProductReleaseRevision(ctx, pr.UUID)
	if err != nil {
		t.Fatalf("GetProductReleaseRevision: %v", err)
	}
	if rev != 1 {
		t.Fatalf("revision = %d, want 1 for a freshly created release", rev)
	}

	if _, err := r.LinkComponent(ctx, pr.UUID, tea.ComponentRef{UUID: component.UUID}); err != nil {
		t.Fatalf("LinkComponent: %v", err)
	}
	rev, err = r.GetProductReleaseRevision(ctx, pr.UUID)
	if err != nil {
		t.Fatalf("GetProductReleaseRevision after LinkComponent: %v", err)
	}
	if rev != 2 {
		t.Fatalf("revision after LinkComponent = %d, want 2", rev)
	}

	// Re-linking bumps again, even though it's arguably a no-op content-wise.
	if _, err := r.LinkComponent(ctx, pr.UUID, tea.ComponentRef{UUID: component.UUID}); err != nil {
		t.Fatalf("LinkComponent (re-link): %v", err)
	}
	rev, err = r.GetProductReleaseRevision(ctx, pr.UUID)
	if err != nil {
		t.Fatalf("GetProductReleaseRevision after re-link: %v", err)
	}
	if rev != 3 {
		t.Fatalf("revision after re-link = %d, want 3", rev)
	}
}

func TestGetProductReleaseRevisionNotFound(t *testing.T) {
	r := newTestRepo(t)
	if _, err := r.GetProductReleaseRevision(context.Background(), "00000000-0000-4000-8000-000000000000"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestProductReleaseRevisionBumpsOnImportComponentLink mirrors
// TestProductReleaseRevisionBumpsOnLinkComponent but for the bundle-import
// path -- a genuinely new link bumps the revision; a matching re-import
// (idempotent no-op) must not.
func TestProductReleaseRevisionBumpsOnImportComponentLink(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	product, err := r.CreateProduct(ctx, "Acme Widget", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	component, err := r.CreateComponent(ctx, "acme-widget-core", nil)
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	pr, err := r.CreateProductRelease(ctx, product.UUID, ProductReleaseInput{
		Version: "1.0.0", CreatedDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateProductRelease: %v", err)
	}

	if err := r.ImportComponentLink(ctx, pr.UUID, tea.ComponentRef{UUID: component.UUID}); err != nil {
		t.Fatalf("ImportComponentLink: %v", err)
	}
	rev, err := r.GetProductReleaseRevision(ctx, pr.UUID)
	if err != nil {
		t.Fatalf("GetProductReleaseRevision: %v", err)
	}
	if rev != 2 {
		t.Fatalf("revision after ImportComponentLink = %d, want 2", rev)
	}

	// Re-importing the identical link is an idempotent no-op -- no bump.
	if err := r.ImportComponentLink(ctx, pr.UUID, tea.ComponentRef{UUID: component.UUID}); err != nil {
		t.Fatalf("ImportComponentLink (re-import): %v", err)
	}
	rev, err = r.GetProductReleaseRevision(ctx, pr.UUID)
	if err != nil {
		t.Fatalf("GetProductReleaseRevision after re-import: %v", err)
	}
	if rev != 2 {
		t.Fatalf("revision after idempotent re-import = %d, want unchanged at 2", rev)
	}
}

func TestComponentReleaseWithDistribution(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	component, err := r.CreateComponent(ctx, "tomcat", nil)
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	cr, err := r.CreateComponentRelease(ctx, component.UUID, ComponentReleaseInput{
		Version:     "11.0.7",
		CreatedDate: time.Date(2026, 5, 7, 18, 8, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateComponentRelease: %v", err)
	}

	dist, err := r.CreateDistribution(ctx, cr.UUID, "linux/amd64 tarball")
	if err != nil {
		t.Fatalf("CreateDistribution: %v", err)
	}
	if dist.DistributionID == "" {
		t.Fatal("expected generated distribution id")
	}

	updatedDist, err := r.SetDistributionFile(ctx, dist.DistributionID, "http://localhost/files/abc123", "abc123")
	if err != nil {
		t.Fatalf("SetDistributionFile: %v", err)
	}
	if updatedDist.URL != "http://localhost/files/abc123" {
		t.Fatalf("URL = %q", updatedDist.URL)
	}
	if len(updatedDist.Checksums) != 1 || updatedDist.Checksums[0].AlgType != "SHA-256" {
		t.Fatalf("Checksums = %+v", updatedDist.Checksums)
	}

	got, err := r.GetComponentRelease(ctx, cr.UUID)
	if err != nil {
		t.Fatalf("GetComponentRelease: %v", err)
	}
	if len(got.Distributions) != 1 || got.Distributions[0].URL != "http://localhost/files/abc123" {
		t.Fatalf("Distributions = %+v", got.Distributions)
	}
}

func TestCreateDistributionUnknownRelease(t *testing.T) {
	r := newTestRepo(t)
	_, err := r.CreateDistribution(context.Background(), "00000000-0000-4000-8000-000000000000", "x")
	if err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestComponentReleaseRevisionBumpsOnDistributionChanges is the regression
// test for component release ETag support: GetComponentRelease's response
// embeds .Distributions, so both creating a new distribution and uploading
// a file for an existing one (which changes its URL/checksums) must bump
// the owning component release's revision.
func TestComponentReleaseRevisionBumpsOnDistributionChanges(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	component, err := r.CreateComponent(ctx, "tomcat", nil)
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	cr, err := r.CreateComponentRelease(ctx, component.UUID, ComponentReleaseInput{
		Version: "11.0.7", CreatedDate: time.Date(2026, 5, 7, 18, 8, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateComponentRelease: %v", err)
	}
	rev, err := r.GetComponentReleaseRevision(ctx, cr.UUID)
	if err != nil {
		t.Fatalf("GetComponentReleaseRevision: %v", err)
	}
	if rev != 1 {
		t.Fatalf("revision = %d, want 1 for a freshly created release", rev)
	}

	dist, err := r.CreateDistribution(ctx, cr.UUID, "linux/amd64 tarball")
	if err != nil {
		t.Fatalf("CreateDistribution: %v", err)
	}
	rev, err = r.GetComponentReleaseRevision(ctx, cr.UUID)
	if err != nil {
		t.Fatalf("GetComponentReleaseRevision after CreateDistribution: %v", err)
	}
	if rev != 2 {
		t.Fatalf("revision after CreateDistribution = %d, want 2", rev)
	}

	if _, err := r.SetDistributionFile(ctx, dist.DistributionID, "http://localhost/files/abc123", "abc123"); err != nil {
		t.Fatalf("SetDistributionFile: %v", err)
	}
	rev, err = r.GetComponentReleaseRevision(ctx, cr.UUID)
	if err != nil {
		t.Fatalf("GetComponentReleaseRevision after SetDistributionFile: %v", err)
	}
	if rev != 3 {
		t.Fatalf("revision after SetDistributionFile = %d, want 3", rev)
	}
}

func TestGetComponentReleaseRevisionNotFound(t *testing.T) {
	r := newTestRepo(t)
	if _, err := r.GetComponentReleaseRevision(context.Background(), "00000000-0000-4000-8000-000000000000"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestLatestCollectionVersionTx (via ImportCollection since
// latestCollectionVersionTx itself is unexported) confirms the light
// latest-version lookup used by getComponentReleaseWithCollection's ETag
// tracks the same "no collection yet" -> "version 1" -> "version 2"
// transitions GetLatestCollection itself observes.
func TestComponentReleaseLatestCollectionVersionTracksNewCollections(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	component, err := r.CreateComponent(ctx, "tomcat", nil)
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	cr, err := r.CreateComponentRelease(ctx, component.UUID, ComponentReleaseInput{
		Version: "11.0.7", CreatedDate: time.Date(2026, 5, 7, 18, 8, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateComponentRelease: %v", err)
	}

	if _, err := r.GetLatestCollection(ctx, cr.UUID, BelongsToComponentRelease); err != ErrNotFound {
		t.Fatalf("GetLatestCollection (no collection yet): err = %v, want ErrNotFound", err)
	}

	c1, err := r.CreateCollectionForComponentRelease(ctx, cr.UUID, CollectionInput{})
	if err != nil {
		t.Fatalf("CreateCollectionForComponentRelease: %v", err)
	}
	latest, err := r.GetLatestCollection(ctx, cr.UUID, BelongsToComponentRelease)
	if err != nil {
		t.Fatalf("GetLatestCollection: %v", err)
	}
	if latest.Version != c1.Version {
		t.Fatalf("latest.Version = %d, want %d", latest.Version, c1.Version)
	}

	c2, err := r.CreateCollectionForComponentRelease(ctx, cr.UUID, CollectionInput{})
	if err != nil {
		t.Fatalf("CreateCollectionForComponentRelease (2nd): %v", err)
	}
	latest, err = r.GetLatestCollection(ctx, cr.UUID, BelongsToComponentRelease)
	if err != nil {
		t.Fatalf("GetLatestCollection (after 2nd): %v", err)
	}
	if latest.Version != c2.Version || latest.Version == c1.Version {
		t.Fatalf("latest.Version = %d, want the newer %d (not the original %d)", latest.Version, c2.Version, c1.Version)
	}
}

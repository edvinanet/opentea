package repo

import (
	"context"
	"testing"
	"time"

	"github.com/oej/opentea/pkg/tea"
)

// TestWatermarkStartsAtZero confirms GetWatermark returns 0, not an error,
// for a family that's never been bumped -- matching bumpWatermarkTx's
// upsert-from-nothing behavior.
func TestWatermarkStartsAtZero(t *testing.T) {
	r := newTestRepo(t)
	w, err := r.GetWatermark(context.Background(), WatermarkProducts)
	if err != nil {
		t.Fatalf("GetWatermark: %v", err)
	}
	if w != 0 {
		t.Fatalf("watermark = %d, want 0 for a never-bumped family", w)
	}
}

// TestWatermarkBumpsOnEveryFamily is the regression test for list-endpoint
// ETag support: every write identified in the bump mapping increments its
// resource family's watermark by exactly 1, and different families don't
// cross-contaminate each other.
func TestWatermarkBumpsOnEveryFamily(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	assertWatermark := func(family string, want int64) {
		t.Helper()
		got, err := r.GetWatermark(ctx, family)
		if err != nil {
			t.Fatalf("GetWatermark(%q): %v", family, err)
		}
		if got != want {
			t.Fatalf("watermark(%q) = %d, want %d", family, got, want)
		}
	}

	// products: CreateProduct
	product, err := r.CreateProduct(ctx, "A", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	assertWatermark(WatermarkProducts, 1)
	assertWatermark(WatermarkComponents, 0) // unrelated family untouched

	// components: CreateComponent
	component, err := r.CreateComponent(ctx, "acme-widget-core", nil)
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	assertWatermark(WatermarkComponents, 1)

	// productReleases: CreateProductRelease
	pr, err := r.CreateProductRelease(ctx, product.UUID, ProductReleaseInput{
		Version: "1.0.0", CreatedDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateProductRelease: %v", err)
	}
	assertWatermark(WatermarkProductReleases, 1)

	// productReleases: LinkComponent bumps again
	if _, err := r.LinkComponent(ctx, pr.UUID, tea.ComponentRef{UUID: component.UUID}); err != nil {
		t.Fatalf("LinkComponent: %v", err)
	}
	assertWatermark(WatermarkProductReleases, 2)

	// componentReleases: CreateComponentRelease
	cr, err := r.CreateComponentRelease(ctx, component.UUID, ComponentReleaseInput{
		Version: "1.0.0", CreatedDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateComponentRelease: %v", err)
	}
	assertWatermark(WatermarkComponentReleases, 1)

	// componentReleases: CreateDistribution bumps again
	dist, err := r.CreateDistribution(ctx, cr.UUID, "linux/amd64 tarball")
	if err != nil {
		t.Fatalf("CreateDistribution: %v", err)
	}
	assertWatermark(WatermarkComponentReleases, 2)

	// componentReleases: SetDistributionFile bumps again
	if _, err := r.SetDistributionFile(ctx, dist.DistributionID, "http://localhost/files/abc", "abc"); err != nil {
		t.Fatalf("SetDistributionFile: %v", err)
	}
	assertWatermark(WatermarkComponentReleases, 3)

	// collections: CreateCollectionForComponentRelease
	if _, err := r.CreateCollectionForComponentRelease(ctx, cr.UUID, CollectionInput{}); err != nil {
		t.Fatalf("CreateCollectionForComponentRelease: %v", err)
	}
	assertWatermark(WatermarkCollections, 1)

	// Deletes bump their own family too.
	if err := r.DeleteComponentRelease(ctx, cr.UUID); err != nil {
		t.Fatalf("DeleteComponentRelease: %v", err)
	}
	assertWatermark(WatermarkComponentReleases, 4)

	if err := r.DeleteProductRelease(ctx, pr.UUID); err != nil {
		t.Fatalf("DeleteProductRelease: %v", err)
	}
	assertWatermark(WatermarkProductReleases, 3)

	if err := r.DeleteComponent(ctx, component.UUID); err != nil {
		t.Fatalf("DeleteComponent: %v", err)
	}
	assertWatermark(WatermarkComponents, 2)

	if err := r.DeleteProduct(ctx, product.UUID); err != nil {
		t.Fatalf("DeleteProduct: %v", err)
	}
	assertWatermark(WatermarkProducts, 2)
}

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
	if pr.Product == nil || *pr.Product != product.UUID {
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

// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"testing"
	"time"
)

func TestGetStats(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	p1, err := r.CreateProduct(ctx, "p1", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if _, err := r.CreateProduct(ctx, "p2", nil); err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if _, err := r.CreateProductRelease(ctx, p1.UUID, ProductReleaseInput{Version: "1.0.0", CreatedDate: time.Now()}); err != nil {
		t.Fatalf("CreateProductRelease: %v", err)
	}

	c1, err := r.CreateComponent(ctx, "c1", nil)
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	cr, err := r.CreateComponentRelease(ctx, c1.UUID, ComponentReleaseInput{Version: "1.0.0", CreatedDate: time.Now()})
	if err != nil {
		t.Fatalf("CreateComponentRelease: %v", err)
	}

	// Two collection *versions* for the same owner should count as one collection.
	if _, err := r.CreateCollectionForComponentRelease(ctx, cr.UUID, CollectionInput{}); err != nil {
		t.Fatalf("CreateCollectionForComponentRelease (v1): %v", err)
	}
	if _, err := r.CreateCollectionForComponentRelease(ctx, cr.UUID, CollectionInput{}); err != nil {
		t.Fatalf("CreateCollectionForComponentRelease (v2): %v", err)
	}

	artifact, err := r.CreateArtifact(ctx, ArtifactInput{Type: "BOM", Formats: []ArtifactFormatInput{{MediaType: "application/json"}}})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	// Directly insert a second revision row for the same artifact uuid --
	// there's no "new revision" repo API yet (out of phase-1 scope), so this
	// exercises the distinct-uuid counting behavior via raw SQL instead.
	if _, err := r.db.ExecContext(ctx, `INSERT INTO artifact (uuid, version, type) VALUES (?, 2, 'BOM')`, artifact.UUID); err != nil {
		t.Fatalf("insert artifact v2: %v", err)
	}

	stats, err := r.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.Products != 2 {
		t.Errorf("Products = %d, want 2", stats.Products)
	}
	if stats.ProductReleases != 1 {
		t.Errorf("ProductReleases = %d, want 1", stats.ProductReleases)
	}
	if stats.Components != 1 {
		t.Errorf("Components = %d, want 1", stats.Components)
	}
	if stats.ComponentReleases != 1 {
		t.Errorf("ComponentReleases = %d, want 1", stats.ComponentReleases)
	}
	if stats.Collections != 1 {
		t.Errorf("Collections = %d, want 1 (two versions of the same collection)", stats.Collections)
	}
	if stats.Artifacts != 1 {
		t.Errorf("Artifacts = %d, want 1 (two revisions of the same artifact)", stats.Artifacts)
	}
}

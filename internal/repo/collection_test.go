// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"testing"
	"time"

	"github.com/oej/opentea/pkg/tea"
)

func TestCollectionVersioningAndArtifactReuse(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	component, err := r.CreateComponent(ctx, "tomcat", nil)
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	cr, err := r.CreateComponentRelease(ctx, component.UUID, ComponentReleaseInput{
		Version:     "11.0.7",
		CreatedDate: time.Now(),
	})
	if err != nil {
		t.Fatalf("CreateComponentRelease: %v", err)
	}

	sbom, err := r.CreateArtifact(ctx, ArtifactInput{Name: "Build SBOM", Type: "BOM", Formats: []ArtifactFormatInput{{MediaType: "application/json"}}})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	c1, err := r.CreateCollectionForComponentRelease(ctx, cr.UUID, CollectionInput{
		UpdateReason: &tea.UpdateReason{Type: "INITIAL_RELEASE", Comment: "first collection"},
		Artifacts:    []ArtifactRef{{UUID: sbom.UUID, Version: sbom.Version}},
	})
	if err != nil {
		t.Fatalf("CreateCollectionForComponentRelease: %v", err)
	}
	if c1.Version != 1 {
		t.Fatalf("Version = %d, want 1", c1.Version)
	}
	if c1.BelongsTo != BelongsToComponentRelease {
		t.Fatalf("BelongsTo = %q", c1.BelongsTo)
	}
	if len(c1.Artifacts) != 1 || c1.Artifacts[0].UUID != sbom.UUID {
		t.Fatalf("Artifacts = %+v", c1.Artifacts)
	}

	vex, err := r.CreateArtifact(ctx, ArtifactInput{Name: "VDR", Type: "VULNERABILITIES", Formats: []ArtifactFormatInput{{MediaType: "application/json"}}})
	if err != nil {
		t.Fatalf("CreateArtifact (vex): %v", err)
	}
	c2, err := r.CreateCollectionForComponentRelease(ctx, cr.UUID, CollectionInput{
		UpdateReason: &tea.UpdateReason{Type: "ARTIFACT_ADDED"},
		Artifacts:    []ArtifactRef{{UUID: sbom.UUID, Version: sbom.Version}, {UUID: vex.UUID, Version: vex.Version}},
	})
	if err != nil {
		t.Fatalf("CreateCollectionForComponentRelease (v2): %v", err)
	}
	if c2.Version != 2 {
		t.Fatalf("Version = %d, want 2 (must increment per owner)", c2.Version)
	}
	if len(c2.Artifacts) != 2 {
		t.Fatalf("Artifacts = %+v, want sbom reused + vex added", c2.Artifacts)
	}

	latest, err := r.GetLatestCollection(ctx, cr.UUID, BelongsToComponentRelease)
	if err != nil {
		t.Fatalf("GetLatestCollection: %v", err)
	}
	if latest.Version != 2 {
		t.Fatalf("latest.Version = %d, want 2", latest.Version)
	}

	v1, err := r.GetCollectionByVersion(ctx, cr.UUID, 1, BelongsToComponentRelease)
	if err != nil {
		t.Fatalf("GetCollectionByVersion(1): %v", err)
	}
	if len(v1.Artifacts) != 1 {
		t.Fatalf("v1.Artifacts = %+v, want unaffected by v2's addition", v1.Artifacts)
	}

	all, err := r.ListCollections(ctx, cr.UUID, "desc", nil, 10, BelongsToComponentRelease)
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(all) != 2 || all[0].Version != 2 || all[1].Version != 1 {
		t.Fatalf("ListCollections (desc) = %+v", all)
	}
}

func TestCreateCollectionUnknownOwner(t *testing.T) {
	r := newTestRepo(t)
	_, err := r.CreateCollectionForComponentRelease(context.Background(), "00000000-0000-4000-8000-000000000000", CollectionInput{})
	if err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetLatestCollectionNotFound(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	component, err := r.CreateComponent(ctx, "empty", nil)
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	cr, err := r.CreateComponentRelease(ctx, component.UUID, ComponentReleaseInput{Version: "1.0.0", CreatedDate: time.Now()})
	if err != nil {
		t.Fatalf("CreateComponentRelease: %v", err)
	}
	if _, err := r.GetLatestCollection(ctx, cr.UUID, BelongsToComponentRelease); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestExistsCollectionVersion confirms the existence check correctly
// distinguishes a real collection version from an unknown version and from
// the same (uuid, version) under the *other* belongsTo type. Unlike
// product/component, a collection version's "owning" release deletion
// doesn't cascade to it -- collection.uuid is polymorphic (either a
// product_release or component_release uuid depending on belongs_to), so it
// can't carry a FOREIGN KEY to either, and there's no DeleteCollection at
// all -- confirmed while writing this test (DeleteComponentRelease leaves a
// component release's collections in place as orphaned rows, still
// reachable by uuid+version+belongsTo). Noted in TODO.md as a pre-existing
// gap, out of scope for this feature to fix.
func TestExistsCollectionVersion(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	component, err := r.CreateComponent(ctx, "tomcat", nil)
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	cr, err := r.CreateComponentRelease(ctx, component.UUID, ComponentReleaseInput{Version: "1.0.0", CreatedDate: time.Now()})
	if err != nil {
		t.Fatalf("CreateComponentRelease: %v", err)
	}
	c1, err := r.CreateCollectionForComponentRelease(ctx, cr.UUID, CollectionInput{})
	if err != nil {
		t.Fatalf("CreateCollectionForComponentRelease: %v", err)
	}

	if err := r.ExistsCollectionVersion(ctx, cr.UUID, c1.Version, BelongsToComponentRelease); err != nil {
		t.Fatalf("ExistsCollectionVersion: %v", err)
	}
	// A real version under the *other* belongsTo type doesn't exist there.
	if err := r.ExistsCollectionVersion(ctx, cr.UUID, c1.Version, BelongsToProductRelease); err != ErrNotFound {
		t.Fatalf("ExistsCollectionVersion (wrong belongsTo): err = %v, want ErrNotFound", err)
	}
	// A nonexistent version under the right owner/type.
	if err := r.ExistsCollectionVersion(ctx, cr.UUID, c1.Version+1, BelongsToComponentRelease); err != ErrNotFound {
		t.Fatalf("ExistsCollectionVersion (unknown version): err = %v, want ErrNotFound", err)
	}
}

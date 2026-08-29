// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"testing"
	"time"

	"github.com/oej/opentea/pkg/tea"
)

func TestCLEEventIDsPerOwnerAndOrdering(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	productA, err := r.CreateProduct(ctx, "A", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	productB, err := r.CreateProduct(ctx, "B", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	now := time.Now()
	e1, err := r.CreateCLEEvent(ctx, OwnerProduct, productA.UUID, CLEEventInput{
		Type: "released", Effective: now, Published: now, Version: "1.0.0", License: "Apache-2.0",
	})
	if err != nil {
		t.Fatalf("CreateCLEEvent: %v", err)
	}
	if e1.ID != 1 {
		t.Fatalf("e1.ID = %d, want 1", e1.ID)
	}

	e2, err := r.CreateCLEEvent(ctx, OwnerProduct, productA.UUID, CLEEventInput{
		Type: "endOfSupport", Effective: now, Published: now, SupportID: "standard",
	})
	if err != nil {
		t.Fatalf("CreateCLEEvent: %v", err)
	}
	if e2.ID != 2 {
		t.Fatalf("e2.ID = %d, want 2 (sequenced per owner)", e2.ID)
	}

	// A different owner's id sequence starts independently at 1, not 3.
	eB, err := r.CreateCLEEvent(ctx, OwnerProduct, productB.UUID, CLEEventInput{
		Type: "released", Effective: now, Published: now, Version: "1.0.0",
	})
	if err != nil {
		t.Fatalf("CreateCLEEvent (owner B): %v", err)
	}
	if eB.ID != 1 {
		t.Fatalf("eB.ID = %d, want 1 (independent per-owner sequence)", eB.ID)
	}

	cle, err := r.GetCLE(ctx, OwnerProduct, productA.UUID)
	if err != nil {
		t.Fatalf("GetCLE: %v", err)
	}
	if len(cle.Events) != 2 {
		t.Fatalf("Events = %+v, want 2", cle.Events)
	}
	// Spec requires newest-first (descending id) ordering.
	if cle.Events[0].ID != 2 || cle.Events[1].ID != 1 {
		t.Fatalf("Events order = [%d, %d], want [2, 1]", cle.Events[0].ID, cle.Events[1].ID)
	}
	if cle.Events[1].License != "Apache-2.0" {
		t.Fatalf("Events[1].License = %q", cle.Events[1].License)
	}

	cleB, err := r.GetCLE(ctx, OwnerProduct, productB.UUID)
	if err != nil {
		t.Fatalf("GetCLE (owner B): %v", err)
	}
	if len(cleB.Events) != 1 {
		t.Fatalf("owner B Events = %+v, want 1 (not contaminated by owner A)", cleB.Events)
	}
}

func TestCLESupportDefinitions(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	product, err := r.CreateProduct(ctx, "A", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	empty, err := r.GetCLE(ctx, OwnerProduct, product.UUID)
	if err != nil {
		t.Fatalf("GetCLE (no events yet): %v", err)
	}
	if len(empty.Events) != 0 || empty.Definitions != nil {
		t.Fatalf("expected empty CLE, got %+v", empty)
	}

	def := tea.CLESupportDefinition{ID: "standard", Description: "Standard product support policy", URL: "https://example.com/support/standard"}
	if _, err := r.CreateCLESupportDefinition(ctx, OwnerProduct, product.UUID, def); err != nil {
		t.Fatalf("CreateCLESupportDefinition: %v", err)
	}

	got, err := r.GetCLE(ctx, OwnerProduct, product.UUID)
	if err != nil {
		t.Fatalf("GetCLE: %v", err)
	}
	if got.Definitions == nil || len(got.Definitions.Support) != 1 || got.Definitions.Support[0].ID != "standard" {
		t.Fatalf("Definitions = %+v", got.Definitions)
	}
}

// TestCLERevisionBumpsOnEveryWrite is the regression test for CLE ETag
// support: an owner with no CLE data yet has revision 0 (not ErrNotFound,
// matching GetCLE's own "empty CLE is valid" semantics), and each of the 4
// CLE write functions bumps it by exactly 1.
func TestCLERevisionBumpsOnEveryWrite(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	product, err := r.CreateProduct(ctx, "A", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	rev, err := r.GetCLERevision(ctx, OwnerProduct, product.UUID)
	if err != nil {
		t.Fatalf("GetCLERevision (no CLE data yet): %v", err)
	}
	if rev != 0 {
		t.Fatalf("revision = %d, want 0 for an owner with no CLE data yet", rev)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := r.CreateCLEEvent(ctx, OwnerProduct, product.UUID, CLEEventInput{Type: "released", Effective: now, Published: now}); err != nil {
		t.Fatalf("CreateCLEEvent: %v", err)
	}
	rev, err = r.GetCLERevision(ctx, OwnerProduct, product.UUID)
	if err != nil {
		t.Fatalf("GetCLERevision after CreateCLEEvent: %v", err)
	}
	if rev != 1 {
		t.Fatalf("revision after CreateCLEEvent = %d, want 1", rev)
	}

	if _, err := r.ImportCLEEvent(ctx, OwnerProduct, product.UUID, tea.CLEEvent{ID: 99, Type: "released", Effective: now, Published: now}); err != nil {
		t.Fatalf("ImportCLEEvent: %v", err)
	}
	rev, err = r.GetCLERevision(ctx, OwnerProduct, product.UUID)
	if err != nil {
		t.Fatalf("GetCLERevision after ImportCLEEvent: %v", err)
	}
	if rev != 2 {
		t.Fatalf("revision after ImportCLEEvent = %d, want 2", rev)
	}

	if _, err := r.CreateCLESupportDefinition(ctx, OwnerProduct, product.UUID, tea.CLESupportDefinition{ID: "standard", Description: "x"}); err != nil {
		t.Fatalf("CreateCLESupportDefinition: %v", err)
	}
	rev, err = r.GetCLERevision(ctx, OwnerProduct, product.UUID)
	if err != nil {
		t.Fatalf("GetCLERevision after CreateCLESupportDefinition: %v", err)
	}
	if rev != 3 {
		t.Fatalf("revision after CreateCLESupportDefinition = %d, want 3", rev)
	}

	if _, err := r.ImportCLESupportDefinition(ctx, OwnerProduct, product.UUID, tea.CLESupportDefinition{ID: "lts", Description: "y"}); err != nil {
		t.Fatalf("ImportCLESupportDefinition: %v", err)
	}
	rev, err = r.GetCLERevision(ctx, OwnerProduct, product.UUID)
	if err != nil {
		t.Fatalf("GetCLERevision after ImportCLESupportDefinition: %v", err)
	}
	if rev != 4 {
		t.Fatalf("revision after ImportCLESupportDefinition = %d, want 4", rev)
	}

	// An idempotent re-import (matching content) must NOT bump.
	if _, err := r.ImportCLEEvent(ctx, OwnerProduct, product.UUID, tea.CLEEvent{ID: 99, Type: "released", Effective: now, Published: now}); err != nil {
		t.Fatalf("ImportCLEEvent (re-import): %v", err)
	}
	rev, err = r.GetCLERevision(ctx, OwnerProduct, product.UUID)
	if err != nil {
		t.Fatalf("GetCLERevision after idempotent re-import: %v", err)
	}
	if rev != 4 {
		t.Fatalf("revision after idempotent re-import = %d, want unchanged at 4", rev)
	}
}

// TestDeleteProductBumpsCLERevision is the regression test for the design-
// review-caught bug: GetCLE never checks whether its owner still exists
// (see its own doc comment -- "valid call on any existing product/release
// even before any lifecycle events are recorded" -- it just queries by
// owner_type/owner_uuid regardless), so deleting a product with CLE events
// must still bump cle_revision, or a client polling with a stale ETag would
// keep getting 304 with now-deleted event data forever instead of the
// current (now-empty) state.
func TestDeleteProductBumpsCLERevision(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	product, err := r.CreateProduct(ctx, "A", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := r.CreateCLEEvent(ctx, OwnerProduct, product.UUID, CLEEventInput{Type: "released", Effective: now, Published: now}); err != nil {
		t.Fatalf("CreateCLEEvent: %v", err)
	}
	beforeDelete, err := r.GetCLERevision(ctx, OwnerProduct, product.UUID)
	if err != nil {
		t.Fatalf("GetCLERevision before delete: %v", err)
	}

	if err := r.DeleteProduct(ctx, product.UUID); err != nil {
		t.Fatalf("DeleteProduct: %v", err)
	}

	afterDelete, err := r.GetCLERevision(ctx, OwnerProduct, product.UUID)
	if err != nil {
		t.Fatalf("GetCLERevision after delete: %v", err)
	}
	if afterDelete == beforeDelete {
		t.Fatalf("revision after DeleteProduct = %d, want different from pre-delete %d (a cached ETag must not still match)", afterDelete, beforeDelete)
	}

	// The events themselves are actually gone (deleteOwnerScoped's existing
	// behavior, unaffected by this change) -- confirms the revision bump
	// reflects a real, visible content change, not a decoy.
	cle, err := r.GetCLE(ctx, OwnerProduct, product.UUID)
	if err != nil {
		t.Fatalf("GetCLE after delete: %v", err)
	}
	if len(cle.Events) != 0 {
		t.Fatalf("Events after delete = %+v, want none", cle.Events)
	}
}

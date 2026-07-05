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

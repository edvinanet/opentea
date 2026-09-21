// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"testing"
	"time"

	"github.com/oej/opentea/pkg/tea"
)

// TestComplianceDocumentIdentifierValidComponent confirms the positive
// case actually works: a COMPLIANCE_DOCUMENT identifier with a real
// compliance-document-type value succeeds on both a Component and a
// ComponentRelease (TEA 1.0, spec/openapi.yaml's identifier-type).
func TestComplianceDocumentIdentifierValidComponent(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	c, err := r.CreateComponent(ctx, "widget-lib", []tea.Identifier{
		{IDType: tea.IdentifierTypeComplianceDocument, IDValue: tea.ComplianceDocumentTypeSOC2TypeII},
	})
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	if len(c.Identifiers) != 1 || c.Identifiers[0].IDValue != tea.ComplianceDocumentTypeSOC2TypeII {
		t.Fatalf("c.Identifiers = %+v", c.Identifiers)
	}

	cr, err := r.CreateComponentRelease(ctx, c.UUID, ComponentReleaseInput{
		Version:     "1.0.0",
		CreatedDate: time.Now(),
		Identifiers: []tea.Identifier{{IDType: tea.IdentifierTypeComplianceDocument, IDValue: tea.ComplianceDocumentTypeISO27001}},
	})
	if err != nil {
		t.Fatalf("CreateComponentRelease: %v", err)
	}
	if len(cr.Identifiers) != 1 || cr.Identifiers[0].IDValue != tea.ComplianceDocumentTypeISO27001 {
		t.Fatalf("cr.Identifiers = %+v", cr.Identifiers)
	}
}

// TestComplianceDocumentIdentifierWrongOwner proves the TEA 1.0 scoping
// rule ("It shall not be used on products, product releases,
// distributions, or CLE events") is actually enforced, not just recorded
// in a comment -- covers every owner type it names, on both the
// Create and Import paths since they don't share a call site.
func TestComplianceDocumentIdentifierWrongOwner(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	badIdentifier := []tea.Identifier{{IDType: tea.IdentifierTypeComplianceDocument, IDValue: tea.ComplianceDocumentTypeGDPR}}

	if _, err := r.CreateProduct(ctx, "should-fail", badIdentifier); err != ErrComplianceDocumentWrongOwner {
		t.Fatalf("CreateProduct: err = %v, want ErrComplianceDocumentWrongOwner", err)
	}

	product, err := r.CreateProduct(ctx, "P", nil)
	if err != nil {
		t.Fatalf("CreateProduct (control): %v", err)
	}
	if _, err := r.CreateProductRelease(ctx, product.UUID, ProductReleaseInput{
		Version: "1.0", CreatedDate: time.Now(), Identifiers: badIdentifier,
	}); err != ErrComplianceDocumentWrongOwner {
		t.Fatalf("CreateProductRelease: err = %v, want ErrComplianceDocumentWrongOwner", err)
	}

	if _, err := r.ImportProduct(ctx, "00000000-0000-4000-8000-000000000001", "imported-product", badIdentifier); err != ErrComplianceDocumentWrongOwner {
		t.Fatalf("ImportProduct: err = %v, want ErrComplianceDocumentWrongOwner", err)
	}

	componentForRelease, err := r.CreateComponent(ctx, "C", nil)
	if err != nil {
		t.Fatalf("CreateComponent (control): %v", err)
	}
	release, err := r.CreateComponentRelease(ctx, componentForRelease.UUID, ComponentReleaseInput{Version: "1.0", CreatedDate: time.Now()})
	if err != nil {
		t.Fatalf("CreateComponentRelease (control): %v", err)
	}
	if _, err := r.ImportDistribution(ctx, ImportDistributionInput{
		DistributionID: "00000000-0000-4000-8000-000000000002", ComponentReleaseUUID: release.UUID,
		Description: "x", Identifiers: badIdentifier,
	}); err != ErrComplianceDocumentWrongOwner {
		t.Fatalf("ImportDistribution: err = %v, want ErrComplianceDocumentWrongOwner", err)
	}
}

// TestComplianceDocumentIdentifierInvalidValue proves idValue is checked
// against the spec's compliance-document-type enum regardless of which
// (otherwise-allowed) owner type it's attached to.
func TestComplianceDocumentIdentifierInvalidValue(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	invalid := []tea.Identifier{{IDType: tea.IdentifierTypeComplianceDocument, IDValue: "NOT_A_REAL_TYPE"}}
	if _, err := r.CreateComponent(ctx, "should-fail", invalid); err != ErrInvalidComplianceDocumentType {
		t.Fatalf("CreateComponent: err = %v, want ErrInvalidComplianceDocumentType", err)
	}

	component, err := r.CreateComponent(ctx, "C", nil)
	if err != nil {
		t.Fatalf("CreateComponent (control): %v", err)
	}
	if _, err := r.CreateComponentRelease(ctx, component.UUID, ComponentReleaseInput{
		Version: "1.0", CreatedDate: time.Now(), Identifiers: invalid,
	}); err != ErrInvalidComplianceDocumentType {
		t.Fatalf("CreateComponentRelease: err = %v, want ErrInvalidComplianceDocumentType", err)
	}
}

// TestComplianceDocumentIdentifierForbiddenOnCLEEvent proves the
// unconditional (no owner-type carve-out) exclusion for CLE events --
// unlike products/releases/distributions, a CLE event on a Component
// still can't carry a COMPLIANCE_DOCUMENT identifier, because the rule
// names "CLE events" directly, not by owner type.
func TestComplianceDocumentIdentifierForbiddenOnCLEEvent(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	component, err := r.CreateComponent(ctx, "C", nil)
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	now := time.Now()
	badIdentifier := []tea.Identifier{{IDType: tea.IdentifierTypeComplianceDocument, IDValue: tea.ComplianceDocumentTypeHIPAA}}

	if _, err := r.CreateCLEEvent(ctx, OwnerComponent, component.UUID, CLEEventInput{
		Type: "released", Effective: now, Published: now, Identifiers: badIdentifier,
	}); err != ErrComplianceDocumentWrongOwner {
		t.Fatalf("CreateCLEEvent (component owner): err = %v, want ErrComplianceDocumentWrongOwner", err)
	}

	if _, err := r.ImportCLEEvent(ctx, OwnerComponent, component.UUID, tea.CLEEvent{
		ID: 1, Type: "released", Effective: now, Published: now, Identifiers: badIdentifier,
	}); err != ErrComplianceDocumentWrongOwner {
		t.Fatalf("ImportCLEEvent: err = %v, want ErrComplianceDocumentWrongOwner", err)
	}
}

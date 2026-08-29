// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oej/opentea/internal/authz"
	"github.com/oej/opentea/internal/model"
)

func TestCreateAndGetEntitlement(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	tmpl, rev, err := r.CreateTemplate(ctx, "Public", "", []model.TemplateCapabilityRule{
		{Capability: authz.CapProductRead, Decision: model.DecisionAllow},
	}, "", "")
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	if err := r.ActivateTemplateRevision(ctx, tmpl.UUID, rev.Revision); err != nil {
		t.Fatalf("ActivateTemplateRevision: %v", err)
	}

	admin, err := r.CreateUser(ctx, "admin-1", "password123", model.RoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	created, err := r.CreateEntitlement(ctx, EntitlementInput{
		SubjectType:       authz.SubjectEveryone,
		TemplateUUID:      tmpl.UUID,
		TemplateRevision:  rev.Revision,
		ResourceType:      model.ResourceAllProducts,
		GrantingAuthority: "test",
	}, admin.UUID)
	if err != nil {
		t.Fatalf("CreateEntitlement: %v", err)
	}
	if created.Status != model.EntitlementActive {
		t.Errorf("Status = %q, want %q", created.Status, model.EntitlementActive)
	}
	if created.Revision != 1 {
		t.Errorf("Revision = %d, want 1", created.Revision)
	}

	got, err := r.GetEntitlement(ctx, created.UUID)
	if err != nil {
		t.Fatalf("GetEntitlement: %v", err)
	}
	if got.GrantingAuthority != "test" || got.CreatedBy != admin.UUID {
		t.Errorf("got = %+v", got)
	}
}

func TestGetEntitlementNotFound(t *testing.T) {
	r := newTestRepo(t)
	_, err := r.GetEntitlement(context.Background(), "00000000-0000-4000-8000-000000000000")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestUpdateEntitlementStatusBumpsRevision(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	tmpl, rev, err := r.CreateTemplate(ctx, "Public", "", nil, "", "")
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	created, err := r.CreateEntitlement(ctx, EntitlementInput{
		SubjectType:       authz.SubjectEveryone,
		TemplateUUID:      tmpl.UUID,
		TemplateRevision:  rev.Revision,
		ResourceType:      model.ResourceAllProducts,
		GrantingAuthority: "test",
	}, "")
	if err != nil {
		t.Fatalf("CreateEntitlement: %v", err)
	}

	updated, err := r.UpdateEntitlementStatus(ctx, created.UUID, model.EntitlementSuspended)
	if err != nil {
		t.Fatalf("UpdateEntitlementStatus: %v", err)
	}
	if updated.Status != model.EntitlementSuspended {
		t.Errorf("Status = %q, want %q", updated.Status, model.EntitlementSuspended)
	}
	if updated.Revision != 2 {
		t.Errorf("Revision = %d, want 2", updated.Revision)
	}
}

func TestListEntitlementsFilter(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	tmpl, rev, err := r.CreateTemplate(ctx, "Public", "", nil, "", "")
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	product, err := r.CreateProduct(ctx, "P", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	ent := createEntitlement(t, r, authz.SubjectEveryone, "", model.ResourceProduct, product.UUID, tmpl.UUID, rev.Revision)

	filtered, err := r.ListEntitlements(ctx, EntitlementFilter{ResourceType: model.ResourceProduct, ResourceID: product.UUID})
	if err != nil {
		t.Fatalf("ListEntitlements: %v", err)
	}
	if len(filtered) != 1 || filtered[0].UUID != ent.UUID {
		t.Fatalf("filtered = %+v, want exactly [%s]", filtered, ent.UUID)
	}

	unfiltered, err := r.ListEntitlements(ctx, EntitlementFilter{})
	if err != nil {
		t.Fatalf("ListEntitlements: %v", err)
	}
	// The migration-seeded bootstrap entitlement is always present too.
	if len(unfiltered) < 2 {
		t.Fatalf("unfiltered = %+v, want at least 2 (bootstrap + created)", unfiltered)
	}
}

func TestDeleteEntitlement(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	tmpl, rev, err := r.CreateTemplate(ctx, "Public", "", nil, "", "")
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	created, err := r.CreateEntitlement(ctx, EntitlementInput{
		SubjectType:       authz.SubjectEveryone,
		TemplateUUID:      tmpl.UUID,
		TemplateRevision:  rev.Revision,
		ResourceType:      model.ResourceAllProducts,
		GrantingAuthority: "test",
	}, "")
	if err != nil {
		t.Fatalf("CreateEntitlement: %v", err)
	}

	if err := r.DeleteEntitlement(ctx, created.UUID); err != nil {
		t.Fatalf("DeleteEntitlement: %v", err)
	}
	if _, err := r.GetEntitlement(ctx, created.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetEntitlement after delete = %v, want ErrNotFound", err)
	}
	if err := r.DeleteEntitlement(ctx, created.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteEntitlement (again) = %v, want ErrNotFound", err)
	}
}

func TestGetPrincipalEntitlementWatermark(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	user, err := r.CreateUser(ctx, "alice", "password123", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Every fresh DB carries the migration-seeded bootstrap entitlement
	// (revision 1, subject 'everyone'), which already applies to any
	// principal -- so the baseline here is 1, not 0.
	before, err := r.GetPrincipalEntitlementWatermark(ctx, user.UUID)
	if err != nil {
		t.Fatalf("GetPrincipalEntitlementWatermark: %v", err)
	}
	if before != 1 {
		t.Fatalf("baseline watermark = %d, want 1 (bootstrap entitlement)", before)
	}

	tmpl, rev, err := r.CreateTemplate(ctx, "VIP", "", nil, "", "")
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	ent, err := r.CreateEntitlement(ctx, EntitlementInput{
		SubjectType:       authz.SubjectPrincipal,
		SubjectID:         user.UUID,
		TemplateUUID:      tmpl.UUID,
		TemplateRevision:  rev.Revision,
		ResourceType:      model.ResourceAllProducts,
		GrantingAuthority: "test",
	}, "")
	if err != nil {
		t.Fatalf("CreateEntitlement: %v", err)
	}

	// The new entitlement also starts at revision 1, so the max is
	// unchanged until it's modified -- bump it while keeping it active
	// (UpdateEntitlementValidity doesn't touch status) and confirm the
	// principal watermark tracks it.
	future := time.Now().Add(24 * time.Hour)
	if _, err := r.UpdateEntitlementValidity(ctx, ent.UUID, nil, &future); err != nil {
		t.Fatalf("UpdateEntitlementValidity: %v", err)
	}
	after, err := r.GetPrincipalEntitlementWatermark(ctx, user.UUID)
	if err != nil {
		t.Fatalf("GetPrincipalEntitlementWatermark: %v", err)
	}
	if after != 2 {
		t.Fatalf("watermark after bumping the principal's own entitlement = %d, want 2", after)
	}

	// A different, unrelated user must not be affected by that principal-
	// scoped entitlement at all -- only the bootstrap baseline applies.
	otherUser, err := r.CreateUser(ctx, "bob", "password123", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	otherWatermark, err := r.GetPrincipalEntitlementWatermark(ctx, otherUser.UUID)
	if err != nil {
		t.Fatalf("GetPrincipalEntitlementWatermark: %v", err)
	}
	if otherWatermark != 1 {
		t.Fatalf("unrelated user's watermark = %d, want 1 (bootstrap only)", otherWatermark)
	}
}

// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"errors"
	"testing"

	"github.com/oej/opentea/internal/authz"
	"github.com/oej/opentea/internal/model"
)

func TestCreateTemplateStartsWithNoActiveRevision(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	tmpl, rev, err := r.CreateTemplate(ctx, "VEX Access", "grants VEX only", []model.TemplateCapabilityRule{
		{Capability: authz.CapArtifactDiscover, ArtifactType: "BOM", Decision: model.DecisionAllow},
	}, "", "initial")
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	if rev.Revision != 1 {
		t.Fatalf("Revision = %d, want 1", rev.Revision)
	}
	if tmpl.ActiveRevision != 0 {
		t.Fatalf("ActiveRevision = %d, want 0 (not yet activated)", tmpl.ActiveRevision)
	}

	got, err := r.GetTemplateRevision(ctx, tmpl.UUID, 1)
	if err != nil {
		t.Fatalf("GetTemplateRevision: %v", err)
	}
	if len(got.Rules) != 1 || got.Rules[0].Capability != authz.CapArtifactDiscover {
		t.Fatalf("Rules = %+v", got.Rules)
	}
}

func TestActivateTemplateRevision(t *testing.T) {
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
	got, err := r.GetTemplate(ctx, tmpl.UUID)
	if err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	if got.ActiveRevision != 1 {
		t.Fatalf("ActiveRevision = %d, want 1", got.ActiveRevision)
	}

	if err := r.ActivateTemplateRevision(ctx, tmpl.UUID, 99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ActivateTemplateRevision(nonexistent revision) = %v, want ErrNotFound", err)
	}
}

func TestCreateTemplateRevisionIncrements(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	tmpl, _, err := r.CreateTemplate(ctx, "Evolving", "", nil, "", "")
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	rev2, err := r.CreateTemplateRevision(ctx, tmpl.UUID, []model.TemplateCapabilityRule{
		{Capability: authz.CapProductRead, Decision: model.DecisionAllow},
	}, "", "widen access")
	if err != nil {
		t.Fatalf("CreateTemplateRevision: %v", err)
	}
	if rev2.Revision != 2 {
		t.Fatalf("Revision = %d, want 2", rev2.Revision)
	}

	// The template's active revision is untouched by creating a new one --
	// activation is a separate, explicit step (spec Sec 13.1).
	got, err := r.GetTemplate(ctx, tmpl.UUID)
	if err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	if got.ActiveRevision != 0 {
		t.Fatalf("ActiveRevision = %d, want 0 (unaffected by CreateTemplateRevision)", got.ActiveRevision)
	}
}

func TestDeleteTemplateInUse(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	tmpl, rev, err := r.CreateTemplate(ctx, "In Use", "", []model.TemplateCapabilityRule{
		{Capability: authz.CapProductRead, Decision: model.DecisionAllow},
	}, "", "")
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	if err := r.ActivateTemplateRevision(ctx, tmpl.UUID, rev.Revision); err != nil {
		t.Fatalf("ActivateTemplateRevision: %v", err)
	}
	if _, err := r.CreateEntitlement(ctx, EntitlementInput{
		SubjectType:       authz.SubjectEveryone,
		TemplateUUID:      tmpl.UUID,
		TemplateRevision:  rev.Revision,
		ResourceType:      model.ResourceAllProducts,
		GrantingAuthority: "test",
	}, ""); err != nil {
		t.Fatalf("CreateEntitlement: %v", err)
	}

	if err := r.DeleteTemplate(ctx, tmpl.UUID); !errors.Is(err, ErrTemplateInUse) {
		t.Fatalf("DeleteTemplate = %v, want ErrTemplateInUse", err)
	}
}

func TestDeleteTemplateNotFound(t *testing.T) {
	r := newTestRepo(t)
	if err := r.DeleteTemplate(context.Background(), "00000000-0000-4000-8000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteTemplate(nonexistent) = %v, want ErrNotFound", err)
	}
}

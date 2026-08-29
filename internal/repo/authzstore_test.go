// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"testing"
	"time"

	"github.com/oej/opentea/internal/authz"
	"github.com/oej/opentea/internal/idgen"
	"github.com/oej/opentea/internal/model"
)

// createActiveTemplate creates a template with one revision granting rules,
// activates that revision, and returns the template UUID and revision
// number -- test helper shared across this file.
func createActiveTemplate(t *testing.T, r *Repo, rules []model.TemplateCapabilityRule) (string, int) {
	t.Helper()
	ctx := context.Background()
	tmpl, rev, err := r.CreateTemplate(ctx, t.Name()+"-template-"+idgen.New(), "", rules, "", "")
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	if err := r.ActivateTemplateRevision(ctx, tmpl.UUID, rev.Revision); err != nil {
		t.Fatalf("ActivateTemplateRevision: %v", err)
	}
	return tmpl.UUID, rev.Revision
}

func createEntitlement(t *testing.T, r *Repo, subjectType authz.SubjectType, subjectID, resourceType, resourceID, templateUUID string, templateRevision int) model.Entitlement {
	t.Helper()
	e, err := r.CreateEntitlement(context.Background(), EntitlementInput{
		SubjectType:       subjectType,
		SubjectID:         subjectID,
		TemplateUUID:      templateUUID,
		TemplateRevision:  templateRevision,
		ResourceType:      resourceType,
		ResourceID:        resourceID,
		GrantingAuthority: "test",
	}, "")
	if err != nil {
		t.Fatalf("CreateEntitlement: %v", err)
	}
	return e
}

// containsEntitlement reports whether candidates includes one with the
// given EntitlementUUID.
func containsEntitlement(candidates []authz.CandidateRule, entitlementUUID string) bool {
	for _, c := range candidates {
		if c.EntitlementUUID == entitlementUUID {
			return true
		}
	}
	return false
}

func TestCandidateRulesProductGroupExpansion(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	product, err := r.CreateProduct(ctx, "Product A", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	group, err := r.CreateProductGroup(ctx, "Group A", "")
	if err != nil {
		t.Fatalf("CreateProductGroup: %v", err)
	}
	if err := r.AddProductGroupMember(ctx, group.UUID, product.UUID); err != nil {
		t.Fatalf("AddProductGroupMember: %v", err)
	}

	tmplUUID, rev := createActiveTemplate(t, r, []model.TemplateCapabilityRule{
		{Capability: authz.CapProductRead, Decision: model.DecisionAllow},
	})
	ent := createEntitlement(t, r, authz.SubjectEveryone, "", model.ResourceProductGroup, group.UUID, tmplUUID, rev)

	candidates, err := r.CandidateRules(ctx, authz.Principal{}, authz.CapProductRead, authz.Resource{ProductUUID: product.UUID})
	if err != nil {
		t.Fatalf("CandidateRules: %v", err)
	}
	if !containsEntitlement(candidates, ent.UUID) {
		t.Errorf("candidates = %+v, want to contain product-group entitlement %s", candidates, ent.UUID)
	}
	for _, c := range candidates {
		if c.EntitlementUUID == ent.UUID && c.ResourceScope != authz.ScopeProductGroup {
			t.Errorf("product-group entitlement's ResourceScope = %v, want ScopeProductGroup", c.ResourceScope)
		}
	}

	// A different, non-member product must NOT see this entitlement.
	other, err := r.CreateProduct(ctx, "Product B", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	candidates, err = r.CandidateRules(ctx, authz.Principal{}, authz.CapProductRead, authz.Resource{ProductUUID: other.UUID})
	if err != nil {
		t.Fatalf("CandidateRules: %v", err)
	}
	if containsEntitlement(candidates, ent.UUID) {
		t.Errorf("non-member product must not see group entitlement %s, got %+v", ent.UUID, candidates)
	}
}

func TestCandidateRulesReleaseGroupExpansion(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	product, err := r.CreateProduct(ctx, "Product A", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	release, err := r.CreateProductRelease(ctx, product.UUID, ProductReleaseInput{Version: "1.0", CreatedDate: time.Now()})
	if err != nil {
		t.Fatalf("CreateProductRelease: %v", err)
	}
	group, err := r.CreateReleaseGroup(ctx, "LTS", "")
	if err != nil {
		t.Fatalf("CreateReleaseGroup: %v", err)
	}
	if err := r.AddReleaseGroupMember(ctx, group.UUID, release.UUID); err != nil {
		t.Fatalf("AddReleaseGroupMember: %v", err)
	}

	tmplUUID, rev := createActiveTemplate(t, r, []model.TemplateCapabilityRule{
		{Capability: authz.CapReleaseRead, Decision: model.DecisionAllow},
	})
	ent := createEntitlement(t, r, authz.SubjectEveryone, "", model.ResourceReleaseGroup, group.UUID, tmplUUID, rev)

	candidates, err := r.CandidateRules(ctx, authz.Principal{}, authz.CapReleaseRead, authz.Resource{ProductReleaseUUID: release.UUID})
	if err != nil {
		t.Fatalf("CandidateRules: %v", err)
	}
	if !containsEntitlement(candidates, ent.UUID) {
		t.Errorf("candidates = %+v, want to contain release-group entitlement %s", candidates, ent.UUID)
	}
}

func TestCandidateRulesCollectionIndependentOfRelease(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	product, err := r.CreateProduct(ctx, "Product A", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	release, err := r.CreateProductRelease(ctx, product.UUID, ProductReleaseInput{Version: "1.0", CreatedDate: time.Now()})
	if err != nil {
		t.Fatalf("CreateProductRelease: %v", err)
	}
	collection, err := r.CreateCollectionForProductRelease(ctx, release.UUID, CollectionInput{})
	if err != nil {
		t.Fatalf("CreateCollectionForProductRelease: %v", err)
	}

	// A release-scoped entitlement whose template grants release.read but
	// NOT collection.download must not produce a candidate for
	// collection.download (spec Sec 12.2: release access doesn't imply
	// collection access).
	releaseOnlyTemplate, rev := createActiveTemplate(t, r, []model.TemplateCapabilityRule{
		{Capability: authz.CapReleaseRead, Decision: model.DecisionAllow},
	})
	releaseOnlyEnt := createEntitlement(t, r, authz.SubjectEveryone, "", model.ResourceProductRelease, release.UUID, releaseOnlyTemplate, rev)

	candidates, err := r.CandidateRules(ctx, authz.Principal{}, authz.CapCollectionDownload, authz.Resource{CollectionUUID: collection.UUID})
	if err != nil {
		t.Fatalf("CandidateRules: %v", err)
	}
	if containsEntitlement(candidates, releaseOnlyEnt.UUID) {
		t.Errorf("release-only entitlement %s must not grant collection.download, got %+v", releaseOnlyEnt.UUID, candidates)
	}

	// A release-scoped entitlement whose template DOES grant
	// collection.download must produce a candidate at ScopeProductRelease
	// (the collection's identity equals its owning release's, but the
	// matching entitlement here is scoped at the release, not the
	// collection, level).
	fullTemplate, rev2 := createActiveTemplate(t, r, []model.TemplateCapabilityRule{
		{Capability: authz.CapCollectionDownload, Decision: model.DecisionAllow},
	})
	fullEnt := createEntitlement(t, r, authz.SubjectEveryone, "", model.ResourceProductRelease, release.UUID, fullTemplate, rev2)

	candidates, err = r.CandidateRules(ctx, authz.Principal{}, authz.CapCollectionDownload, authz.Resource{CollectionUUID: collection.UUID})
	if err != nil {
		t.Fatalf("CandidateRules: %v", err)
	}
	if !containsEntitlement(candidates, fullEnt.UUID) {
		t.Errorf("release entitlement granting collection.download must apply to its collection, got %+v", candidates)
	}

	// And a direct collection-scoped entitlement must also apply.
	directTemplate, rev3 := createActiveTemplate(t, r, []model.TemplateCapabilityRule{
		{Capability: authz.CapCollectionDownload, Decision: model.DecisionAllow},
	})
	directEnt := createEntitlement(t, r, authz.SubjectEveryone, "", model.ResourceCollection, collection.UUID, directTemplate, rev3)

	candidates, err = r.CandidateRules(ctx, authz.Principal{}, authz.CapCollectionDownload, authz.Resource{CollectionUUID: collection.UUID})
	if err != nil {
		t.Fatalf("CandidateRules: %v", err)
	}
	if !containsEntitlement(candidates, directEnt.UUID) {
		t.Errorf("direct collection entitlement must apply, got %+v", candidates)
	}
}

func TestCandidateRulesExpiredAndInactiveExcluded(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	product, err := r.CreateProduct(ctx, "Product A", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	tmplUUID, rev := createActiveTemplate(t, r, []model.TemplateCapabilityRule{
		{Capability: authz.CapProductRead, Decision: model.DecisionAllow},
	})

	expired := createEntitlement(t, r, authz.SubjectEveryone, "", model.ResourceProduct, product.UUID, tmplUUID, rev)
	past := time.Now().Add(-24 * time.Hour)
	if _, err := r.UpdateEntitlementValidity(ctx, expired.UUID, nil, &past); err != nil {
		t.Fatalf("UpdateEntitlementValidity: %v", err)
	}

	suspended := createEntitlement(t, r, authz.SubjectEveryone, "", model.ResourceProduct, product.UUID, tmplUUID, rev)
	if _, err := r.UpdateEntitlementStatus(ctx, suspended.UUID, model.EntitlementSuspended); err != nil {
		t.Fatalf("UpdateEntitlementStatus: %v", err)
	}

	active := createEntitlement(t, r, authz.SubjectEveryone, "", model.ResourceProduct, product.UUID, tmplUUID, rev)

	candidates, err := r.CandidateRules(ctx, authz.Principal{}, authz.CapProductRead, authz.Resource{ProductUUID: product.UUID})
	if err != nil {
		t.Fatalf("CandidateRules: %v", err)
	}
	if containsEntitlement(candidates, expired.UUID) {
		t.Errorf("expired entitlement must be excluded, got %+v", candidates)
	}
	if containsEntitlement(candidates, suspended.UUID) {
		t.Errorf("suspended entitlement must be excluded, got %+v", candidates)
	}
	if !containsEntitlement(candidates, active.UUID) {
		t.Errorf("active entitlement must be included, got %+v", candidates)
	}
}

func TestCandidateRulesAnonymousOnlySeesEveryone(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	product, err := r.CreateProduct(ctx, "Product A", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	user, err := r.CreateUser(ctx, "alice", "password123", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	tmplUUID, rev := createActiveTemplate(t, r, []model.TemplateCapabilityRule{
		{Capability: authz.CapProductRead, Decision: model.DecisionAllow},
	})

	authEnt := createEntitlement(t, r, authz.SubjectAuthenticated, "", model.ResourceProduct, product.UUID, tmplUUID, rev)
	principalEnt := createEntitlement(t, r, authz.SubjectPrincipal, user.UUID, model.ResourceProduct, product.UUID, tmplUUID, rev)

	anonCandidates, err := r.CandidateRules(ctx, authz.Principal{}, authz.CapProductRead, authz.Resource{ProductUUID: product.UUID})
	if err != nil {
		t.Fatalf("CandidateRules (anonymous): %v", err)
	}
	if containsEntitlement(anonCandidates, authEnt.UUID) {
		t.Errorf("anonymous caller must not see an 'authenticated'-scoped entitlement, got %+v", anonCandidates)
	}
	if containsEntitlement(anonCandidates, principalEnt.UUID) {
		t.Errorf("anonymous caller must not see a 'principal'-scoped entitlement, got %+v", anonCandidates)
	}

	authCandidates, err := r.CandidateRules(ctx, authz.Principal{UserUUID: user.UUID}, authz.CapProductRead, authz.Resource{ProductUUID: product.UUID})
	if err != nil {
		t.Fatalf("CandidateRules (authenticated): %v", err)
	}
	if !containsEntitlement(authCandidates, authEnt.UUID) {
		t.Errorf("authenticated caller must see the 'authenticated'-scoped entitlement, got %+v", authCandidates)
	}
	if !containsEntitlement(authCandidates, principalEnt.UUID) {
		t.Errorf("matching principal must see its own entitlement, got %+v", authCandidates)
	}

	otherUser, err := r.CreateUser(ctx, "bob", "password123", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	otherCandidates, err := r.CandidateRules(ctx, authz.Principal{UserUUID: otherUser.UUID}, authz.CapProductRead, authz.Resource{ProductUUID: product.UUID})
	if err != nil {
		t.Fatalf("CandidateRules (other principal): %v", err)
	}
	if containsEntitlement(otherCandidates, principalEnt.UUID) {
		t.Errorf("a different principal must not see someone else's principal-scoped entitlement, got %+v", otherCandidates)
	}
}

func TestCandidateRulesArtifactSharedAcrossCollections(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	product, err := r.CreateProduct(ctx, "Product A", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	release, err := r.CreateProductRelease(ctx, product.UUID, ProductReleaseInput{Version: "1.0", CreatedDate: time.Now()})
	if err != nil {
		t.Fatalf("CreateProductRelease: %v", err)
	}
	artifact, err := r.CreateArtifact(ctx, ArtifactInput{Name: "sbom.json", Type: "BOM"})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	if _, err := r.CreateCollectionForProductRelease(ctx, release.UUID, CollectionInput{
		Artifacts: []ArtifactRef{{UUID: artifact.UUID, Version: 1}},
	}); err != nil {
		t.Fatalf("CreateCollectionForProductRelease: %v", err)
	}

	tmplUUID, rev := createActiveTemplate(t, r, []model.TemplateCapabilityRule{
		{Capability: authz.CapArtifactDownload, Decision: model.DecisionAllow},
	})
	ent := createEntitlement(t, r, authz.SubjectEveryone, "", model.ResourceProductRelease, release.UUID, tmplUUID, rev)

	// Direct fetch with no collection context (as getArtifactByVersion/
	// getLatestArtifact would call it) must still discover the reachable
	// release-scoped entitlement via the artifact's collection membership.
	candidates, err := r.CandidateRules(ctx, authz.Principal{}, authz.CapArtifactDownload, authz.Resource{ArtifactUUID: artifact.UUID, ArtifactType: "BOM"})
	if err != nil {
		t.Fatalf("CandidateRules: %v", err)
	}
	if !containsEntitlement(candidates, ent.UUID) {
		t.Errorf("artifact reachable via an authorized collection relationship must see that entitlement, got %+v", candidates)
	}
}

func TestCandidateRulesBootstrapEntitlementApplies(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	product, err := r.CreateProduct(ctx, "Product A", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	// Every fresh DB carries the migration-seeded bootstrap entitlement
	// (see internal/db/migrations/0005_authz.sql) -- an anonymous caller
	// should see it as an all_products candidate for any capability it grants.
	candidates, err := r.CandidateRules(ctx, authz.Principal{}, authz.CapProductRead, authz.Resource{ProductUUID: product.UUID})
	if err != nil {
		t.Fatalf("CandidateRules: %v", err)
	}
	found := false
	for _, c := range candidates {
		if c.ResourceScope == authz.ScopeAllProducts && c.Decision == "allow" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the bootstrap all_products allow candidate, got %+v", candidates)
	}
}

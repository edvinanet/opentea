package authz

import (
	"context"
	"errors"
	"testing"
)

// fakeStore returns a fixed set of candidates (or a fixed error), letting
// Decide's ranking/deny-wins/default-deny logic be tested without any
// database involved -- these are exactly the conformance checks spec
// Sec 26.1 requires a conforming implementation to pass.
type fakeStore struct {
	candidates []CandidateRule
	err        error

	// recorded call args, for the pass-through test below.
	gotSubject    Principal
	gotCapability Capability
	gotResource   Resource
}

func (f *fakeStore) CandidateRules(_ context.Context, subject Principal, capability Capability, resource Resource) ([]CandidateRule, error) {
	f.gotSubject = subject
	f.gotCapability = capability
	f.gotResource = resource
	return f.candidates, f.err
}

func TestDecide(t *testing.T) {
	tests := []struct {
		name            string
		candidates      []CandidateRule
		wantAllowed     bool
		wantReason      ReasonCode
		wantEntitlement string
	}{
		{
			name:            "default deny on no candidates",
			candidates:      nil,
			wantAllowed:     false,
			wantReason:      ReasonNoApplicableRule,
			wantEntitlement: "",
		},
		{
			name: "single allow",
			candidates: []CandidateRule{
				{EntitlementUUID: "e1", ResourceScope: ScopeProduct, SubjectType: SubjectEveryone, Decision: "allow"},
			},
			wantAllowed:     true,
			wantReason:      ReasonExplicitAllow,
			wantEntitlement: "e1",
		},
		{
			name: "conflicting equal-specificity rules resolve to deny",
			candidates: []CandidateRule{
				{EntitlementUUID: "e1", ResourceScope: ScopeProduct, SubjectType: SubjectEveryone, Decision: "allow"},
				{EntitlementUUID: "e2", ResourceScope: ScopeProduct, SubjectType: SubjectEveryone, Decision: "deny"},
			},
			wantAllowed:     false,
			wantReason:      ReasonExplicitDeny,
			wantEntitlement: "e2",
		},
		{
			name: "narrower resource restriction defeats broader grant",
			candidates: []CandidateRule{
				{EntitlementUUID: "e1", ResourceScope: ScopeAllProducts, SubjectType: SubjectEveryone, Decision: "allow"},
				{EntitlementUUID: "e2", ResourceScope: ScopeProduct, SubjectType: SubjectEveryone, Decision: "deny"},
			},
			wantAllowed:     false,
			wantReason:      ReasonExplicitDeny,
			wantEntitlement: "e2",
		},
		{
			name: "narrower resource restriction defeats broader allow even from a more specific subject",
			candidates: []CandidateRule{
				{EntitlementUUID: "e1", ResourceScope: ScopeAllProducts, SubjectType: SubjectPrincipal, Decision: "allow"},
				{EntitlementUUID: "e2", ResourceScope: ScopeProduct, SubjectType: SubjectEveryone, Decision: "deny"},
			},
			wantAllowed:     false,
			wantReason:      ReasonExplicitDeny,
			wantEntitlement: "e2",
		},
		{
			name: "subject specificity decides within equal resource specificity",
			candidates: []CandidateRule{
				{EntitlementUUID: "e1", ResourceScope: ScopeProduct, SubjectType: SubjectEveryone, Decision: "deny"},
				{EntitlementUUID: "e2", ResourceScope: ScopeProduct, SubjectType: SubjectPrincipal, Decision: "allow"},
			},
			wantAllowed:     true,
			wantReason:      ReasonExplicitAllow,
			wantEntitlement: "e2",
		},
		{
			name: "authenticated deny beats everyone allow at equal resource scope",
			candidates: []CandidateRule{
				{EntitlementUUID: "e1", ResourceScope: ScopeProduct, SubjectType: SubjectEveryone, Decision: "allow"},
				{EntitlementUUID: "e2", ResourceScope: ScopeProduct, SubjectType: SubjectAuthenticated, Decision: "deny"},
			},
			wantAllowed:     false,
			wantReason:      ReasonExplicitDeny,
			wantEntitlement: "e2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{candidates: tt.candidates}
			got, err := Decide(context.Background(), store, Principal{}, CapProductRead, Resource{ProductUUID: "p1"})
			if err != nil {
				t.Fatalf("Decide: %v", err)
			}
			if got.Allowed != tt.wantAllowed {
				t.Errorf("Allowed = %v, want %v", got.Allowed, tt.wantAllowed)
			}
			if got.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", got.Reason, tt.wantReason)
			}
			if got.MatchedEntitlementUUID != tt.wantEntitlement {
				t.Errorf("MatchedEntitlementUUID = %q, want %q", got.MatchedEntitlementUUID, tt.wantEntitlement)
			}
		})
	}
}

func TestDecidePropagatesStoreError(t *testing.T) {
	wantErr := errors.New("store unavailable")
	store := &fakeStore{err: wantErr}
	_, err := Decide(context.Background(), store, Principal{}, CapProductRead, Resource{ProductUUID: "p1"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Decide error = %v, want %v", err, wantErr)
	}
}

func TestDecidePassesArgumentsToStoreUnchanged(t *testing.T) {
	store := &fakeStore{candidates: []CandidateRule{
		{EntitlementUUID: "e1", ResourceScope: ScopeArtifact, SubjectType: SubjectEveryone, Decision: "allow"},
	}}
	subject := Principal{UserUUID: "user-123"}
	resource := Resource{ArtifactUUID: "art-1", ArtifactType: "BOM"}

	if _, err := Decide(context.Background(), store, subject, CapArtifactDownload, resource); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if store.gotSubject != subject {
		t.Errorf("gotSubject = %+v, want %+v", store.gotSubject, subject)
	}
	if store.gotCapability != CapArtifactDownload {
		t.Errorf("gotCapability = %q, want %q", store.gotCapability, CapArtifactDownload)
	}
	if store.gotResource != resource {
		t.Errorf("gotResource = %+v, want %+v", store.gotResource, resource)
	}
}

func TestPrincipalIsAuthenticated(t *testing.T) {
	if (Principal{}).IsAuthenticated() {
		t.Error("zero-value Principal must be anonymous")
	}
	if !(Principal{UserUUID: "u1"}).IsAuthenticated() {
		t.Error("Principal with a UserUUID must be authenticated")
	}
}

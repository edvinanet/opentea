// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Package authz implements the deterministic policy-evaluation algorithm
// from ~/TEA_AUTHENTICATION_AUTHORIZATION_SPECIFICATION.md Sec 15, scoped to
// opentea's single-tenant profile: everyone/authenticated/principal
// subjects, template-based capability rules, and the resource scopes in
// internal/db/migrations/0005_authz.sql. This package is the evaluation
// engine only -- it has no direct database dependency (see Store) and knows
// nothing about HTTP; callers in internal/api and internal/admin resolve a
// Principal and Resource and call Decide.
package authz

// Capability is one of the spec Sec 12.1 consumer-read capability names. A
// closed Go type (not a bare string) so a typo can't silently fail to match
// any rule and get missed in review -- callers should only ever use the
// Cap* constants below, never a literal string.
type Capability string

// The minimum consumer capability vocabulary this deployment enforces
// (spec Sec 12.1). insight.query and the events.*/subscription.* family are
// intentionally absent -- opentea has no query language and no event feed
// yet, so there is nothing for those capabilities to gate.
const (
	CapProductDiscover      Capability = "product.discover"
	CapProductRead          Capability = "product.read"
	CapReleaseDiscover      Capability = "release.discover"
	CapReleaseRead          Capability = "release.read"
	CapLifecycleCurrentRead Capability = "lifecycle.current.read"
	CapLifecycleHistoryRead Capability = "lifecycle.history.read"
	CapCollectionDiscover   Capability = "collection.discover"
	CapCollectionRead       Capability = "collection.read"
	CapCollectionDownload   Capability = "collection.download"
	CapArtifactDiscover     Capability = "artifact.discover"
	CapArtifactMetadataRead Capability = "artifact.metadata.read"
	CapArtifactDownload     Capability = "artifact.download"
)

// ArtifactType constrains an artifact.* capability rule to one controlled
// artifact classification (spec Sec 17.1). It mirrors pkg/tea's artifact
// type enum verbatim; an empty ArtifactType means "unconstrained" (applies
// to an artifact of any type).
type ArtifactType string

// SubjectType is one of the three subjects this single-tenant profile
// supports (spec Sec 27's minimum profile, minus the organization subject:
// this deployment is one implicit organization, so there is nothing for it
// to distinguish).
type SubjectType string

// Subject specificity, broadest to narrowest, is everyone < authenticated <
// principal (spec Sec 15.3).
const (
	SubjectEveryone      SubjectType = "everyone"
	SubjectAuthenticated SubjectType = "authenticated"
	SubjectPrincipal     SubjectType = "principal"
)

// Principal identifies the caller a capability check is evaluated for.
// Anonymous callers are represented as a Principal with an empty UserUUID,
// not a separate nil/pointer case -- so a forgotten nil-check can never
// accidentally treat an anonymous request as authenticated.
type Principal struct {
	UserUUID string
}

// IsAuthenticated reports whether p represents a resolved, validated bearer
// caller rather than an anonymous one.
func (p Principal) IsAuthenticated() bool { return p.UserUUID != "" }

// ResourceScopeType enumerates the resource-scope kinds a capability check
// can be evaluated against. Declaration order is significant: it IS the
// resource-specificity ranking from spec Sec 15.2 (broadest first), relied
// on directly by moreSpecific in engine.go.
type ResourceScopeType int

// Resource specificity, broadest to narrowest (spec Sec 15.2), extended
// with ScopeCollection/ScopeArtifact beyond the spec's five-scope minimum
// profile since this deployment enforces collection/artifact capabilities.
const (
	ScopeAllProducts ResourceScopeType = iota
	ScopeProductGroup
	ScopeProduct
	ScopeReleaseGroup
	ScopeProductRelease
	ScopeCollection
	ScopeArtifact
)

// Resource identifies the concrete thing a capability check is against.
// Callers set exactly the field(s) relevant to the capability being
// checked, and nothing else: a product.* check sets only ProductUUID; a
// release.*/lifecycle.* check on a product release sets only
// ProductReleaseUUID (its owning product is derived via a join, not
// required from the caller); the same checks on a component/component
// release set ComponentUUID/ComponentReleaseUUID instead -- TEA's wire
// model has Component/ComponentRelease as top-level resources independent
// of Product/ProductRelease, and Phase 1 gives them a simpler (ungrouped)
// scope model, see internal/db/migrations/0005_authz.sql's header; a
// collection.* check sets only CollectionUUID (its owning release and
// belongs_to are looked up from the collection row itself -- in this
// schema a collection's uuid IS its owning product_release or
// component_release's uuid, so no separate release field is needed); an
// artifact.* check sets ArtifactUUID (+ ArtifactType), and CollectionUUID
// too if the capability check is reached through one specific collection
// (e.g. an artifact fetched via a collection listing) -- left empty for a
// direct artifact fetch with no collection context, in which case every
// collection referencing the artifact is considered (spec Sec 17.2: shared
// artifacts are accessible via at least one authorized relationship).
type Resource struct {
	ProductUUID          string
	ProductReleaseUUID   string
	ComponentUUID        string
	ComponentReleaseUUID string
	CollectionUUID       string
	ArtifactUUID         string
	ArtifactType         ArtifactType
}

// ReasonCode is a stable, non-leaking explanation for a Decision -- safe to
// log (spec Sec 22.4/15.4) but MUST NOT be included in a /tea/v1 response
// body (spec 15.4: explainability must not leak to the consumer).
type ReasonCode string

// Reason codes a Decision can carry.
const (
	// ReasonNoApplicableRule means default deny: nothing matched at all.
	ReasonNoApplicableRule ReasonCode = "no_applicable_rule"
	// ReasonExplicitDeny means the winning candidate rule said deny.
	ReasonExplicitDeny ReasonCode = "explicit_deny"
	// ReasonExplicitAllow means the winning candidate rule said allow.
	ReasonExplicitAllow ReasonCode = "explicit_allow"
)

// Decision is the outcome of one Decide call, carrying enough provenance to
// satisfy spec Sec 15.4 (explainability) and Sec 22.4 (decision records)
// without exposing that provenance in a /tea/v1 response.
type Decision struct {
	Allowed                 bool
	Reason                  ReasonCode
	MatchedEntitlementUUID  string
	MatchedTemplateUUID     string
	MatchedTemplateRevision int
	ResourceScope           ResourceScopeType
	SubjectType             SubjectType
}

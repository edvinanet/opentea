package model

import (
	"time"

	"github.com/oej/opentea/internal/authz"
)

// Resource-scope kinds an Entitlement.ResourceType can hold, matching the
// entitlement.resource_type CHECK values in
// internal/db/migrations/0005_authz.sql.
const (
	ResourceAllProducts      = "all_products"
	ResourceProductGroup     = "product_group"
	ResourceProduct          = "product"
	ResourceReleaseGroup     = "release_group"
	ResourceProductRelease   = "product_release"
	ResourceCollection       = "collection"
	ResourceArtifact         = "artifact"
	ResourceComponent        = "component"
	ResourceComponentRelease = "component_release"
)

// Entitlement lifecycle states (entitlement.status CHECK values).
const (
	EntitlementActive    = "active"
	EntitlementSuspended = "suspended"
	EntitlementRevoked   = "revoked"
)

// Capability rule decisions (template_capability_rule.decision CHECK
// values) -- matches the string form authz.CandidateRule.Decision expects.
const (
	DecisionAllow = "allow"
	DecisionDeny  = "deny"
)

// Template is a reusable, versioned set of capability decisions (spec
// Sec 13). It names neither a subject nor a resource -- an Entitlement
// supplies those. ActiveRevision is 0 until an admin explicitly activates
// one via POST .../activate.
type Template struct {
	UUID           string    `json:"uuid"`
	Name           string    `json:"name"`
	Description    string    `json:"description,omitempty"`
	ActiveRevision int       `json:"activeRevision,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}

// TemplateCapabilityRule is one allow/deny decision within a
// TemplateRevision, optionally constrained to one artifact classification.
type TemplateCapabilityRule struct {
	Capability   authz.Capability   `json:"capability"`
	ArtifactType authz.ArtifactType `json:"artifactType,omitempty"`
	Decision     string             `json:"decision"`
}

// TemplateRevision is one immutable, versioned edit of a Template's rules
// (spec Sec 13.1: "Changing a template MUST create a new revision").
type TemplateRevision struct {
	TemplateUUID string                   `json:"templateUuid"`
	Revision     int                      `json:"revision"`
	CreatedAt    time.Time                `json:"createdAt"`
	CreatedBy    string                   `json:"createdBy,omitempty"`
	Comment      string                   `json:"comment,omitempty"`
	Rules        []TemplateCapabilityRule `json:"rules"`
}

// ProductGroup is a named, publisher-managed set of products usable as an
// entitlement resource scope (spec Sec 11.2). Flat -- no nesting.
type ProductGroup struct {
	UUID        string    `json:"uuid"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// ReleaseGroup is a named set of product releases for a single product,
// such as a maintenance track (spec Sec 4.1). Product-release-only in
// Phase 1 -- see internal/db/migrations/0005_authz.sql's header comment.
type ReleaseGroup struct {
	UUID        string    `json:"uuid"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Entitlement is the assignment of a subject + resource scope to a template
// revision, with status/validity/provenance (spec Sec 14.1). SubjectID is
// set only when SubjectType is "principal" (a user.uuid); ResourceID is set
// for every ResourceType except ResourceAllProducts.
type Entitlement struct {
	UUID              string            `json:"uuid"`
	SubjectType       authz.SubjectType `json:"subjectType"`
	SubjectID         string            `json:"subjectId,omitempty"`
	TemplateUUID      string            `json:"templateUuid"`
	TemplateRevision  int               `json:"templateRevision"`
	ResourceType      string            `json:"resourceType"`
	ResourceID        string            `json:"resourceId,omitempty"`
	Status            string            `json:"status"`
	ValidFrom         *time.Time        `json:"validFrom,omitempty"`
	ValidUntil        *time.Time        `json:"validUntil,omitempty"`
	GrantingAuthority string            `json:"grantingAuthority"`
	CreatedAt         time.Time         `json:"createdAt"`
	CreatedBy         string            `json:"createdBy,omitempty"`
	Revision          int               `json:"revision"`
}

// AdminAuditEntry is one administrative audit record (spec Sec 22.5):
// template/entitlement/product-group/release-group lifecycle changes.
// PreviousState is empty for creates; both states are JSON snapshots of the
// affected object, not free-form text.
type AdminAuditEntry struct {
	ID             int64     `json:"id"`
	At             time.Time `json:"at"`
	ActorUUID      string    `json:"actorUuid,omitempty"`
	RequestID      string    `json:"requestId,omitempty"`
	Operation      string    `json:"operation"`
	TargetType     string    `json:"targetType"`
	TargetID       string    `json:"targetId"`
	PreviousState  string    `json:"previousState,omitempty"`
	ResultingState string    `json:"resultingState"`
	Reason         string    `json:"reason,omitempty"`
}

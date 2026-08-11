package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/oej/opentea/internal/authz"
)

// var _ authz.Store = (*Repo)(nil) documents (and enforces at compile time)
// that *Repo satisfies the interface internal/authz's policy engine depends
// on -- see CandidateRules below.
var _ authz.Store = (*Repo)(nil)

// CandidateRules implements authz.Store against the real database. It
// dispatches on which Resource field is set (see Resource's doc comment
// for the exact contract callers must follow) to one of four expansion
// strategies -- product, release, collection, or artifact -- each of which
// issues a handful of small, independently indexed queries (one per
// resource-scope tier that could apply) rather than one large join, mainly
// for clarity and testability; Phase 1 accepts the modest extra round-trip
// cost (see the plan's "known Phase 1 simplification" note on
// internal/api's filterAuthorized, which makes the same tradeoff for list
// filtering).
func (r *Repo) CandidateRules(ctx context.Context, subject authz.Principal, capability authz.Capability, resource authz.Resource) ([]authz.CandidateRule, error) {
	switch {
	case resource.ArtifactUUID != "":
		return r.candidateRulesForArtifact(ctx, subject, capability, resource)
	case resource.CollectionUUID != "":
		return r.candidateRulesForCollection(ctx, subject, capability, resource.CollectionUUID, resource.ArtifactType)
	case resource.ProductReleaseUUID != "":
		return r.candidateRulesForRelease(ctx, subject, capability, resource.ProductReleaseUUID, resource.ArtifactType)
	case resource.ProductUUID != "":
		return r.candidateRulesForProduct(ctx, subject, capability, resource.ProductUUID, resource.ArtifactType)
	case resource.ComponentReleaseUUID != "":
		return r.candidateRulesForComponentRelease(ctx, subject, capability, resource.ComponentReleaseUUID, resource.ArtifactType)
	case resource.ComponentUUID != "":
		return r.candidateRulesForComponent(ctx, subject, capability, resource.ComponentUUID, resource.ArtifactType)
	default:
		return nil, errors.New("authz: Resource has no identifying field set")
	}
}

// candidateRulesForComponent expands through all_products and a direct
// 'component' scope on componentUUID. No component-group scope exists in
// Phase 1 (see internal/db/migrations/0005_authz.sql's header) -- the
// 'component' candidate is tagged authz.ScopeProduct, the same specificity
// tier as a direct product grant, since a single Decide call is always for
// one branch (product or component) and the two are never compared against
// each other.
func (r *Repo) candidateRulesForComponent(ctx context.Context, subject authz.Principal, capability authz.Capability, componentUUID string, artifactType authz.ArtifactType) ([]authz.CandidateRule, error) {
	allProducts, err := candidateRulesAtScope(ctx, r.conn(), subject, capability, artifactType, authz.ScopeAllProducts, "all_products", "")
	if err != nil {
		return nil, err
	}
	componentCandidates, err := candidateRulesAtScope(ctx, r.conn(), subject, capability, artifactType, authz.ScopeProduct, "component", componentUUID)
	if err != nil {
		return nil, err
	}
	return append(allProducts, componentCandidates...), nil
}

// candidateRulesForComponentRelease expands through everything
// candidateRulesForComponent does (via componentReleaseUUID's owning
// component), plus a direct 'component_release' scope on
// componentReleaseUUID itself (tagged authz.ScopeProductRelease). No
// component-release-group scope exists in Phase 1. Returns ErrNotFound if
// componentReleaseUUID doesn't exist.
func (r *Repo) candidateRulesForComponentRelease(ctx context.Context, subject authz.Principal, capability authz.Capability, componentReleaseUUID string, artifactType authz.ArtifactType) ([]authz.CandidateRule, error) {
	var componentUUID string
	err := r.conn().QueryRowContext(ctx, `SELECT component_uuid FROM component_release WHERE uuid = ?`, componentReleaseUUID).Scan(&componentUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	out, err := r.candidateRulesForComponent(ctx, subject, capability, componentUUID, artifactType)
	if err != nil {
		return nil, err
	}
	releaseCandidates, err := candidateRulesAtScope(ctx, r.conn(), subject, capability, artifactType, authz.ScopeProductRelease, "component_release", componentReleaseUUID)
	if err != nil {
		return nil, err
	}
	return append(out, releaseCandidates...), nil
}

// candidateRulesForProduct expands through all_products, every product
// group productUUID belongs to, and productUUID itself.
func (r *Repo) candidateRulesForProduct(ctx context.Context, subject authz.Principal, capability authz.Capability, productUUID string, artifactType authz.ArtifactType) ([]authz.CandidateRule, error) {
	var out []authz.CandidateRule

	allProducts, err := candidateRulesAtScope(ctx, r.conn(), subject, capability, artifactType, authz.ScopeAllProducts, "all_products", "")
	if err != nil {
		return nil, err
	}
	out = append(out, allProducts...)

	groupUUIDs, err := r.ListProductGroupsContaining(ctx, productUUID)
	if err != nil {
		return nil, err
	}
	for _, groupUUID := range groupUUIDs {
		candidates, err := candidateRulesAtScope(ctx, r.conn(), subject, capability, artifactType, authz.ScopeProductGroup, "product_group", groupUUID)
		if err != nil {
			return nil, err
		}
		out = append(out, candidates...)
	}

	productCandidates, err := candidateRulesAtScope(ctx, r.conn(), subject, capability, artifactType, authz.ScopeProduct, "product", productUUID)
	if err != nil {
		return nil, err
	}
	return append(out, productCandidates...), nil
}

// candidateRulesForRelease expands through everything candidateRulesForProduct
// does (via releaseUUID's owning product), plus every release group
// releaseUUID belongs to, plus releaseUUID itself. Returns ErrNotFound if
// releaseUUID doesn't exist.
func (r *Repo) candidateRulesForRelease(ctx context.Context, subject authz.Principal, capability authz.Capability, releaseUUID string, artifactType authz.ArtifactType) ([]authz.CandidateRule, error) {
	var productUUID string
	err := r.conn().QueryRowContext(ctx, `SELECT product_uuid FROM product_release WHERE uuid = ?`, releaseUUID).Scan(&productUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	out, err := r.candidateRulesForProduct(ctx, subject, capability, productUUID, artifactType)
	if err != nil {
		return nil, err
	}

	groupUUIDs, err := r.ListReleaseGroupsContaining(ctx, releaseUUID)
	if err != nil {
		return nil, err
	}
	for _, groupUUID := range groupUUIDs {
		candidates, err := candidateRulesAtScope(ctx, r.conn(), subject, capability, artifactType, authz.ScopeReleaseGroup, "release_group", groupUUID)
		if err != nil {
			return nil, err
		}
		out = append(out, candidates...)
	}

	releaseCandidates, err := candidateRulesAtScope(ctx, r.conn(), subject, capability, artifactType, authz.ScopeProductRelease, "product_release", releaseUUID)
	if err != nil {
		return nil, err
	}
	return append(out, releaseCandidates...), nil
}

// candidateRulesForCollection expands independently of product/release
// visibility (spec Sec 12.2/17.1): a direct 'collection' scope on
// collectionUUID always applies, plus everything candidateRulesForRelease
// (belongs_to PRODUCT_RELEASE) or candidateRulesForComponentRelease
// (belongs_to COMPONENT_RELEASE) expands to for its owning release -- a
// release-scoped entitlement only produces a candidate here if its
// *template* actually has a collection.* rule, which is what keeps
// product/release access from implying collection access. Returns
// ErrNotFound if collectionUUID doesn't exist (any version).
func (r *Repo) candidateRulesForCollection(ctx context.Context, subject authz.Principal, capability authz.Capability, collectionUUID string, artifactType authz.ArtifactType) ([]authz.CandidateRule, error) {
	var belongsTo string
	err := r.conn().QueryRowContext(ctx, `SELECT belongs_to FROM collection WHERE uuid = ? LIMIT 1`, collectionUUID).Scan(&belongsTo)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	// collection.uuid IS its owning product_release.uuid or
	// component_release.uuid in this schema (see
	// internal/db/migrations/0001_init.sql's comment on the collection
	// table) -- no separate lookup needed.
	var out []authz.CandidateRule
	if belongsTo == "PRODUCT_RELEASE" {
		releaseCandidates, err := r.candidateRulesForRelease(ctx, subject, capability, collectionUUID, artifactType)
		if err != nil {
			return nil, err
		}
		out = append(out, releaseCandidates...)
	} else {
		releaseCandidates, err := r.candidateRulesForComponentRelease(ctx, subject, capability, collectionUUID, artifactType)
		if err != nil {
			return nil, err
		}
		out = append(out, releaseCandidates...)
	}

	collectionCandidates, err := candidateRulesAtScope(ctx, r.conn(), subject, capability, artifactType, authz.ScopeCollection, "collection", collectionUUID)
	if err != nil {
		return nil, err
	}
	return append(out, collectionCandidates...), nil
}

// candidateRulesForArtifact expands independently of collection/release/
// product visibility (spec Sec 12.2): all_products, a direct 'artifact'
// scope on artifactUUID, and -- for each collection currently referencing
// artifactUUID (or just the one collection the caller already knows about,
// if resource.CollectionUUID was set) -- that collection's own expansion
// via candidateRulesForCollection. A shared artifact is reachable if ANY
// of its relationships grants access (spec Sec 17.2), so this unions
// across every referencing collection rather than requiring all of them.
func (r *Repo) candidateRulesForArtifact(ctx context.Context, subject authz.Principal, capability authz.Capability, resource authz.Resource) ([]authz.CandidateRule, error) {
	var out []authz.CandidateRule

	allProducts, err := candidateRulesAtScope(ctx, r.conn(), subject, capability, resource.ArtifactType, authz.ScopeAllProducts, "all_products", "")
	if err != nil {
		return nil, err
	}
	out = append(out, allProducts...)

	artifactCandidates, err := candidateRulesAtScope(ctx, r.conn(), subject, capability, resource.ArtifactType, authz.ScopeArtifact, "artifact", resource.ArtifactUUID)
	if err != nil {
		return nil, err
	}
	out = append(out, artifactCandidates...)

	collectionUUIDs := []string{resource.CollectionUUID}
	if resource.CollectionUUID == "" {
		collectionUUIDs, err = r.listCollectionsContainingArtifact(ctx, resource.ArtifactUUID)
		if err != nil {
			return nil, err
		}
	}
	for _, collectionUUID := range collectionUUIDs {
		candidates, err := r.candidateRulesForCollection(ctx, subject, capability, collectionUUID, resource.ArtifactType)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, candidates...)
	}
	return out, nil
}

// listCollectionsContainingArtifact returns the distinct collection UUIDs
// (across every version) that reference artifactUUID (any version) via
// collection_artifact.
func (r *Repo) listCollectionsContainingArtifact(ctx context.Context, artifactUUID string) ([]string, error) {
	rows, err := r.conn().QueryContext(ctx, `SELECT DISTINCT collection_uuid FROM collection_artifact WHERE artifact_uuid = ?`, artifactUUID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			return nil, err
		}
		out = append(out, uuid)
	}
	return out, rows.Err()
}

// ListProductGroupsContaining returns every product_group UUID productUUID
// belongs to.
func (r *Repo) ListProductGroupsContaining(ctx context.Context, productUUID string) ([]string, error) {
	rows, err := r.conn().QueryContext(ctx, `SELECT product_group_uuid FROM product_group_member WHERE product_uuid = ?`, productUUID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			return nil, err
		}
		out = append(out, uuid)
	}
	return out, rows.Err()
}

// ListReleaseGroupsContaining returns every release_group UUID releaseUUID
// belongs to.
func (r *Repo) ListReleaseGroupsContaining(ctx context.Context, releaseUUID string) ([]string, error) {
	rows, err := r.conn().QueryContext(ctx, `SELECT release_group_uuid FROM release_group_member WHERE product_release_uuid = ?`, releaseUUID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			return nil, err
		}
		out = append(out, uuid)
	}
	return out, rows.Err()
}

// candidateRulesAtScope is the one query every expansion tier funnels
// through: entitlements at (resourceType, resourceID) [resourceID ignored
// when resourceType is "all_products", where it's always NULL], applicable
// to subject, not expired/inactive, joined to their template's rule for
// capability [+ artifactType if set].
func candidateRulesAtScope(ctx context.Context, q dbtx, subject authz.Principal, capability authz.Capability, artifactType authz.ArtifactType, scope authz.ResourceScopeType, resourceType, resourceID string) ([]authz.CandidateRule, error) {
	now := formatTime(time.Now())

	query := `
		SELECT e.uuid, e.template_uuid, e.template_revision, e.subject_type, r.decision
		FROM entitlement e
		JOIN template_capability_rule r ON r.template_uuid = e.template_uuid AND r.revision = e.template_revision
		WHERE e.resource_type = ?
		  AND (e.resource_type = 'all_products' OR e.resource_id = ?)
		  AND r.capability = ?
		  AND (r.artifact_type IS NULL OR r.artifact_type = ?)
		  AND e.status = 'active'
		  AND (e.valid_from IS NULL OR e.valid_from <= ?)
		  AND (e.valid_until IS NULL OR e.valid_until >= ?)
		  AND e.subject_type IN (` + subjectTypePlaceholders(subject) + `)
		  AND (e.subject_type != 'principal' OR e.subject_id = ?)`

	args := []any{resourceType, resourceID, string(capability), nullIfEmpty(string(artifactType)), now, now}
	args = append(args, subjectTypeArgs(subject)...)
	args = append(args, subject.UserUUID)

	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []authz.CandidateRule
	for rows.Next() {
		var c authz.CandidateRule
		var subjectType string
		if err := rows.Scan(&c.EntitlementUUID, &c.TemplateUUID, &c.TemplateRevision, &subjectType, &c.Decision); err != nil {
			return nil, err
		}
		c.SubjectType = authz.SubjectType(subjectType)
		c.ResourceScope = scope
		out = append(out, c)
	}
	return out, rows.Err()
}

// subjectTypePlaceholders returns the "?, ?, ..." placeholder list matching
// subjectTypeArgs, sized to how many subject types apply to subject.
func subjectTypePlaceholders(subject authz.Principal) string {
	if subject.IsAuthenticated() {
		return "?, ?, ?"
	}
	return "?"
}

// subjectTypeArgs returns the subject types applicable to subject:
// 'everyone' always; 'authenticated' and 'principal' too if subject is
// authenticated (an anonymous caller can never match an 'authenticated' or
// 'principal' entitlement).
func subjectTypeArgs(subject authz.Principal) []any {
	if subject.IsAuthenticated() {
		return []any{string(authz.SubjectEveryone), string(authz.SubjectAuthenticated), string(authz.SubjectPrincipal)}
	}
	return []any{string(authz.SubjectEveryone)}
}

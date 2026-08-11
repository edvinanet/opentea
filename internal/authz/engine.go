package authz

import "context"

// CandidateRule is one applicable, non-expired, active (subject, resource
// scope, decision) tuple gathered from entitlements before ranking. Store
// implementations are responsible for excluding expired/inactive
// entitlements and non-matching subjects; Decide only ranks whatever it's
// given.
type CandidateRule struct {
	EntitlementUUID  string
	TemplateUUID     string
	TemplateRevision int
	ResourceScope    ResourceScopeType
	SubjectType      SubjectType
	// Decision is "allow" or "deny", copied verbatim from
	// template_capability_rule.decision.
	Decision string
}

// Store is the read-only data dependency Decide needs. Implemented by
// internal/repo against the real database; kept as an interface here (not a
// direct repo dependency) so the ranking/deny-wins algorithm is unit
// testable against a fake with no SQLite involved.
type Store interface {
	// CandidateRules returns every entitlement-derived rule applicable to
	// subject, capability, and resource: scoped to a resource-scope that
	// contains resource (e.g. for a product-release resource, this
	// includes rules scoped to that exact release, any release group
	// containing it, its product, any product group containing that
	// product, and all-products); applicable to one of subject's subject
	// types (always 'everyone'; 'authenticated' and 'principal' only if
	// subject.IsAuthenticated(), with 'principal' matched to
	// subject.UserUUID specifically); not expired (valid_from/valid_until)
	// and status='active' as of now; and has an explicit allow or deny
	// rule for capability (constrained by resource.ArtifactType when set)
	// in that entitlement's template revision. An entitlement whose
	// template has no rule at all for capability is NOT returned -- that's
	// the "inherit" case (this entitlement doesn't speak to this
	// capability), which must fall through to a broader or default
	// decision, not be reported as a candidate.
	CandidateRules(ctx context.Context, subject Principal, capability Capability, resource Resource) ([]CandidateRule, error)
}

// Decide implements spec Sec 15.1-15.3's deterministic evaluation
// algorithm: gather every applicable candidate rule; if none, default deny
// (Sec 15.3); otherwise narrow to the candidates at the single narrowest
// resource-specificity tier present (Sec 15.2's ordering, encoded in
// ResourceScopeType's declaration order), then narrow further to the
// narrowest subject-specificity tier present within that (everyone <
// authenticated < principal); at that final tier, a deny wins over any
// allow (Sec 15.2/15.3, "conflicting decisions ... resolve to deny").
//
// Decide does not implement spec Sec 15.1's "apply active emergency
// restrictions" step -- opentea has no emergency-restriction concept
// (Phase 1 deferral, see plan); a future addition would be a new
// higher-priority candidate source, not a change to this ranking.
//
// A non-nil error from store.CandidateRules is propagated as-is. Per spec
// Sec 24 ("fail closed"), callers MUST treat any such error as a deny for
// anything but a resource that is locally known to be public independent of
// this call.
func Decide(ctx context.Context, store Store, subject Principal, capability Capability, resource Resource) (Decision, error) {
	candidates, err := store.CandidateRules(ctx, subject, capability, resource)
	if err != nil {
		return Decision{}, err
	}
	if len(candidates) == 0 {
		return Decision{Allowed: false, Reason: ReasonNoApplicableRule}, nil
	}

	atNarrowestScope := narrowestByResourceScope(candidates)
	final := narrowestBySubject(atNarrowestScope)

	for _, c := range final {
		if c.Decision == "deny" {
			return decisionFrom(c, false, ReasonExplicitDeny), nil
		}
	}
	return decisionFrom(final[0], true, ReasonExplicitAllow), nil
}

// narrowestByResourceScope returns the subset of candidates at the single
// highest (narrowest) ResourceScope value present.
func narrowestByResourceScope(candidates []CandidateRule) []CandidateRule {
	maxScope := candidates[0].ResourceScope
	for _, c := range candidates[1:] {
		if c.ResourceScope > maxScope {
			maxScope = c.ResourceScope
		}
	}
	var out []CandidateRule
	for _, c := range candidates {
		if c.ResourceScope == maxScope {
			out = append(out, c)
		}
	}
	return out
}

// narrowestBySubject returns the subset of candidates at the single highest
// (narrowest) subject-specificity rank present.
func narrowestBySubject(candidates []CandidateRule) []CandidateRule {
	maxRank := subjectRank(candidates[0].SubjectType)
	for _, c := range candidates[1:] {
		if r := subjectRank(c.SubjectType); r > maxRank {
			maxRank = r
		}
	}
	var out []CandidateRule
	for _, c := range candidates {
		if subjectRank(c.SubjectType) == maxRank {
			out = append(out, c)
		}
	}
	return out
}

// subjectRank orders SubjectType from broadest (0) to narrowest (spec
// Sec 15.3: everyone < authenticated < principal).
func subjectRank(s SubjectType) int {
	switch s {
	case SubjectPrincipal:
		return 2
	case SubjectAuthenticated:
		return 1
	default: // SubjectEveryone
		return 0
	}
}

func decisionFrom(c CandidateRule, allowed bool, reason ReasonCode) Decision {
	return Decision{
		Allowed:                 allowed,
		Reason:                  reason,
		MatchedEntitlementUUID:  c.EntitlementUUID,
		MatchedTemplateUUID:     c.TemplateUUID,
		MatchedTemplateRevision: c.TemplateRevision,
		ResourceScope:           c.ResourceScope,
		SubjectType:             c.SubjectType,
	}
}

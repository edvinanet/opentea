package admin

import "github.com/oej/opentea/internal/authz"

// validCapabilities/validSubjectTypes/validResourceTypes mirror the CHECK
// constraints in internal/db/migrations/0005_authz.sql -- duplicated here
// (not derived from the DB) so a malformed request gets a clean 400 with a
// specific message instead of a raw SQL constraint-violation 500. Keep in
// sync with the migration if either changes. validArtifactTypes itself is
// artifact.go's existing map (same artifact.type enum, reused as-is).
var validCapabilities = map[authz.Capability]bool{
	authz.CapProductDiscover:      true,
	authz.CapProductRead:          true,
	authz.CapReleaseDiscover:      true,
	authz.CapReleaseRead:          true,
	authz.CapLifecycleCurrentRead: true,
	authz.CapLifecycleHistoryRead: true,
	authz.CapCollectionDiscover:   true,
	authz.CapCollectionRead:       true,
	authz.CapCollectionDownload:   true,
	authz.CapArtifactDiscover:     true,
	authz.CapArtifactMetadataRead: true,
	authz.CapArtifactDownload:     true,
}

var validSubjectTypes = map[authz.SubjectType]bool{
	authz.SubjectEveryone:      true,
	authz.SubjectAuthenticated: true,
	authz.SubjectPrincipal:     true,
}

var validResourceTypes = map[string]bool{
	"all_products": true, "product_group": true, "product": true,
	"release_group": true, "product_release": true, "collection": true, "artifact": true,
	"component": true, "component_release": true,
}

var validDecisions = map[string]bool{"allow": true, "deny": true}

// validateRules reports the first validation error found in rules, or "" if
// none.
func validateRules(rules []capabilityRuleRequest) string {
	for _, rule := range rules {
		if !validCapabilities[rule.Capability] {
			return "invalid capability: " + string(rule.Capability)
		}
		if rule.ArtifactType != "" && !validArtifactTypes[string(rule.ArtifactType)] {
			return "invalid artifactType: " + string(rule.ArtifactType)
		}
		if !validDecisions[rule.Decision] {
			return "invalid decision (must be \"allow\" or \"deny\"): " + rule.Decision
		}
	}
	return ""
}

// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package admin

import (
	"net/http"

	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/trust"
)

// registerRoutes wires every admin endpoint behind requireRole: GET (read)
// routes need at least "consumer"; POST/DELETE (write) routes need "admin".
func (s *Server) registerRoutes(mux *http.ServeMux) {
	consumer := model.RoleConsumer
	admin := model.RoleAdmin

	mux.HandleFunc("POST /admin/v1/products", s.requireRole(admin, s.createProduct))
	mux.HandleFunc("GET /admin/v1/products", s.requireRole(consumer, s.listProducts))
	mux.HandleFunc("GET /admin/v1/products/{uuid}", s.requireRole(consumer, s.getProduct))
	mux.HandleFunc("DELETE /admin/v1/products/{uuid}", s.requireRole(admin, s.deleteProduct))
	mux.HandleFunc("POST /admin/v1/products/{uuid}/releases", s.requireRole(admin, s.createProductRelease))
	mux.HandleFunc("POST /admin/v1/products/{uuid}/cle/events", s.requireRole(admin, s.createCLEEventForOwner("PRODUCT")))
	mux.HandleFunc("POST /admin/v1/products/{uuid}/cle/definitions", s.requireRole(admin, s.createCLEDefinitionForOwner("PRODUCT")))

	mux.HandleFunc("GET /admin/v1/productReleases/{uuid}", s.requireRole(consumer, s.getProductRelease))
	mux.HandleFunc("DELETE /admin/v1/productReleases/{uuid}", s.requireRole(admin, s.deleteProductRelease))
	mux.HandleFunc("POST /admin/v1/productReleases/{uuid}/components", s.requireRole(admin, s.linkComponent))
	mux.HandleFunc("POST /admin/v1/productReleases/{uuid}/collections", s.requireRole(admin, s.createCollectionForProductRelease))
	mux.HandleFunc("POST /admin/v1/productReleases/{uuid}/cle/events", s.requireRole(admin, s.createCLEEventForOwner("PRODUCT_RELEASE")))
	mux.HandleFunc("POST /admin/v1/productReleases/{uuid}/cle/definitions", s.requireRole(admin, s.createCLEDefinitionForOwner("PRODUCT_RELEASE")))

	mux.HandleFunc("POST /admin/v1/components", s.requireRole(admin, s.createComponent))
	mux.HandleFunc("GET /admin/v1/components", s.requireRole(consumer, s.listComponents))
	mux.HandleFunc("GET /admin/v1/components/{uuid}", s.requireRole(consumer, s.getComponent))
	mux.HandleFunc("DELETE /admin/v1/components/{uuid}", s.requireRole(admin, s.deleteComponent))
	mux.HandleFunc("POST /admin/v1/components/{uuid}/releases", s.requireRole(admin, s.createComponentRelease))
	mux.HandleFunc("POST /admin/v1/components/{uuid}/cle/events", s.requireRole(admin, s.createCLEEventForOwner("COMPONENT")))
	mux.HandleFunc("POST /admin/v1/components/{uuid}/cle/definitions", s.requireRole(admin, s.createCLEDefinitionForOwner("COMPONENT")))

	mux.HandleFunc("GET /admin/v1/componentReleases/{uuid}", s.requireRole(consumer, s.getComponentRelease))
	mux.HandleFunc("DELETE /admin/v1/componentReleases/{uuid}", s.requireRole(admin, s.deleteComponentRelease))
	mux.HandleFunc("POST /admin/v1/componentReleases/{uuid}/distributions", s.requireRole(admin, s.createDistribution))
	mux.HandleFunc("POST /admin/v1/componentReleases/{uuid}/collections", s.requireRole(admin, s.createCollectionForComponentRelease))
	mux.HandleFunc("POST /admin/v1/componentReleases/{uuid}/cle/events", s.requireRole(admin, s.createCLEEventForOwner("COMPONENT_RELEASE")))
	mux.HandleFunc("POST /admin/v1/componentReleases/{uuid}/cle/definitions", s.requireRole(admin, s.createCLEDefinitionForOwner("COMPONENT_RELEASE")))

	mux.HandleFunc("POST /admin/v1/distributions/{id}/files", s.requireRole(admin, s.uploadDistributionFile))

	mux.HandleFunc("POST /admin/v1/artifacts", s.requireRole(admin, s.createArtifact))
	mux.HandleFunc("POST /admin/v1/artifacts/{uuid}/{version}/files", s.requireRole(admin, s.uploadArtifactFormatFile))

	mux.HandleFunc("POST /admin/v1/users", s.requireRole(admin, s.createUser))
	mux.HandleFunc("GET /admin/v1/users", s.requireRole(consumer, s.listUsers))
	mux.HandleFunc("DELETE /admin/v1/users/{uuid}", s.requireRole(admin, s.deleteUser))

	mux.HandleFunc("GET /admin/v1/stats", s.requireRole(consumer, s.getStats))

	// Product import/export bundles: backup, ownership transfer, and
	// provider migration -- admin-only in both directions (not
	// consumer-accessible), and entirely separate from /tea/v1 and any
	// future publisher API. See docs/bundle-format.md.
	mux.HandleFunc("GET /admin/v1/products/{uuid}/export", s.requireRole(admin, s.exportProduct))
	mux.HandleFunc("POST /admin/v1/products/import", s.requireRole(admin, s.importProduct))

	// Consumer-API (/tea/v1) authorization management: templates,
	// entitlements, product/release groups. See
	// ~/TEA_AUTHENTICATION_AUTHORIZATION_SPECIFICATION.md and
	// internal/db/migrations/0005_authz.sql. JSON API only in this phase --
	// no /admin/ui pages yet.
	mux.HandleFunc("POST /admin/v1/templates", s.requireRole(admin, s.createTemplate))
	mux.HandleFunc("GET /admin/v1/templates", s.requireRole(consumer, s.listTemplates))
	mux.HandleFunc("GET /admin/v1/templates/{uuid}", s.requireRole(consumer, s.getTemplate))
	mux.HandleFunc("POST /admin/v1/templates/{uuid}/revisions", s.requireRole(admin, s.createTemplateRevision))
	mux.HandleFunc("POST /admin/v1/templates/{uuid}/activate", s.requireRole(admin, s.activateTemplateRevision))
	mux.HandleFunc("DELETE /admin/v1/templates/{uuid}", s.requireRole(admin, s.deleteTemplate))

	mux.HandleFunc("POST /admin/v1/entitlements", s.requireRole(admin, s.createEntitlement))
	mux.HandleFunc("GET /admin/v1/entitlements", s.requireRole(consumer, s.listEntitlements))
	mux.HandleFunc("GET /admin/v1/entitlements/{uuid}", s.requireRole(consumer, s.getEntitlement))
	mux.HandleFunc("PATCH /admin/v1/entitlements/{uuid}/status", s.requireRole(admin, s.updateEntitlementStatus))
	mux.HandleFunc("PATCH /admin/v1/entitlements/{uuid}/validity", s.requireRole(admin, s.updateEntitlementValidity))
	mux.HandleFunc("DELETE /admin/v1/entitlements/{uuid}", s.requireRole(admin, s.deleteEntitlement))

	mux.HandleFunc("POST /admin/v1/productGroups", s.requireRole(admin, s.createProductGroup))
	mux.HandleFunc("GET /admin/v1/productGroups", s.requireRole(consumer, s.listProductGroups))
	mux.HandleFunc("GET /admin/v1/productGroups/{uuid}", s.requireRole(consumer, s.getProductGroup))
	mux.HandleFunc("DELETE /admin/v1/productGroups/{uuid}", s.requireRole(admin, s.deleteProductGroup))
	mux.HandleFunc("POST /admin/v1/productGroups/{uuid}/members", s.requireRole(admin, s.addProductGroupMember))
	mux.HandleFunc("DELETE /admin/v1/productGroups/{uuid}/members/{productUuid}", s.requireRole(admin, s.removeProductGroupMember))

	mux.HandleFunc("POST /admin/v1/releaseGroups", s.requireRole(admin, s.createReleaseGroup))
	mux.HandleFunc("GET /admin/v1/releaseGroups", s.requireRole(consumer, s.listReleaseGroups))
	mux.HandleFunc("GET /admin/v1/releaseGroups/{uuid}", s.requireRole(consumer, s.getReleaseGroup))
	mux.HandleFunc("DELETE /admin/v1/releaseGroups/{uuid}", s.requireRole(admin, s.deleteReleaseGroup))
	mux.HandleFunc("POST /admin/v1/releaseGroups/{uuid}/members", s.requireRole(admin, s.addReleaseGroupMember))
	mux.HandleFunc("DELETE /admin/v1/releaseGroups/{uuid}/members/{releaseUuid}", s.requireRole(admin, s.removeReleaseGroupMember))

	// TEA Trust Architecture overlay (github.com/oej/tea-trust-architecture):
	// cryptographic evidence of an artifact/collection's origin and
	// integrity. Deliberately separate from the authz routes above -- authz
	// answers "can this caller read this," trust answers "is this evidence
	// valid." See internal/trust and internal/db/migrations/0006_trust.sql.
	// A collection is looked up by its own (uuid, version) identity here
	// (collection.uuid IS its owning product_release/component_release
	// uuid, per 0001_init.sql), not nested under productReleases/
	// componentReleases like collection creation is -- evidence attachment
	// doesn't need that route context.
	mux.HandleFunc("POST /admin/v1/artifacts/{uuid}/{version}/evidenceBundle", s.requireRole(admin, s.createEvidenceBundleForOwner("ARTIFACT", trust.ObjectTypeArtifact)))
	mux.HandleFunc("POST /admin/v1/collections/{uuid}/{version}/evidenceBundle", s.requireRole(admin, s.createEvidenceBundleForOwner("COLLECTION", trust.ObjectTypeCollection)))
	mux.HandleFunc("GET /admin/v1/evidenceBundles/{uuid}", s.requireRole(consumer, s.getEvidenceBundle))

	// /publisher/v1 Layer B/D bearer credential issuance (internal/publisher,
	// design/publisher-service.md §10.2/§10.4) -- an operator action, not
	// part of the standard protocol, hence admin-gated here rather than
	// exposed under /publisher/v1 itself. The raw token is returned only
	// once, at creation (createPublisherCredentialResponse).
	mux.HandleFunc("POST /admin/v1/publisherCredentials", s.requireRole(admin, s.createPublisherCredential))
	mux.HandleFunc("GET /admin/v1/publisherCredentials", s.requireRole(consumer, s.listPublisherCredentials))
	mux.HandleFunc("DELETE /admin/v1/publisherCredentials/{uuid}", s.requireRole(admin, s.revokePublisherCredential))
}

package admin

import (
	"net/http"

	"github.com/oej/opentea/internal/model"
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
}

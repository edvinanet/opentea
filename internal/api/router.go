package api

import "net/http"

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /tea/v1/product/{uuid}", s.getProduct)
	mux.HandleFunc("GET /tea/v1/product/{uuid}/releases", s.listReleasesByProduct)
	mux.HandleFunc("GET /tea/v1/product/{uuid}/cle", s.cleByProduct)
	mux.HandleFunc("GET /tea/v1/products", s.queryProducts)

	mux.HandleFunc("GET /tea/v1/productRelease/{uuid}", s.getProductRelease)
	mux.HandleFunc("GET /tea/v1/productRelease/{uuid}/cle", s.cleByProductRelease)
	mux.HandleFunc("GET /tea/v1/productReleases", s.queryProductReleases)
	mux.HandleFunc("GET /tea/v1/productRelease/{uuid}/collection/latest", s.latestCollectionForProductRelease)
	mux.HandleFunc("GET /tea/v1/productRelease/{uuid}/collections", s.listCollectionsForProductRelease)
	mux.HandleFunc("GET /tea/v1/productRelease/{uuid}/collection/{collectionVersion}", s.getCollectionForProductRelease)

	mux.HandleFunc("GET /tea/v1/component/{uuid}", s.getComponent)
	mux.HandleFunc("GET /tea/v1/component/{uuid}/releases", s.listReleasesByComponent)
	mux.HandleFunc("GET /tea/v1/component/{uuid}/cle", s.cleByComponent)
	mux.HandleFunc("GET /tea/v1/components", s.queryComponents)

	mux.HandleFunc("GET /tea/v1/componentRelease/{uuid}", s.getComponentReleaseWithCollection)
	mux.HandleFunc("GET /tea/v1/componentRelease/{uuid}/cle", s.cleByComponentRelease)
	mux.HandleFunc("GET /tea/v1/componentReleases", s.queryComponentReleases)
	mux.HandleFunc("GET /tea/v1/componentRelease/{uuid}/collection/latest", s.latestCollectionForComponentRelease)
	mux.HandleFunc("GET /tea/v1/componentRelease/{uuid}/collections", s.listCollectionsForComponentRelease)
	mux.HandleFunc("GET /tea/v1/componentRelease/{uuid}/collection/{collectionVersion}", s.getCollectionForComponentRelease)

	mux.HandleFunc("GET /tea/v1/artifact/{uuid}/latest", s.getLatestArtifact)
	mux.HandleFunc("GET /tea/v1/artifact/{uuid}/{artifactVersion}", s.getArtifactByVersion)

	mux.HandleFunc("GET /tea/v1/discovery", s.discoveryByTEI)
}

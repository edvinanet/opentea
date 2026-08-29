// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package api

import "net/http"

// registerRoutes wires the consumer API's routes under s.cfg.APIBasePath
// (default "/tea/v1", but configurable -- see config.Config.APIBasePath).
func (s *Server) registerRoutes(mux *http.ServeMux) {
	base := s.cfg.APIBasePath

	mux.HandleFunc("GET "+base+"/product/{uuid}", s.getProduct)
	mux.HandleFunc("GET "+base+"/product/{uuid}/releases", s.listReleasesByProduct)
	mux.HandleFunc("GET "+base+"/product/{uuid}/cle", s.cleByProduct)
	mux.HandleFunc("GET "+base+"/products", s.queryProducts)

	mux.HandleFunc("GET "+base+"/productRelease/{uuid}", s.getProductRelease)
	mux.HandleFunc("GET "+base+"/productRelease/{uuid}/cle", s.cleByProductRelease)
	mux.HandleFunc("GET "+base+"/productReleases", s.queryProductReleases)
	mux.HandleFunc("GET "+base+"/productRelease/{uuid}/collection/latest", s.latestCollectionForProductRelease)
	mux.HandleFunc("GET "+base+"/productRelease/{uuid}/collections", s.listCollectionsForProductRelease)
	mux.HandleFunc("GET "+base+"/productRelease/{uuid}/collection/{collectionVersion}", s.getCollectionForProductRelease)

	mux.HandleFunc("GET "+base+"/component/{uuid}", s.getComponent)
	mux.HandleFunc("GET "+base+"/component/{uuid}/releases", s.listReleasesByComponent)
	mux.HandleFunc("GET "+base+"/component/{uuid}/cle", s.cleByComponent)
	mux.HandleFunc("GET "+base+"/components", s.queryComponents)

	mux.HandleFunc("GET "+base+"/componentRelease/{uuid}", s.getComponentReleaseWithCollection)
	mux.HandleFunc("GET "+base+"/componentRelease/{uuid}/cle", s.cleByComponentRelease)
	mux.HandleFunc("GET "+base+"/componentReleases", s.queryComponentReleases)
	mux.HandleFunc("GET "+base+"/componentRelease/{uuid}/collection/latest", s.latestCollectionForComponentRelease)
	mux.HandleFunc("GET "+base+"/componentRelease/{uuid}/collections", s.listCollectionsForComponentRelease)
	mux.HandleFunc("GET "+base+"/componentRelease/{uuid}/collection/{collectionVersion}", s.getCollectionForComponentRelease)

	mux.HandleFunc("GET "+base+"/artifact/{uuid}/latest", s.getLatestArtifact)
	mux.HandleFunc("GET "+base+"/artifact/{uuid}/{artifactVersion}", s.getArtifactByVersion)

	mux.HandleFunc("GET "+base+"/discovery", s.discoveryByTEI)
}

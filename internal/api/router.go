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

	// TEA 1.0 (spec/openapi.yaml): artifact content/signature download,
	// both GET and HEAD. Registered without a method prefix (matching any
	// method, then guarded by requireGetOrHead below) rather than separate
	// "GET "/"HEAD " patterns -- Go's ServeMux rejects that combination
	// here: a wildcard HEAD pattern (.../{artifactVersion}/...) and a
	// literal GET pattern (.../latest/...) for an overlapping path panic
	// at registration ("matches fewer methods... but has a more general
	// path pattern"), a known ServeMux corner case when literal and
	// wildcard segments mix across methods.
	mux.HandleFunc(base+"/artifact/{uuid}/latest/download", requireGetOrHead(s.downloadLatestArtifact))
	mux.HandleFunc(base+"/artifact/{uuid}/{artifactVersion}/download", requireGetOrHead(s.downloadArtifactByVersion))
	mux.HandleFunc(base+"/artifact/{uuid}/latest/signature/download", requireGetOrHead(s.downloadLatestArtifactSignature))
	mux.HandleFunc(base+"/artifact/{uuid}/{artifactVersion}/signature/download", requireGetOrHead(s.downloadArtifactSignatureByVersion))

	mux.HandleFunc("GET "+base+"/discovery", s.discovery)

	mux.HandleFunc("POST "+base+"/token", s.requestToken)
}

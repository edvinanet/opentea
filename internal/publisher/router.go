// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package publisher

import (
	"net/http"

	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/repo"
)

// registerRoutes wires every /publisher/v1 endpoint behind requireScope:
// see design/publisher-service.md §10.4 for the full/cicd split rationale.
// "cicd" may create/upload/validate artifacts and drive collection-draft
// assembly and the mechanical prepare/cancel/commit steps; it may never
// create products/components/releases/CLE events, or approve/reject --
// "full" only.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	full := model.PublisherScopeFull
	cicd := model.PublisherScopeCICD

	// Products / Releases.
	mux.HandleFunc("POST /publisher/v1/products", s.requireScope(full, s.createProduct))
	mux.HandleFunc("POST /publisher/v1/products/{uuid}/releases", s.requireScope(full, s.createProductRelease))

	// Components.
	mux.HandleFunc("GET /publisher/v1/components", s.requireScope(full, s.findComponents))
	mux.HandleFunc("POST /publisher/v1/components", s.requireScope(full, s.createComponent))
	mux.HandleFunc("POST /publisher/v1/components/{uuid}/releases", s.requireScope(full, s.createComponentRelease))
	mux.HandleFunc("POST /publisher/v1/productReleases/{uuid}/components", s.requireScope(full, s.linkComponent))

	// Artifacts.
	mux.HandleFunc("POST /publisher/v1/artifacts", s.requireScope(cicd, s.createArtifact))
	mux.HandleFunc("POST /publisher/v1/artifacts/{uuid}/{version}/files", s.requireScope(cicd, s.uploadArtifactFile))
	mux.HandleFunc("POST /publisher/v1/artifacts/{uuid}/{version}/signature/files", s.requireScope(cicd, s.uploadArtifactSignatureFile))
	mux.HandleFunc("POST /publisher/v1/artifacts/{uuid}/{version}/evidence/prepare", s.requireScope(cicd, s.prepareArtifactEvidence))
	mux.HandleFunc("POST /publisher/v1/artifacts/{uuid}/{version}/evidence", s.requireScope(cicd, s.submitArtifactEvidence))

	// Lifecycle (CLE) -- product/productRelease/component/componentRelease.
	mux.HandleFunc("POST /publisher/v1/products/{uuid}/cle/events", s.requireScope(full, s.createCLEEventForOwner(repo.OwnerProduct)))
	mux.HandleFunc("POST /publisher/v1/productReleases/{uuid}/cle/events", s.requireScope(full, s.createCLEEventForOwner(repo.OwnerProductRelease)))
	mux.HandleFunc("POST /publisher/v1/components/{uuid}/cle/events", s.requireScope(full, s.createCLEEventForOwner(repo.OwnerComponent)))
	mux.HandleFunc("POST /publisher/v1/componentReleases/{uuid}/cle/events", s.requireScope(full, s.createCLEEventForOwner(repo.OwnerComponentRelease)))

	// Collections: staging + atomic commit, both product-release-owned and
	// component-release-owned (design/publisher-openapi.yaml only sketches
	// the productReleases path to keep the draft short -- see its own
	// comment at line 497-499 -- both are in scope here).
	s.registerCollectionDraftRoutes(mux, "productReleases", repo.BelongsToProductRelease)
	s.registerCollectionDraftRoutes(mux, "componentReleases", repo.BelongsToComponentRelease)
}

func (s *Server) registerCollectionDraftRoutes(mux *http.ServeMux, pathPrefix, ownerType string) {
	full := model.PublisherScopeFull
	cicd := model.PublisherScopeCICD
	base := "/publisher/v1/" + pathPrefix + "/{uuid}/collectionDraft"

	mux.HandleFunc("PUT "+base, s.requireScope(cicd, s.putCollectionDraftForOwner(ownerType)))
	mux.HandleFunc("GET "+base, s.requireScope(cicd, s.getCollectionDraftForOwner(ownerType)))
	mux.HandleFunc("DELETE "+base, s.requireScope(cicd, s.deleteCollectionDraftForOwner(ownerType)))
	mux.HandleFunc("POST "+base+"/approve", s.requireScope(full, s.decideCollectionDraftForOwner(ownerType, true)))
	mux.HandleFunc("POST "+base+"/reject", s.requireScope(full, s.decideCollectionDraftForOwner(ownerType, false)))
	mux.HandleFunc("POST "+base+"/prepareCommit", s.requireScope(cicd, s.prepareCollectionCommitForOwner(ownerType)))
	mux.HandleFunc("POST "+base+"/cancelPrepare", s.requireScope(cicd, s.cancelPrepareCollectionCommitForOwner(ownerType)))
	mux.HandleFunc("POST "+base+"/commit", s.requireScope(cicd, s.commitCollectionDraftForOwner(ownerType)))
}

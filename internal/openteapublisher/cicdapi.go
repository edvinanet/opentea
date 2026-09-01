// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// cicdapi.go implements opentea-publisher's own /cicdapi/v1 -- a mediated,
// narrower surface for CI/CD pipelines that proxies straight through to a
// stored Target's real /publisher/v1 via pkg/teapublisherclient, but only
// ever presents that target's "full"-scoped bearer_token to operations
// internal/publisher itself treats as cicd-scoped (design/publisher-
// service.md §18.11) -- product/component/CLE creation and collection-
// draft approve/reject stay unreachable from here, exactly mirroring
// internal/publisher/router.go's own full/cicd line.
package openteapublisher

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/pkg/tea"
	"github.com/oej/opentea/pkg/teapublisher"
	"github.com/oej/opentea/pkg/teapublisherclient"
)

// maxCICDUploadBody bounds an uploaded artifact file -- matches
// internal/publisher/artifact.go's own maxUploadBody, since this proxies
// straight through to the same protocol.
const maxCICDUploadBody = 1 << 30 // 1 GiB

// maxCICDJSONBody bounds a JSON request body -- matches
// internal/publisher/server.go's own maxJSONBody exactly, since this
// proxies straight through to that same protocol and shouldn't reject
// anything the real target would accept.
const maxCICDJSONBody = 10 << 20 // 10 MiB

// decodeJSON mirrors internal/publisher/server.go's own decodeJSON
// exactly (same limit, no DisallowUnknownFields) -- this package proxies
// requests to that same protocol, so it must accept exactly what the real
// target accepts, not a stricter subset.
func decodeJSON(r *http.Request, v any) error {
	defer func() { _ = r.Body.Close() }()
	return json.NewDecoder(io.LimitReader(r.Body, maxCICDJSONBody)).Decode(v)
}

// writeClientError replays a teapublisherclient error to the CI/CD caller
// as close to verbatim as possible: an *APIError carries the target's own
// real status/body (see pkg/teapublisherclient/errors.go's own doc
// comment -- written with exactly this use in mind), so the caller sees
// what the target actually said, not a paraphrase. Anything else (e.g.
// the target being unreachable) is a genuine internal error on
// opentea-publisher's side.
func writeClientError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *teapublisherclient.APIError
	if errors.As(err, &apiErr) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(apiErr.StatusCode)
		_, _ = w.Write(apiErr.Body)
		return
	}
	httpx.InternalError(w, r, err)
}

// clientForTarget builds a teapublisherclient.Client for targetUUID,
// presenting that target's own stored (full-scoped) bearer_token --
// requireCICDCredential has already established the caller may act as
// cicd for this target; the target itself never sees "cicd" here, since
// it isn't a distinct credential on that side, only a narrower one issued
// by opentea-publisher (cicd_credential).
func (s *Server) clientForTarget(ctx context.Context, targetUUID string) (*teapublisherclient.Client, error) {
	target, err := s.repo.GetTarget(ctx, targetUUID)
	if err != nil {
		return nil, err
	}
	return teapublisherclient.NewClient(target.BaseURL, target.BearerToken), nil
}

func (s *Server) cicdCreateArtifact(w http.ResponseWriter, r *http.Request, targetUUID string) {
	var req teapublisher.ArtifactCreate
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	client, err := s.clientForTarget(r.Context(), targetUUID)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	artifact, err := client.CreateArtifact(r.Context(), req)
	if err != nil {
		writeClientError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, artifact)
}

func (s *Server) cicdUploadArtifactFile(w http.ResponseWriter, r *http.Request, targetUUID string) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	version, err := httpx.PathPositiveInt(r, "version")
	if err != nil {
		httpx.BadRequest(w, "invalid version")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxCICDUploadBody)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httpx.BadRequest(w, "invalid multipart form: "+err.Error())
		return
	}
	mediaType := r.FormValue("mediaType")
	if mediaType == "" {
		httpx.BadRequest(w, "mediaType is required")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.BadRequest(w, "missing \"file\" form field: "+err.Error())
		return
	}
	defer func() { _ = file.Close() }()

	client, err := s.clientForTarget(r.Context(), targetUUID)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if err := client.UploadArtifactFile(r.Context(), uuid, version, mediaType, header.Filename, file); err != nil {
		writeClientError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) cicdPrepareArtifactEvidence(w http.ResponseWriter, r *http.Request, targetUUID string) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	version, err := httpx.PathPositiveInt(r, "version")
	if err != nil {
		httpx.BadRequest(w, "invalid version")
		return
	}
	client, err := s.clientForTarget(r.Context(), targetUUID)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	resp, err := client.PrepareArtifactEvidence(r.Context(), uuid, version)
	if err != nil {
		writeClientError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (s *Server) cicdSubmitArtifactEvidence(w http.ResponseWriter, r *http.Request, targetUUID string) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	version, err := httpx.PathPositiveInt(r, "version")
	if err != nil {
		httpx.BadRequest(w, "invalid version")
		return
	}
	var req teapublisher.EvidenceSubmission
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	client, err := s.clientForTarget(r.Context(), targetUUID)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	bundle, err := client.SubmitArtifactEvidence(r.Context(), uuid, version, req)
	if err != nil {
		writeClientError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, bundle)
}

// collectionDraftOps is a small dispatch table selecting which pair of
// owner-type-specific pkg/teapublisherclient methods a generic collection-
// draft handler should call -- mirrors internal/publisher/collectiondraft.go's
// own putCollectionDraftForOwner(ownerType) factory shape, but dispatches
// to two distinct client methods per owner type rather than one repo
// method taking an ownerType string, since the client exposes them that
// way.
type collectionDraftOps struct {
	put           func(c *teapublisherclient.Client, ctx context.Context, ownerUUID string, in teapublisher.CollectionDraftArtifactList) (teapublisher.CollectionDraft, error)
	get           func(c *teapublisherclient.Client, ctx context.Context, ownerUUID string) (teapublisher.CollectionDraft, error)
	deleteDraft   func(c *teapublisherclient.Client, ctx context.Context, ownerUUID string) error
	prepareCommit func(c *teapublisherclient.Client, ctx context.Context, ownerUUID string) (teapublisher.PrepareCommitResponse, error)
	cancelPrepare func(c *teapublisherclient.Client, ctx context.Context, ownerUUID string) error
	commit        func(c *teapublisherclient.Client, ctx context.Context, ownerUUID string, in teapublisher.EvidenceSubmission) (tea.Collection, error)
}

var productReleaseDraftOps = collectionDraftOps{
	put: func(c *teapublisherclient.Client, ctx context.Context, ownerUUID string, in teapublisher.CollectionDraftArtifactList) (teapublisher.CollectionDraft, error) {
		return c.PutProductReleaseCollectionDraft(ctx, ownerUUID, in)
	},
	get: func(c *teapublisherclient.Client, ctx context.Context, ownerUUID string) (teapublisher.CollectionDraft, error) {
		return c.GetProductReleaseCollectionDraft(ctx, ownerUUID)
	},
	deleteDraft: func(c *teapublisherclient.Client, ctx context.Context, ownerUUID string) error {
		return c.DeleteProductReleaseCollectionDraft(ctx, ownerUUID)
	},
	prepareCommit: func(c *teapublisherclient.Client, ctx context.Context, ownerUUID string) (teapublisher.PrepareCommitResponse, error) {
		return c.PrepareProductReleaseCollectionCommit(ctx, ownerUUID)
	},
	cancelPrepare: func(c *teapublisherclient.Client, ctx context.Context, ownerUUID string) error {
		return c.CancelPrepareProductReleaseCollectionCommit(ctx, ownerUUID)
	},
	commit: func(c *teapublisherclient.Client, ctx context.Context, ownerUUID string, in teapublisher.EvidenceSubmission) (tea.Collection, error) {
		return c.CommitProductReleaseCollectionDraft(ctx, ownerUUID, in)
	},
}

var componentReleaseDraftOps = collectionDraftOps{
	put: func(c *teapublisherclient.Client, ctx context.Context, ownerUUID string, in teapublisher.CollectionDraftArtifactList) (teapublisher.CollectionDraft, error) {
		return c.PutComponentReleaseCollectionDraft(ctx, ownerUUID, in)
	},
	get: func(c *teapublisherclient.Client, ctx context.Context, ownerUUID string) (teapublisher.CollectionDraft, error) {
		return c.GetComponentReleaseCollectionDraft(ctx, ownerUUID)
	},
	deleteDraft: func(c *teapublisherclient.Client, ctx context.Context, ownerUUID string) error {
		return c.DeleteComponentReleaseCollectionDraft(ctx, ownerUUID)
	},
	prepareCommit: func(c *teapublisherclient.Client, ctx context.Context, ownerUUID string) (teapublisher.PrepareCommitResponse, error) {
		return c.PrepareComponentReleaseCollectionCommit(ctx, ownerUUID)
	},
	cancelPrepare: func(c *teapublisherclient.Client, ctx context.Context, ownerUUID string) error {
		return c.CancelPrepareComponentReleaseCollectionCommit(ctx, ownerUUID)
	},
	commit: func(c *teapublisherclient.Client, ctx context.Context, ownerUUID string, in teapublisher.EvidenceSubmission) (tea.Collection, error) {
		return c.CommitComponentReleaseCollectionDraft(ctx, ownerUUID, in)
	},
}

func (s *Server) cicdPutCollectionDraft(ops collectionDraftOps) cicdHandler {
	return func(w http.ResponseWriter, r *http.Request, targetUUID string) {
		ownerUUID, err := httpx.PathUUID(r, "uuid")
		if err != nil {
			httpx.BadRequest(w, "invalid uuid")
			return
		}
		var req teapublisher.CollectionDraftArtifactList
		if err := decodeJSON(r, &req); err != nil {
			httpx.BadRequest(w, "invalid JSON body: "+err.Error())
			return
		}
		client, err := s.clientForTarget(r.Context(), targetUUID)
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		draft, err := ops.put(client, r.Context(), ownerUUID, req)
		if err != nil {
			writeClientError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, draft)
	}
}

func (s *Server) cicdGetCollectionDraft(ops collectionDraftOps) cicdHandler {
	return func(w http.ResponseWriter, r *http.Request, targetUUID string) {
		ownerUUID, err := httpx.PathUUID(r, "uuid")
		if err != nil {
			httpx.BadRequest(w, "invalid uuid")
			return
		}
		client, err := s.clientForTarget(r.Context(), targetUUID)
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		draft, err := ops.get(client, r.Context(), ownerUUID)
		if err != nil {
			writeClientError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, draft)
	}
}

func (s *Server) cicdDeleteCollectionDraft(ops collectionDraftOps) cicdHandler {
	return func(w http.ResponseWriter, r *http.Request, targetUUID string) {
		ownerUUID, err := httpx.PathUUID(r, "uuid")
		if err != nil {
			httpx.BadRequest(w, "invalid uuid")
			return
		}
		client, err := s.clientForTarget(r.Context(), targetUUID)
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		if err := ops.deleteDraft(client, r.Context(), ownerUUID); err != nil {
			writeClientError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) cicdPrepareCollectionCommit(ops collectionDraftOps) cicdHandler {
	return func(w http.ResponseWriter, r *http.Request, targetUUID string) {
		ownerUUID, err := httpx.PathUUID(r, "uuid")
		if err != nil {
			httpx.BadRequest(w, "invalid uuid")
			return
		}
		client, err := s.clientForTarget(r.Context(), targetUUID)
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		resp, err := ops.prepareCommit(client, r.Context(), ownerUUID)
		if err != nil {
			writeClientError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, resp)
	}
}

func (s *Server) cicdCancelPrepareCollectionCommit(ops collectionDraftOps) cicdHandler {
	return func(w http.ResponseWriter, r *http.Request, targetUUID string) {
		ownerUUID, err := httpx.PathUUID(r, "uuid")
		if err != nil {
			httpx.BadRequest(w, "invalid uuid")
			return
		}
		client, err := s.clientForTarget(r.Context(), targetUUID)
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		if err := ops.cancelPrepare(client, r.Context(), ownerUUID); err != nil {
			writeClientError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) cicdCommitCollectionDraft(ops collectionDraftOps) cicdHandler {
	return func(w http.ResponseWriter, r *http.Request, targetUUID string) {
		ownerUUID, err := httpx.PathUUID(r, "uuid")
		if err != nil {
			httpx.BadRequest(w, "invalid uuid")
			return
		}
		var req teapublisher.EvidenceSubmission
		if err := decodeJSON(r, &req); err != nil {
			httpx.BadRequest(w, "invalid JSON body: "+err.Error())
			return
		}
		client, err := s.clientForTarget(r.Context(), targetUUID)
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		collection, err := ops.commit(client, r.Context(), ownerUUID, req)
		if err != nil {
			writeClientError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, collection)
	}
}

// registerCICDRoutes wires the whole /cicdapi/v1 surface onto mux -- the
// cicd-scoped subset only (design/publisher-service.md §18.11): artifacts
// (create/upload/evidence) and collection-draft mechanics for both owner
// types. Deliberately absent: product/component/CLE creation,
// linkComponent, and approve/reject -- those stay "full"-only, exactly
// mirroring internal/publisher/router.go's own line between the two
// scopes. This exclusion is structural (the routes simply don't exist on
// this mux), not a runtime check.
func (s *Server) registerCICDRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /cicdapi/v1/artifacts", s.requireCICDCredential(s.cicdCreateArtifact))
	mux.HandleFunc("POST /cicdapi/v1/artifacts/{uuid}/{version}/files", s.requireCICDCredential(s.cicdUploadArtifactFile))
	mux.HandleFunc("POST /cicdapi/v1/artifacts/{uuid}/{version}/evidence/prepare", s.requireCICDCredential(s.cicdPrepareArtifactEvidence))
	mux.HandleFunc("POST /cicdapi/v1/artifacts/{uuid}/{version}/evidence", s.requireCICDCredential(s.cicdSubmitArtifactEvidence))

	s.registerCollectionDraftCICDRoutes(mux, "/cicdapi/v1/productReleases/{uuid}/collectionDraft", productReleaseDraftOps)
	s.registerCollectionDraftCICDRoutes(mux, "/cicdapi/v1/componentReleases/{uuid}/collectionDraft", componentReleaseDraftOps)
}

func (s *Server) registerCollectionDraftCICDRoutes(mux *http.ServeMux, base string, ops collectionDraftOps) {
	mux.HandleFunc("PUT "+base, s.requireCICDCredential(s.cicdPutCollectionDraft(ops)))
	mux.HandleFunc("GET "+base, s.requireCICDCredential(s.cicdGetCollectionDraft(ops)))
	mux.HandleFunc("DELETE "+base, s.requireCICDCredential(s.cicdDeleteCollectionDraft(ops)))
	mux.HandleFunc("POST "+base+"/prepareCommit", s.requireCICDCredential(s.cicdPrepareCollectionCommit(ops)))
	mux.HandleFunc("POST "+base+"/cancelPrepare", s.requireCICDCredential(s.cicdCancelPrepareCollectionCommit(ops)))
	mux.HandleFunc("POST "+base+"/commit", s.requireCICDCredential(s.cicdCommitCollectionDraft(ops)))
}

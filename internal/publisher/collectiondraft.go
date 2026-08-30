// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package publisher

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/internal/trust"
	"github.com/oej/opentea/pkg/teapublisher"
)

// putCollectionDraftForOwner implements putCollectionDraft: create-or-
// replace the draft's artifact list (design/publisher-service.md §7.7).
func (s *Server) putCollectionDraftForOwner(ownerType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		if req.Actor == "" {
			httpx.BadRequest(w, "actor is required")
			return
		}
		refs := make([]repo.ArtifactRef, len(req.Artifacts))
		for i, a := range req.Artifacts {
			refs[i] = repo.ArtifactRef{UUID: a.UUID, Version: a.Version}
		}

		draft, err := s.repo.PutCollectionDraft(r.Context(), ownerType, ownerUUID, req.Actor, refs, req.UpdateReason, s.cfg.PublisherDraftTTL)
		writeCollectionDraftResult(w, r, draft, err)
	}
}

// getCollectionDraftForOwner implements getCollectionDraft: review the
// draft, including a diff against the owner's current live collection.
func (s *Server) getCollectionDraftForOwner(ownerType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ownerUUID, err := httpx.PathUUID(r, "uuid")
		if err != nil {
			httpx.BadRequest(w, "invalid uuid")
			return
		}
		draft, err := s.repo.GetCollectionDraft(r.Context(), ownerType, ownerUUID)
		writeCollectionDraftResult(w, r, draft, err)
	}
}

// deleteCollectionDraftForOwner implements deleteCollectionDraft: abandon
// the draft without publishing it.
func (s *Server) deleteCollectionDraftForOwner(ownerType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ownerUUID, err := httpx.PathUUID(r, "uuid")
		if err != nil {
			httpx.BadRequest(w, "invalid uuid")
			return
		}
		err = s.repo.DeleteCollectionDraft(r.Context(), ownerType, ownerUUID)
		if errors.Is(err, repo.ErrNotFound) {
			httpx.NotFound(w)
			return
		}
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// decideCollectionDraftForOwner backs approveCollectionDraft/
// rejectCollectionDraft: record a decision against the draft's current
// content (design/publisher-service.md §7.9). actor must be a human, per
// design/publisher-service.md §14.3 -- not enforced here (no capability
// vocabulary exists yet to distinguish a human-operated "full" credential
// from an automated one, design/publisher-service.md §11 open question
// #10); this operation is "full"-scope only (excluded from "cicd"), which
// is this implementation's structural approximation of that requirement.
func (s *Server) decideCollectionDraftForOwner(ownerType string, approve bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ownerUUID, err := httpx.PathUUID(r, "uuid")
		if err != nil {
			httpx.BadRequest(w, "invalid uuid")
			return
		}
		var req teapublisher.ApprovalDecision
		if err := decodeJSON(r, &req); err != nil {
			httpx.BadRequest(w, "invalid JSON body: "+err.Error())
			return
		}
		if req.Actor == "" {
			httpx.BadRequest(w, "actor is required")
			return
		}

		draft, err := s.repo.DecideCollectionDraft(r.Context(), ownerType, ownerUUID, req.Actor, req.Comment, approve, s.cfg.PublisherApprovalTTL)
		if errors.Is(err, repo.ErrSelfApproval) {
			httpx.Forbidden(w, "actor equals the draft's own draftedBy -- self-approval is rejected")
			return
		}
		writeCollectionDraftResult(w, r, draft, err)
	}
}

// prepareCollectionCommitForOwner implements prepareCollectionCommit:
// Collection Signing part 1 (design/publisher-service.md §7.8) -- requires
// a current, unexpired approval, locks the draft, returns the would-be
// collection and its digest to sign.
func (s *Server) prepareCollectionCommitForOwner(ownerType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ownerUUID, err := httpx.PathUUID(r, "uuid")
		if err != nil {
			httpx.BadRequest(w, "invalid uuid")
			return
		}
		wouldBe, err := s.repo.PrepareCollectionCommit(r.Context(), ownerType, ownerUUID, s.cfg.PublisherLockTTL)
		if errors.Is(err, repo.ErrApprovalRequired) {
			httpx.Conflict(w, "no current approval on record for this draft")
			return
		}
		if errors.Is(err, repo.ErrNotFound) {
			httpx.NotFound(w)
			return
		}
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		digestToSign, err := digestOf(wouldBe)
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, teapublisher.PrepareCommitResponse{
			DigestToSign:      digestToSign,
			WouldBeCollection: wouldBe,
		})
	}
}

// cancelPrepareCollectionCommitForOwner implements
// cancelPrepareCollectionCommit: release the lock without deleting the
// draft.
func (s *Server) cancelPrepareCollectionCommitForOwner(ownerType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ownerUUID, err := httpx.PathUUID(r, "uuid")
		if err != nil {
			httpx.BadRequest(w, "invalid uuid")
			return
		}
		err = s.repo.CancelPrepareCollectionCommit(r.Context(), ownerType, ownerUUID)
		if errors.Is(err, repo.ErrNotFound) || errors.Is(err, repo.ErrLockNotHeld) {
			httpx.NotFound(w)
			return
		}
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// commitCollectionDraftForOwner implements commitCollectionDraft:
// Collection Signing part 2 + Commit, atomic. Re-derives the exact digest
// prepareCollectionCommit returned (never trusts a cached value), verifies
// the submitted signature, then persists via repo.CommitCollectionDraft --
// mirrors submitArtifactEvidence's verify-before-store sequence exactly,
// over the would-be collection instead of an artifact.
func (s *Server) commitCollectionDraftForOwner(ownerType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		if req.ObjectDigestValue == "" {
			httpx.BadRequest(w, "objectDigestValue is required")
			return
		}
		if !validSignatureFormats[req.SignatureFormat] {
			httpx.BadRequest(w, "signatureFormat: only \"jws-detached\" is implemented in this phase")
			return
		}
		if req.SignatureValue == "" {
			httpx.BadRequest(w, "signatureValue is required")
			return
		}
		if req.CertificatePEM == "" {
			httpx.BadRequest(w, "certificatePem is required")
			return
		}

		digestBytes, err := hex.DecodeString(req.ObjectDigestValue)
		if err != nil {
			httpx.BadRequest(w, "objectDigestValue must be hex-encoded")
			return
		}
		sigBytes, err := base64.StdEncoding.DecodeString(req.SignatureValue)
		if err != nil {
			httpx.BadRequest(w, "signatureValue must be base64-encoded")
			return
		}

		wouldBe, err := s.repo.PeekCollectionDraftCommit(r.Context(), ownerType, ownerUUID)
		if errors.Is(err, repo.ErrApprovalRequired) {
			httpx.Conflict(w, "no current approval on record for this draft")
			return
		}
		if errors.Is(err, repo.ErrNotFound) || errors.Is(err, repo.ErrLockNotHeld) {
			httpx.NotFound(w)
			return
		}
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		computed, err := digestOf(wouldBe)
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		if !strings.EqualFold(computed, req.ObjectDigestValue) {
			httpx.BadRequest(w, "objectDigestValue does not match the server-recomputed digest -- the draft changed since prepareCollectionCommit was called")
			return
		}

		pub, err := trust.ParseCertificatePublicKey(req.CertificatePEM, time.Now())
		if err != nil {
			httpx.BadRequest(w, "certificatePem: "+err.Error())
			return
		}
		if !trust.Verify(pub, digestBytes, sigBytes) {
			httpx.BadRequest(w, "signatureValue does not verify against certificatePem and objectDigestValue")
			return
		}
		fingerprint, trustDomain, err := trust.ParseCertificateSubject(req.CertificatePEM, pub)
		if err != nil {
			httpx.BadRequest(w, "certificatePem: "+err.Error())
			return
		}

		collection, err := s.repo.CommitCollectionDraft(r.Context(), ownerType, ownerUUID, repo.CommitEvidenceInput{
			ObjectMediaType:        req.ObjectMediaType,
			ObjectLocation:         req.ObjectLocation,
			ObjectDigestValue:      req.ObjectDigestValue,
			SignatureFormat:        req.SignatureFormat,
			SignatureValue:         req.SignatureValue,
			CertificateFormat:      "x509-pem",
			CertificateValue:       req.CertificatePEM,
			CertificateFingerprint: string(fingerprint),
			CertificateTrustDomain: string(trustDomain),
		})
		if errors.Is(err, repo.ErrFingerprintReused) {
			httpx.BadRequest(w, "certificate fingerprint has already been used by another evidence bundle")
			return
		}
		if errors.Is(err, repo.ErrApprovalRequired) {
			httpx.Conflict(w, "no current approval on record for this draft")
			return
		}
		if errors.Is(err, repo.ErrNotFound) || errors.Is(err, repo.ErrLockNotHeld) {
			httpx.NotFound(w)
			return
		}
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, collection)
	}
}

// digestOf computes the hex SHA-256 digest of c's RFC 8785 canonical form
// -- sign exactly these bytes (shared by prepareCollectionCommit's
// response and commitCollectionDraft's re-derivation of it).
func digestOf(c any) (string, error) {
	canonical, err := trust.Canonicalize(c)
	if err != nil {
		return "", err
	}
	return trust.SHA256Hex(canonical), nil
}

// writeCollectionDraftResult maps the shared put/get/approve/reject error
// vocabulary (repo.ErrNotFound/ErrDraftLocked) to the responses
// design/publisher-openapi.yaml documents for each of those operations.
func writeCollectionDraftResult(w http.ResponseWriter, r *http.Request, draft teapublisher.CollectionDraft, err error) {
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if errors.Is(err, repo.ErrDraftLocked) {
		httpx.Conflict(w, "the draft is locked by an outstanding prepareCollectionCommit -- call cancelPrepare first, or wait for the pending commit to complete")
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, draft)
}

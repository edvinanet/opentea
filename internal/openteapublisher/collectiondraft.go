// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// collectiondraft.go: one product release's whole collection-draft
// lifecycle (design/publisher-service.md §18.7/§18.9/§18.10) -- the
// release detail page (current live collection + draft panel), adding/
// removing an artifact reference from the draft, protocol-level approve/
// reject, and the single "Sign & Publish" action. productRelease only;
// componentRelease is the same shape throughout (see
// internal/openteapublisher/cicdapi.go's own collectionDraftOps dispatch
// table for the precedent) but deliberately not built in this pass.
package openteapublisher

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/oej/opentea/internal/trust"
	"github.com/oej/opentea/pkg/teapublisher"
	"github.com/oej/opentea/pkg/teapublisherclient"
)

// collectionDraftBasePath builds the URL prefix every handler in this
// file redirects/re-renders against.
func collectionDraftBasePath(targetUUID, releaseUUID string) string {
	return "/targets/" + targetUUID + "/productReleases/" + releaseUUID
}

func (s *Server) productReleasePage(w http.ResponseWriter, r *http.Request, staff Staff) {
	targetUUID := r.PathValue("targetUuid")
	releaseUUID := r.PathValue("uuid")
	target, err := s.repo.GetTarget(r.Context(), targetUUID)
	if errors.Is(err, ErrNotFound) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	data, err := s.loadProductReleaseData(r, staff, target, releaseUUID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.renderAuthenticated(w, "productrelease", data)
}

// loadProductReleaseData fetches everything productrelease.html needs:
// the release-with-collection (read side) and its current draft, if any
// (write side -- GetProductReleaseCollectionDraft needs a bearer
// credential, so this isn't a teaclient-only read). A 404 on the draft
// fetch means no draft has been started yet, not an error -- rendered as
// an empty teapublisher.CollectionDraft{}, matching putProductReleaseDraft's
// own "0 meaning no draft exists yet" revision convention.
func (s *Server) loadProductReleaseData(r *http.Request, staff Staff, target Target, releaseUUID string) (pageData, error) {
	readClient, err := s.teaClientForTarget(r.Context(), target.UUID)
	if err != nil {
		return pageData{}, err
	}
	release, err := readClient.GetProductReleaseWithCollection(r.Context(), releaseUUID)
	if err != nil {
		return pageData{}, err
	}

	writeClient, err := s.teaPublisherClientForTarget(r.Context(), target.UUID)
	if err != nil {
		return pageData{}, err
	}
	draft, err := writeClient.GetProductReleaseCollectionDraft(r.Context(), releaseUUID)
	if err != nil && !teapublisherclient.IsNotFound(err) {
		return pageData{}, err
	}

	return pageData{
		Staff: staff, Target: target, ProductRelease: release, Draft: draft,
		CanApprove: staff.WorkflowRole == StaffWorkflowRoleSecurityComplianceApprover,
		IsDrafter:  draft.DraftedBy != "" && draft.DraftedBy == staff.Username,
	}, nil
}

func (s *Server) rerenderProductReleaseWithError(w http.ResponseWriter, r *http.Request, staff Staff, target Target, releaseUUID, message string) {
	data, err := s.loadProductReleaseData(r, staff, target, releaseUUID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	data.Error = message
	s.renderAuthenticated(w, "productrelease", data)
}

// updateCollectionDraftForm adds or removes exactly one artifact
// reference: fetches the current draft (an absent draft is treated as an
// empty one, revision 0), applies the one change, then PUTs the full list
// back with ExpectedRevision set to what was just read -- an optimistic-
// concurrency courtesy against a *different* caller's unseen edit
// (teapublisher.CollectionDraftArtifactList's own doc comment), not a
// substitute for real idempotency. On a 409 (ExpectedRevision stale),
// this does not retry -- silently retrying would risk clobbering
// whatever the other caller just wrote, exactly what ExpectedRevision
// exists to prevent; instead it re-renders with an error and the fresh
// state.
func (s *Server) updateCollectionDraftForm(w http.ResponseWriter, r *http.Request, staff Staff) {
	targetUUID := r.PathValue("targetUuid")
	releaseUUID := r.PathValue("uuid")
	target, err := s.repo.GetTarget(r.Context(), targetUUID)
	if errors.Is(err, ErrNotFound) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, collectionDraftBasePath(targetUUID, releaseUUID), http.StatusSeeOther)
		return
	}
	action := r.PostFormValue("action")
	artifactUUID := r.PostFormValue("artifactUuid")
	versionStr := r.PostFormValue("artifactVersion")
	version, convErr := strconv.Atoi(versionStr)
	if (action != "add" && action != "remove") || artifactUUID == "" || convErr != nil || version < 1 {
		s.rerenderProductReleaseWithError(w, r, staff, target, releaseUUID, "a valid action, artifact UUID, and artifact version are required")
		return
	}

	client, err := s.teaPublisherClientForTarget(r.Context(), targetUUID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	current, err := client.GetProductReleaseCollectionDraft(r.Context(), releaseUUID)
	if err != nil && !teapublisherclient.IsNotFound(err) {
		s.rerenderProductReleaseWithError(w, r, staff, target, releaseUUID, decodeAPIErrorMessage(err))
		return
	}

	artifacts := applyArtifactChange(current.Artifacts, action, artifactUUID, version)
	expectedRevision := current.Revision
	_, err = client.PutProductReleaseCollectionDraft(r.Context(), releaseUUID, teapublisher.CollectionDraftArtifactList{
		Actor:            staff.Username,
		Artifacts:        artifacts,
		ExpectedRevision: &expectedRevision,
	})
	if teapublisherclient.IsConflict(err) {
		s.rerenderProductReleaseWithError(w, r, staff, target, releaseUUID,
			"the draft changed since this page loaded -- reload and try again rather than risk overwriting that edit")
		return
	}
	if err != nil {
		s.rerenderProductReleaseWithError(w, r, staff, target, releaseUUID, decodeAPIErrorMessage(err))
		return
	}
	http.Redirect(w, r, collectionDraftBasePath(targetUUID, releaseUUID), http.StatusSeeOther)
}

// applyArtifactChange returns a new slice with the one requested change
// applied -- "add" appends (no de-duplication beyond what the target's
// own CollectionDraftArtifactList accepts; adding the same reference
// twice is the caller's mistake to notice, not silently corrected here),
// "remove" drops the first matching (uuid, version) pair.
func applyArtifactChange(current []teapublisher.ArtifactVersionRef, action, artifactUUID string, version int) []teapublisher.ArtifactVersionRef {
	switch action {
	case "add":
		return append(current, teapublisher.ArtifactVersionRef{UUID: artifactUUID, Version: version})
	case "remove":
		out := make([]teapublisher.ArtifactVersionRef, 0, len(current))
		removed := false
		for _, a := range current {
			if !removed && a.UUID == artifactUUID && a.Version == version {
				removed = true
				continue
			}
			out = append(out, a)
		}
		return out
	default:
		return current
	}
}

// decideCollectionDraftForm backs approveCollectionDraftForm/
// rejectCollectionDraftForm: identical shape, differing only in which
// client method and which redirect-safe outcome is right. Gated by
// requireApprovalRole (router.go) -- closing design/publisher-service.md
// §18.9's own named gap ("nothing today limits which staff members may
// approve this, protocol-level, decision specifically... reusing
// security_compliance_approver here too is the obvious next step").
func (s *Server) decideCollectionDraftForm(approve bool) staffHandler {
	return func(w http.ResponseWriter, r *http.Request, staff Staff) {
		targetUUID := r.PathValue("targetUuid")
		releaseUUID := r.PathValue("uuid")
		target, err := s.repo.GetTarget(r.Context(), targetUUID)
		if errors.Is(err, ErrNotFound) {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Redirect(w, r, collectionDraftBasePath(targetUUID, releaseUUID), http.StatusSeeOther)
			return
		}
		comment := r.PostFormValue("comment")

		client, err := s.teaPublisherClientForTarget(r.Context(), targetUUID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		decision := teapublisher.ApprovalDecision{Actor: staff.Username, Comment: comment}
		var decErr error
		if approve {
			_, decErr = client.ApproveProductReleaseCollectionDraft(r.Context(), releaseUUID, decision)
		} else {
			_, decErr = client.RejectProductReleaseCollectionDraft(r.Context(), releaseUUID, decision)
		}
		if decErr != nil {
			s.rerenderProductReleaseWithError(w, r, staff, target, releaseUUID, decodeAPIErrorMessage(decErr))
			return
		}
		http.Redirect(w, r, collectionDraftBasePath(targetUUID, releaseUUID), http.StatusSeeOther)
	}
}

// ephemeralKeyValidity bounds the ephemeral signing key signAndPublishForm
// generates -- it's used and destroyed within this single request, so a
// short validity is all it ever needs; matches internal/trust's own
// MaxKeyValidity ceiling rather than requesting the maximum for no reason.
const ephemeralKeyValidity = 5 * time.Minute

// signAndPublishForm is design/publisher-service.md §18.10's one "Sign &
// Publish" action: prepare, generate a fresh ephemeral Ed25519 key +
// self-signed certificate, sign the returned digest, commit, and discard
// the key -- all within this one request, matching v1's ephemeral-only
// signing model (§17.5). No separate download-digest/upload-signature
// ceremony (that's only needed for §9.5's Web PKI/HSM/air-gapped mode,
// out of scope for v1). Gated by requireSession (router.go), not
// requireApprovalRole -- the target's own approval/lock checks are the
// real authority (§18.9/§18.10: the GUI's job is UX, not the enforcement
// boundary); the template disables the button client-side unless
// draft.Approval.Status == "approved" as a courtesy only.
func (s *Server) signAndPublishForm(w http.ResponseWriter, r *http.Request, staff Staff) {
	targetUUID := r.PathValue("targetUuid")
	releaseUUID := r.PathValue("uuid")
	target, err := s.repo.GetTarget(r.Context(), targetUUID)
	if errors.Is(err, ErrNotFound) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	client, err := s.teaPublisherClientForTarget(r.Context(), targetUUID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	prepared, err := client.PrepareProductReleaseCollectionCommit(r.Context(), releaseUUID)
	if err != nil {
		s.rerenderProductReleaseWithError(w, r, staff, target, releaseUUID, decodeAPIErrorMessage(err))
		return
	}
	digestBytes, err := hex.DecodeString(prepared.DigestToSign)
	if err != nil {
		s.rerenderProductReleaseWithError(w, r, staff, target, releaseUUID, "target returned a malformed digest: "+err.Error())
		return
	}

	kp, err := trust.GenerateEphemeralKey(ephemeralKeyValidity)
	if err != nil {
		s.rerenderProductReleaseWithError(w, r, staff, target, releaseUUID, "could not generate a signing key: "+err.Error())
		return
	}
	defer kp.Destroy()

	certPEM, err := trust.BuildCertificate(kp, s.trustDomain())
	if err != nil {
		s.rerenderProductReleaseWithError(w, r, staff, target, releaseUUID, "could not build a signing certificate: "+err.Error())
		return
	}
	sig := trust.Sign(kp.Private, digestBytes)

	_, err = client.CommitProductReleaseCollectionDraft(r.Context(), releaseUUID, teapublisher.EvidenceSubmission{
		ObjectDigestValue: prepared.DigestToSign,
		SignatureFormat:   teapublisher.SignatureFormatJWSDetached,
		SignatureValue:    base64.StdEncoding.EncodeToString(sig),
		CertificatePEM:    certPEM,
	})
	if err != nil {
		s.rerenderProductReleaseWithError(w, r, staff, target, releaseUUID, decodeAPIErrorMessage(err))
		return
	}
	http.Redirect(w, r, collectionDraftBasePath(targetUUID, releaseUUID), http.StatusSeeOther)
}

// trustDomain derives this deployment's trust.TrustDomain from Config.RootURL's
// hostname -- a real, deployment-specific value (the certificate's SAN is
// "<fingerprint>.<trustDomain>", internal/trust.TrustDomain's own doc
// comment) rather than a hardcoded placeholder. DNS trust-anchor
// publication for it is a later phase (design/publisher-service.md §16),
// not needed for signing to work correctly now.
func (s *Server) trustDomain() trust.TrustDomain {
	host := s.cfg.RootURL
	if u, err := url.Parse(s.cfg.RootURL); err == nil && u.Hostname() != "" {
		host = u.Hostname()
	}
	return trust.TrustDomain(host)
}

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

// validSignatureFormats mirrors internal/admin/evidencebundle.go's own --
// see its doc comment for why only jws-detached is actually accepted:
// verification below only ever does one thing (raw Ed25519 over the
// digest bytes), so accepting a label this handler doesn't really verify
// would let a caller store evidence mislabeled as a format it isn't.
var validSignatureFormats = map[string]bool{
	string(trust.SignatureFormatJWSDetached): true,
}

// prepareArtifactEvidence implements prepareArtifactEvidence: returns the
// current artifact and the digest to sign over it. Callable repeatedly --
// reflects current state each time, locks nothing (design/publisher-openapi.yaml).
func (s *Server) prepareArtifactEvidence(w http.ResponseWriter, r *http.Request) {
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

	artifact, err := s.repo.GetArtifactByVersion(r.Context(), uuid, version)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}

	canonical, err := trust.Canonicalize(artifact)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, teapublisher.PrepareArtifactEvidenceResponse{
		DigestToSign: trust.SHA256Hex(canonical),
		Artifact:     artifact,
	})
}

// submitArtifactEvidence implements submitArtifactEvidence: verify-before-
// store, mirroring internal/admin/evidencebundle.go's
// createEvidenceBundleForOwner("ARTIFACT", ...) exactly (digest re-derived
// server-side and checked against the submission, signature verified
// against the certificate before anything is persisted) -- the CI/CD-usable,
// GUI-independent operation design/publisher-service.md §10.4/§14.3 names
// explicitly. CertificateChain is accepted but not yet used (opentea Phase 1
// only verifies self-signed leaf certificates, same restriction
// internal/admin/evidencebundle.go documents).
func (s *Server) submitArtifactEvidence(w http.ResponseWriter, r *http.Request) {
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

	artifact, err := s.repo.GetArtifactByVersion(r.Context(), uuid, version)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	canonical, err := trust.Canonicalize(artifact)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if computed := trust.SHA256Hex(canonical); !strings.EqualFold(computed, req.ObjectDigestValue) {
		httpx.BadRequest(w, "objectDigestValue does not match the server-computed digest of the target artifact -- it may have changed since prepareArtifactEvidence was called")
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

	bundle, err := s.repo.CreateEvidenceBundle(r.Context(), repo.EvidenceBundleInput{
		OwnerType:              "ARTIFACT",
		OwnerUUID:              uuid,
		OwnerVersion:           version,
		ObjectType:             "artifact",
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
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if errors.Is(err, repo.ErrFingerprintReused) {
		httpx.BadRequest(w, "certificate fingerprint has already been used by another evidence bundle")
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, bundle)
}

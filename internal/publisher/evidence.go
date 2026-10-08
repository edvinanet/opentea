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
// would let a caller store evidence mislabeled as a format it isn't. The
// exact bytes that "one thing" covers -- and why jws-detached here is a
// restricted profile, not real RFC 7797 JWS -- are now pinned down
// precisely, with a published byte-exact test vector, in
// design/publisher-service.md §9.7 (docs/security-review-publisher-design-260828.md
// finding 5).
var validSignatureFormats = map[string]bool{
	string(trust.SignatureFormatJWSDetached): true,
}

// prepareArtifactEvidence implements prepareArtifactEvidence: returns the
// current artifact and the digest to sign over it. Callable repeatedly --
// reflects current state each time, locks nothing (design/publisher-openapi.yaml).
func (s *Server) prepareArtifactEvidence(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		badRequest(w, r, teapublisher.ErrorInvalidPathParameter, "invalid uuid")
		return
	}
	version, err := httpx.PathPositiveInt(r, "version")
	if err != nil {
		badRequest(w, r, teapublisher.ErrorInvalidPathParameter, "invalid version")
		return
	}

	artifact, err := s.repo.GetArtifactByVersion(r.Context(), uuid, version)
	if errors.Is(err, repo.ErrNotFound) {
		notFoundErr(w, r)
		return
	}
	if err != nil {
		internalErr(w, r, err)
		return
	}

	canonical, err := trust.Canonicalize(artifact)
	if err != nil {
		internalErr(w, r, err)
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
		badRequest(w, r, teapublisher.ErrorInvalidPathParameter, "invalid uuid")
		return
	}
	version, err := httpx.PathPositiveInt(r, "version")
	if err != nil {
		badRequest(w, r, teapublisher.ErrorInvalidPathParameter, "invalid version")
		return
	}

	var req teapublisher.EvidenceSubmission
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, r, teapublisher.ErrorInvalidRequestBody, "invalid JSON body: "+err.Error())
		return
	}
	if req.ObjectDigestValue == "" {
		badRequest(w, r, teapublisher.ErrorMissingField, "objectDigestValue is required", teapublisher.FieldError{Field: "objectDigestValue", Message: "is required"})
		return
	}
	// Unsupported signature format -- one of finding 15's four named
	// "indistinguishable 400s" (docs/security-review-publisher-design-260828.md),
	// now ErrorUnsupportedSignatureFormat instead of a generic 400.
	if !validSignatureFormats[req.SignatureFormat] {
		badRequest(w, r, teapublisher.ErrorUnsupportedSignatureFormat, "signatureFormat: only \"jws-detached\" is implemented in this phase", teapublisher.FieldError{Field: "signatureFormat", Message: "only \"jws-detached\" is implemented in this phase"})
		return
	}
	if req.SignatureValue == "" {
		badRequest(w, r, teapublisher.ErrorMissingField, "signatureValue is required", teapublisher.FieldError{Field: "signatureValue", Message: "is required"})
		return
	}
	if req.CertificatePEM == "" {
		badRequest(w, r, teapublisher.ErrorMissingField, "certificatePem is required", teapublisher.FieldError{Field: "certificatePem", Message: "is required"})
		return
	}

	digestBytes, err := hex.DecodeString(req.ObjectDigestValue)
	if err != nil {
		badRequest(w, r, teapublisher.ErrorInvalidField, "objectDigestValue must be hex-encoded", teapublisher.FieldError{Field: "objectDigestValue", Message: "must be hex-encoded"})
		return
	}
	sigBytes, err := base64.StdEncoding.DecodeString(req.SignatureValue)
	if err != nil {
		badRequest(w, r, teapublisher.ErrorInvalidField, "signatureValue must be base64-encoded", teapublisher.FieldError{Field: "signatureValue", Message: "must be base64-encoded"})
		return
	}

	artifact, err := s.repo.GetArtifactByVersion(r.Context(), uuid, version)
	if errors.Is(err, repo.ErrNotFound) {
		notFoundErr(w, r)
		return
	}
	if err != nil {
		internalErr(w, r, err)
		return
	}
	canonical, err := trust.Canonicalize(artifact)
	if err != nil {
		internalErr(w, r, err)
		return
	}
	// Digest mismatch -- finding 15's "stale prepare" case: the artifact
	// changed since prepareArtifactEvidence returned this digest.
	if computed := trust.SHA256Hex(canonical); !strings.EqualFold(computed, req.ObjectDigestValue) {
		badRequest(w, r, teapublisher.ErrorDigestMismatch, "objectDigestValue does not match the server-computed digest of the target artifact -- it may have changed since prepareArtifactEvidence was called", teapublisher.FieldError{Field: "objectDigestValue", Message: "does not match the server-computed digest"})
		return
	}

	// Invalid certificate -- finding 15's third named case.
	pub, err := trust.ParseCertificatePublicKey(req.CertificatePEM, time.Now())
	if err != nil {
		badRequest(w, r, teapublisher.ErrorCertificateInvalid, "certificatePem: "+err.Error(), teapublisher.FieldError{Field: "certificatePem", Message: err.Error()})
		return
	}
	// Signature mismatch -- finding 15's fourth named case.
	if !trust.Verify(pub, digestBytes, sigBytes) {
		badRequest(w, r, teapublisher.ErrorSignatureInvalid, "signatureValue does not verify against certificatePem and objectDigestValue", teapublisher.FieldError{Field: "signatureValue", Message: "does not verify against certificatePem and objectDigestValue"})
		return
	}
	fingerprint, trustDomain, err := trust.ParseCertificateSubject(req.CertificatePEM, pub)
	if err != nil {
		badRequest(w, r, teapublisher.ErrorCertificateInvalid, "certificatePem: "+err.Error(), teapublisher.FieldError{Field: "certificatePem", Message: err.Error()})
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
		notFoundErr(w, r)
		return
	}
	if errors.Is(err, repo.ErrFingerprintReused) {
		badRequest(w, r, teapublisher.ErrorFingerprintReused, "certificate fingerprint has already been used by another evidence bundle")
		return
	}
	if err != nil {
		internalErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, bundle)
}

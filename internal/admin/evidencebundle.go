// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package admin

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
)

// createEvidenceBundleRequest accepts exactly one of Evidence (raw signing
// material this server verifies and stores) or Ref (a pointer to a bundle
// hosted elsewhere) -- see validate().
type createEvidenceBundleRequest struct {
	Evidence *evidenceMaterial          `json:"evidence,omitempty"`
	Ref      *evidenceBundleRefMaterial `json:"evidenceBundleRef,omitempty"`
}

// evidenceMaterial is the raw signing material for an evidence bundle this
// server holds locally. ObjectDigestValue is the hex SHA-256 digest of the
// object bytes that were signed -- and MUST equal this server's own
// SHA-256(Canonicalize(...)) of the actual current tea.Artifact/
// tea.Collection identified by the request path (checked in
// createEvidenceBundleForOwner before verifying the signature); a
// submission whose digest doesn't match is rejected, not just one whose
// signature doesn't verify. This is what makes the signature mean
// something about *this* object rather than merely being self-consistent
// with an arbitrary caller-chosen digest. SignatureValue is the
// base64-encoded Ed25519 signature computed directly over those digest
// bytes (not the object bytes themselves -- this lets the server verify
// without needing the, potentially large, object bytes on hand a second
// time, the same reasoning the schema's own objectRef.location field
// anticipates for *fetching* the object, just not for what gets signed).
type evidenceMaterial struct {
	ObjectDigestValue string `json:"objectDigestValue"`
	ObjectMediaType   string `json:"objectMediaType,omitempty"`
	ObjectLocation    string `json:"objectLocation,omitempty"`
	SignatureFormat   string `json:"signatureFormat"`
	SignatureValue    string `json:"signatureValue"`
	CertificatePEM    string `json:"certificatePem"`
}

type evidenceBundleRefMaterial struct {
	URI         string `json:"uri"`
	DigestValue string `json:"digestValue"`
}

// validSignatureFormats is the set this API actually accepts -- narrower
// than internal/trust.SignatureFormat's full closed vocabulary
// (cms-detached/dsse-envelope/jws-detached/cose-sign1) and the DB CHECK
// constraint in 0006_trust.sql, both of which stay broader on purpose so a
// later phase adding real CMS/DSSE/COSE support doesn't need a migration.
// This map is the live security boundary: verification below only ever
// does one thing (decode SignatureValue as raw base64 and check it as a
// raw Ed25519 signature over the digest bytes) -- it does not parse a real
// CMS/DSSE/COSE envelope structure for any of those labels. Accepting a
// label this handler doesn't actually verify as that format would let a
// caller store evidence mislabeled as (say) "cms-detached" while
// containing no CMS structure at all, misleading any consumer that trusts
// the label (found by external security review). Widen this only in step
// with adding a real per-format parser/verifier below.
var validSignatureFormats = map[string]bool{
	string(trust.SignatureFormatJWSDetached): true,
}

func (req createEvidenceBundleRequest) validate() string {
	if (req.Evidence == nil) == (req.Ref == nil) {
		return "exactly one of \"evidence\" or \"evidenceBundleRef\" is required"
	}
	if req.Evidence != nil {
		e := req.Evidence
		if e.ObjectDigestValue == "" {
			return "evidence.objectDigestValue is required"
		}
		if !validSignatureFormats[e.SignatureFormat] {
			return "evidence.signatureFormat: only \"jws-detached\" is implemented in this phase"
		}
		if e.SignatureValue == "" {
			return "evidence.signatureValue is required"
		}
		if e.CertificatePEM == "" {
			return "evidence.certificatePem is required"
		}
	}
	if req.Ref != nil {
		if req.Ref.URI == "" {
			return "evidenceBundleRef.uri is required"
		}
		if req.Ref.DigestValue == "" {
			return "evidenceBundleRef.digestValue is required"
		}
	}
	return ""
}

// createEvidenceBundleForOwner returns a handler that attaches an evidence
// bundle to ownerType ("ARTIFACT" or "COLLECTION"), reading the owner's
// uuid/version from the "uuid"/"version" path values. Shared by
// createArtifactEvidenceBundle and createCollectionEvidenceBundle since the
// request/verification/persistence logic is identical -- only the owner
// type and TEA object type differ.
func (s *Server) createEvidenceBundleForOwner(ownerType string, objectType trust.ObjectType) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ownerUUID, err := httpx.PathUUID(r, "uuid")
		if err != nil {
			httpx.BadRequest(w, "invalid uuid")
			return
		}
		ownerVersion, err := httpx.PathPositiveInt(r, "version")
		if err != nil {
			httpx.BadRequest(w, "invalid version")
			return
		}

		var req createEvidenceBundleRequest
		if err := decodeJSON(r, &req); err != nil {
			httpx.BadRequest(w, "invalid JSON body: "+err.Error())
			return
		}
		if msg := req.validate(); msg != "" {
			httpx.BadRequest(w, msg)
			return
		}

		actor := actorFromContext(r.Context())

		if req.Ref != nil {
			var createdRef any
			err := s.repo.WithTx(r.Context(), func(tx *repo.Repo) error {
				ref, err := tx.CreateEvidenceBundleRef(r.Context(), repo.EvidenceBundleRefInput{
					OwnerType:    ownerType,
					OwnerUUID:    ownerUUID,
					OwnerVersion: ownerVersion,
					URI:          req.Ref.URI,
					DigestValue:  req.Ref.DigestValue,
				})
				if err != nil {
					return err
				}
				createdRef = ref
				return auditWrite(r.Context(), tx, actor.UUID, requestIDFrom(r), "evidenceBundle.createRef", "evidence_bundle", ownerUUID, nil, ref)
			})
			if errors.Is(err, repo.ErrNotFound) {
				httpx.NotFound(w)
				return
			}
			if err != nil {
				httpx.InternalError(w, r, err)
				return
			}
			httpx.WriteJSON(w, http.StatusCreated, createdRef)
			return
		}

		e := req.Evidence

		digestBytes, err := hex.DecodeString(e.ObjectDigestValue)
		if err != nil {
			httpx.BadRequest(w, "evidence.objectDigestValue must be hex-encoded")
			return
		}
		sigBytes, err := base64.StdEncoding.DecodeString(e.SignatureValue)
		if err != nil {
			httpx.BadRequest(w, "evidence.signatureValue must be base64-encoded")
			return
		}

		// The submitted digest must equal this server's own computed digest
		// of the real target object -- otherwise a caller could sign
		// arbitrary self-consistent bytes and attach the result to any
		// artifact/collection without the signature proving anything about
		// that object's actual content. (Found by external security review:
		// the signature was previously verified only against whatever
		// digest the caller supplied, never checked against the real
		// object -- so evidence could be internally valid while attesting
		// to nothing.) This is itself part of the verify-before-store
		// requirement below, not a separate concern -- a digest is
		// "submitted evidence" too.
		owner, err := s.repo.GetEvidenceOwnerObject(r.Context(), ownerType, ownerUUID, ownerVersion)
		if errors.Is(err, repo.ErrNotFound) {
			httpx.NotFound(w)
			return
		}
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		canonicalOwner, err := trust.Canonicalize(owner)
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		if computed := trust.SHA256Hex(canonicalOwner); !strings.EqualFold(computed, e.ObjectDigestValue) {
			httpx.BadRequest(w, "evidence.objectDigestValue does not match the server-computed digest of the target object")
			return
		}

		// Verify BEFORE any repo call -- tea-trust-architecture
		// 08-evidence-bundle.md Sec 5.4/15.3: a publisher API MUST verify
		// submitted evidence and MUST NOT store it if verification fails.
		pub, err := trust.ParseCertificatePublicKey(e.CertificatePEM, time.Now())
		if err != nil {
			httpx.BadRequest(w, "evidence.certificatePem: "+err.Error())
			return
		}
		if !trust.Verify(pub, digestBytes, sigBytes) {
			httpx.BadRequest(w, "evidence.signatureValue does not verify against evidence.certificatePem and evidence.objectDigestValue")
			return
		}
		fingerprint, trustDomain, err := trust.ParseCertificateSubject(e.CertificatePEM, pub)
		if err != nil {
			httpx.BadRequest(w, "evidence.certificatePem: "+err.Error())
			return
		}

		var created any
		err = s.repo.WithTx(r.Context(), func(tx *repo.Repo) error {
			bundle, err := tx.CreateEvidenceBundle(r.Context(), repo.EvidenceBundleInput{
				OwnerType:              ownerType,
				OwnerUUID:              ownerUUID,
				OwnerVersion:           ownerVersion,
				ObjectType:             string(objectType),
				ObjectMediaType:        e.ObjectMediaType,
				ObjectLocation:         e.ObjectLocation,
				ObjectDigestValue:      e.ObjectDigestValue,
				SignatureFormat:        e.SignatureFormat,
				SignatureValue:         e.SignatureValue,
				CertificateFormat:      "x509-pem",
				CertificateValue:       e.CertificatePEM,
				CertificateFingerprint: string(fingerprint),
				CertificateTrustDomain: string(trustDomain),
			})
			if err != nil {
				return err
			}
			created = bundle
			return auditWrite(r.Context(), tx, actor.UUID, requestIDFrom(r), "evidenceBundle.create", "evidence_bundle", bundle.UUID, nil, bundle)
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
		httpx.WriteJSON(w, http.StatusCreated, created)
	}
}

func (s *Server) getEvidenceBundle(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	bundle, err := s.repo.GetEvidenceBundle(r.Context(), uuid)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, bundle)
}

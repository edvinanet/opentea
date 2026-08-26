package admin

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
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
// object bytes that were signed; SignatureValue is the base64-encoded
// Ed25519 signature computed directly over those digest bytes (not the
// object bytes themselves -- this lets the server verify without needing
// the, potentially large or externally-hosted, object bytes on hand, the
// same reasoning the schema's own objectRef.location field already
// anticipates).
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

var validSignatureFormats = map[string]bool{
	"cms-detached": true, "dsse-envelope": true, "jws-detached": true, "cose-sign1": true,
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
			return "evidence.signatureFormat must be a valid signature-format enum value"
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
		fingerprint, trustDomain, err := trust.ParseCertificateSubject(e.CertificatePEM)
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

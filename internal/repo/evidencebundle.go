package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/oej/opentea/internal/idgen"
	"github.com/oej/opentea/pkg/tea"
)

// ErrFingerprintReused is returned by CreateEvidenceBundle when
// in.CertificateFingerprint has already been registered by an earlier
// evidence bundle -- tea-trust-architecture 01-tea-trust-architecture.md
// Sec 9 requires reuse of a known fingerprint be rejected, since ephemeral
// signing keys must never be reused across signing events.
var ErrFingerprintReused = errors.New("repo: certificate fingerprint already used")

// EvidenceBundleInput carries everything needed to create one evidence
// bundle. Owner{Type,UUID,Version} identifies the artifact or collection
// this bundle covers; the caller (internal/admin) is responsible for having
// already verified Signature/Certificate against ObjectDigestValue via
// internal/trust before calling this -- CreateEvidenceBundle persists, it
// does not verify.
type EvidenceBundleInput struct {
	OwnerType              string // "ARTIFACT" or "COLLECTION"
	OwnerUUID              string
	OwnerVersion           int
	ObjectType             string
	ObjectMediaType        string
	ObjectLocation         string
	ObjectDigestValue      string
	SignatureFormat        string
	SignatureValue         string
	SignatureDigestValue   string
	CertificateFormat      string
	CertificateValue       string
	CertificateFingerprint string
	CertificateTrustDomain string
}

// CreateEvidenceBundle registers in.CertificateFingerprint (rejecting reuse
// with ErrFingerprintReused) and inserts the bundle row in one transaction
// -- registration and creation must succeed or fail together, since a
// bundle referencing an unregistered fingerprint would defeat the reuse
// check entirely. Returns ErrNotFound if the owning artifact/collection
// (OwnerType, OwnerUUID, OwnerVersion) doesn't exist.
func (r *Repo) CreateEvidenceBundle(ctx context.Context, in EvidenceBundleInput) (tea.EvidenceBundle, error) {
	return runInTx(ctx, r, func(tx dbtx) (tea.EvidenceBundle, error) {
		exists, err := ownerExistsTx(ctx, tx, in.OwnerType, in.OwnerUUID, in.OwnerVersion)
		if err != nil {
			return tea.EvidenceBundle{}, err
		}
		if !exists {
			return tea.EvidenceBundle{}, ErrNotFound
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO used_fingerprint (fingerprint, trust_domain) VALUES (?, ?)`,
			in.CertificateFingerprint, in.CertificateTrustDomain,
		); err != nil {
			if isUniqueConstraintError(err) {
				return tea.EvidenceBundle{}, ErrFingerprintReused
			}
			return tea.EvidenceBundle{}, err
		}

		id := idgen.New()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO evidence_bundle (
				uuid, owner_type, owner_uuid, owner_version,
				object_type, object_media_type, object_location, object_digest_value,
				signature_format, signature_value, signature_digest_value,
				certificate_format, certificate_value, certificate_fingerprint
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, in.OwnerType, in.OwnerUUID, in.OwnerVersion,
			in.ObjectType, nullIfEmpty(in.ObjectMediaType), nullIfEmpty(in.ObjectLocation), in.ObjectDigestValue,
			in.SignatureFormat, in.SignatureValue, nullIfEmpty(in.SignatureDigestValue),
			in.CertificateFormat, in.CertificateValue, in.CertificateFingerprint,
		); err != nil {
			return tea.EvidenceBundle{}, err
		}

		if _, err := tx.ExecContext(ctx,
			`UPDATE used_fingerprint SET evidence_bundle_uuid = ? WHERE fingerprint = ?`,
			id, in.CertificateFingerprint,
		); err != nil {
			return tea.EvidenceBundle{}, err
		}

		return getEvidenceBundleTx(ctx, tx, id)
	})
}

// ownerExistsTx reports whether the (ownerType, ownerUUID, ownerVersion)
// tuple an evidence bundle would attach to actually exists.
func ownerExistsTx(ctx context.Context, q dbtx, ownerType, ownerUUID string, ownerVersion int) (bool, error) {
	var table string
	switch ownerType {
	case "ARTIFACT":
		table = "artifact"
	case "COLLECTION":
		table = "collection"
	default:
		return false, fmt.Errorf("repo: unknown evidence bundle owner type %q", ownerType)
	}
	var count int
	err := q.QueryRowContext(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE uuid = ? AND version = ?`, table), ownerUUID, ownerVersion).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetEvidenceBundle fetches an evidence bundle by uuid. Returns ErrNotFound
// if it doesn't exist.
func (r *Repo) GetEvidenceBundle(ctx context.Context, uuid string) (tea.EvidenceBundle, error) {
	return getEvidenceBundleTx(ctx, r.conn(), uuid)
}

// GetEvidenceBundleForOwner returns the evidence bundle attached to
// (ownerType, ownerUUID, ownerVersion). Phase 1 has no bundle versioning or
// supersession, so there is at most one such bundle; returns ErrNotFound if
// none exists.
func (r *Repo) GetEvidenceBundleForOwner(ctx context.Context, ownerType, ownerUUID string, ownerVersion int) (tea.EvidenceBundle, error) {
	var id string
	err := r.conn().QueryRowContext(ctx,
		`SELECT uuid FROM evidence_bundle WHERE owner_type = ? AND owner_uuid = ? AND owner_version = ?`,
		ownerType, ownerUUID, ownerVersion,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return tea.EvidenceBundle{}, ErrNotFound
	}
	if err != nil {
		return tea.EvidenceBundle{}, err
	}
	return getEvidenceBundleTx(ctx, r.conn(), id)
}

func getEvidenceBundleTx(ctx context.Context, q dbtx, uuid string) (tea.EvidenceBundle, error) {
	var (
		bundleVersion, ownerType, ownerUUID                                 string
		ownerVersion                                                        int
		objectType, objectDigestValue, signatureFormat, signatureValue      string
		certificateFormat, certificateValue, certificateFingerprint, status string
		objectMediaType, objectLocation, signatureDigestValue               sql.NullString
	)
	err := q.QueryRowContext(ctx,
		`SELECT bundle_version, owner_type, owner_uuid, owner_version,
			object_type, object_media_type, object_location, object_digest_value,
			signature_format, signature_value, signature_digest_value,
			certificate_format, certificate_value, certificate_fingerprint, status
		 FROM evidence_bundle WHERE uuid = ?`, uuid,
	).Scan(
		&bundleVersion, &ownerType, &ownerUUID, &ownerVersion,
		&objectType, &objectMediaType, &objectLocation, &objectDigestValue,
		&signatureFormat, &signatureValue, &signatureDigestValue,
		&certificateFormat, &certificateValue, &certificateFingerprint, &status,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return tea.EvidenceBundle{}, ErrNotFound
	}
	if err != nil {
		return tea.EvidenceBundle{}, err
	}

	timestamps, err := listEvidenceBundleTimestampsTx(ctx, q, uuid)
	if err != nil {
		return tea.EvidenceBundle{}, err
	}
	transparency, err := listEvidenceBundleTransparencyTx(ctx, q, uuid)
	if err != nil {
		return tea.EvidenceBundle{}, err
	}

	return tea.EvidenceBundle{
		UUID:          uuid,
		BundleVersion: bundleVersion,
		OwnerType:     ownerType,
		OwnerUUID:     ownerUUID,
		OwnerVersion:  ownerVersion,
		Status:        status,
		Object: tea.EvidenceObjectRef{
			ObjectType: objectType,
			MediaType:  objectMediaType.String,
			Location:   objectLocation.String,
			Digest:     tea.EvidenceDigest{Algorithm: "sha-256", Value: objectDigestValue},
		},
		Signature: tea.EvidenceSignature{
			Format:          signatureFormat,
			Value:           signatureValue,
			SignatureDigest: nullableDigest(signatureDigestValue),
		},
		Certificate: tea.EvidenceCertificate{
			Format:      certificateFormat,
			Certificate: certificateValue,
			Fingerprint: &tea.EvidenceDigest{Algorithm: "sha-256", Value: certificateFingerprint},
		},
		Timestamps:   timestamps,
		Transparency: transparency,
	}, nil
}

func nullableDigest(v sql.NullString) *tea.EvidenceDigest {
	if !v.Valid {
		return nil
	}
	return &tea.EvidenceDigest{Algorithm: "sha-256", Value: v.String}
}

// EvidenceBundleTimestampInput carries one RFC 3161 timestamp to attach to
// an existing evidence bundle. Defined now so Phase 2's timestamp client
// only needs to call AddEvidenceBundleTimestamp, not add a migration.
type EvidenceBundleTimestampInput struct {
	TSASubjectDN         string
	TSAPubkeyFingerprint string
	TSAURI               string
	Token                string
	MessageImprintValue  string
}

// AddEvidenceBundleTimestamp appends a timestamp row to evidenceBundleUUID.
// Returns ErrNotFound if the bundle doesn't exist. Unused until Phase 2.
func (r *Repo) AddEvidenceBundleTimestamp(ctx context.Context, evidenceBundleUUID string, in EvidenceBundleTimestampInput) error {
	_, err := runInTx(ctx, r, func(tx dbtx) (struct{}, error) {
		if _, err := getEvidenceBundleTx(ctx, tx, evidenceBundleUUID); err != nil {
			return struct{}{}, err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO evidence_bundle_timestamp (evidence_bundle_uuid, format, tsa_subject_dn, tsa_pubkey_fingerprint, tsa_uri, token, message_imprint_value)
			 VALUES (?, 'rfc3161', ?, ?, ?, ?, ?)`,
			evidenceBundleUUID, nullIfEmpty(in.TSASubjectDN), nullIfEmpty(in.TSAPubkeyFingerprint), nullIfEmpty(in.TSAURI), in.Token, nullIfEmpty(in.MessageImprintValue),
		)
		return struct{}{}, err
	})
	return err
}

func listEvidenceBundleTimestampsTx(ctx context.Context, q dbtx, evidenceBundleUUID string) ([]tea.EvidenceTimestamp, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT tsa_subject_dn, tsa_pubkey_fingerprint, tsa_uri, token, message_imprint_value
		 FROM evidence_bundle_timestamp WHERE evidence_bundle_uuid = ? ORDER BY id`, evidenceBundleUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []tea.EvidenceTimestamp{}
	for rows.Next() {
		var subjectDN, pubkeyFingerprint, uri, token, imprint sql.NullString
		if err := rows.Scan(&subjectDN, &pubkeyFingerprint, &uri, &token, &imprint); err != nil {
			return nil, err
		}
		out = append(out, tea.EvidenceTimestamp{
			Format:               "rfc3161",
			TSASubjectDN:         subjectDN.String,
			TSAPubkeyFingerprint: pubkeyFingerprint.String,
			TSAURI:               uri.String,
			Token:                token.String,
			MessageImprintValue:  imprint.String,
		})
	}
	return out, rows.Err()
}

// EvidenceBundleTransparencyInput carries one transparency-log entry to
// attach to an existing evidence bundle. verificationJSON is opaque,
// system-specific JSON text (see 0006_trust.sql's evidence_bundle_transparency
// comment). Defined now so Phase 3's transparency-log client only needs to
// call AddEvidenceBundleTransparencyEntry, not add a migration.
type EvidenceBundleTransparencyInput struct {
	System             string // "rekor", "sigsum", or "scitt"
	EvidenceType       string
	BindingType        string
	BindingDigestValue string
	VerificationJSON   string
}

// AddEvidenceBundleTransparencyEntry appends a transparency-log entry to
// evidenceBundleUUID. Returns ErrNotFound if the bundle doesn't exist.
// Unused until Phase 3.
func (r *Repo) AddEvidenceBundleTransparencyEntry(ctx context.Context, evidenceBundleUUID string, in EvidenceBundleTransparencyInput) error {
	_, err := runInTx(ctx, r, func(tx dbtx) (struct{}, error) {
		if _, err := getEvidenceBundleTx(ctx, tx, evidenceBundleUUID); err != nil {
			return struct{}{}, err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO evidence_bundle_transparency (evidence_bundle_uuid, system, evidence_type, binding_type, binding_digest_value, verification_json)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			evidenceBundleUUID, in.System, in.EvidenceType, in.BindingType, in.BindingDigestValue, in.VerificationJSON,
		)
		return struct{}{}, err
	})
	return err
}

func listEvidenceBundleTransparencyTx(ctx context.Context, q dbtx, evidenceBundleUUID string) ([]tea.EvidenceTransparency, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT system, evidence_type, binding_type, binding_digest_value, verification_json
		 FROM evidence_bundle_transparency WHERE evidence_bundle_uuid = ? ORDER BY id`, evidenceBundleUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []tea.EvidenceTransparency{}
	for rows.Next() {
		var system, evidenceType, bindingType, bindingDigest, verificationJSON string
		if err := rows.Scan(&system, &evidenceType, &bindingType, &bindingDigest, &verificationJSON); err != nil {
			return nil, err
		}
		out = append(out, tea.EvidenceTransparency{
			System:             system,
			EvidenceType:       evidenceType,
			BindingType:        bindingType,
			BindingDigestValue: bindingDigest,
			VerificationJSON:   json.RawMessage(verificationJSON),
		})
	}
	return out, rows.Err()
}

// ErrEvidenceBundleIncomplete is returned by MarkEvidenceBundleComplete
// when the bundle doesn't yet have both at least one timestamp and at
// least one transparency entry from rekor or sigsum -- the schema's
// minItems:1 + "contains" constraint on a spec-conformant bundle,
// enforced here rather than at the DB layer since Phase 1 can't populate
// either yet.
var ErrEvidenceBundleIncomplete = errors.New("repo: evidence bundle is missing required timestamp or transparency evidence")

// MarkEvidenceBundleComplete transitions uuid's status from draft to
// complete, iff it has at least one timestamp AND at least one
// transparency entry whose system is "rekor" or "sigsum" (an scitt-only
// entry does not satisfy this, matching the schema's requirement that at
// least one of Rekor or Sigsum evidence be present; SCITT is additional,
// optional evidence). Returns ErrEvidenceBundleIncomplete, not a silent
// no-op, if the precondition isn't met.
func (r *Repo) MarkEvidenceBundleComplete(ctx context.Context, uuid string) (tea.EvidenceBundle, error) {
	return runInTx(ctx, r, func(tx dbtx) (tea.EvidenceBundle, error) {
		bundle, err := getEvidenceBundleTx(ctx, tx, uuid)
		if err != nil {
			return tea.EvidenceBundle{}, err
		}
		if len(bundle.Timestamps) == 0 {
			return tea.EvidenceBundle{}, ErrEvidenceBundleIncomplete
		}
		hasRekorOrSigsum := false
		for _, t := range bundle.Transparency {
			if t.System == "rekor" || t.System == "sigsum" {
				hasRekorOrSigsum = true
				break
			}
		}
		if !hasRekorOrSigsum {
			return tea.EvidenceBundle{}, ErrEvidenceBundleIncomplete
		}

		if _, err := tx.ExecContext(ctx, `UPDATE evidence_bundle SET status = 'complete' WHERE uuid = ?`, uuid); err != nil {
			return tea.EvidenceBundle{}, err
		}
		return getEvidenceBundleTx(ctx, tx, uuid)
	})
}

// EvidenceBundleRefInput carries an external-reference evidence bundle:
// the bundle itself lives elsewhere, only a URI + digest is stored here.
type EvidenceBundleRefInput struct {
	OwnerType    string
	OwnerUUID    string
	OwnerVersion int
	URI          string
	DigestValue  string
}

// CreateEvidenceBundleRef records an external evidence-bundle reference for
// (OwnerType, OwnerUUID, OwnerVersion). Returns ErrNotFound if the owning
// artifact/collection doesn't exist.
func (r *Repo) CreateEvidenceBundleRef(ctx context.Context, in EvidenceBundleRefInput) (tea.EvidenceBundleRef, error) {
	return runInTx(ctx, r, func(tx dbtx) (tea.EvidenceBundleRef, error) {
		exists, err := ownerExistsTx(ctx, tx, in.OwnerType, in.OwnerUUID, in.OwnerVersion)
		if err != nil {
			return tea.EvidenceBundleRef{}, err
		}
		if !exists {
			return tea.EvidenceBundleRef{}, ErrNotFound
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO evidence_bundle_ref (owner_type, owner_uuid, owner_version, uri, digest_value) VALUES (?, ?, ?, ?, ?)`,
			in.OwnerType, in.OwnerUUID, in.OwnerVersion, in.URI, in.DigestValue,
		); err != nil {
			return tea.EvidenceBundleRef{}, err
		}
		return tea.EvidenceBundleRef{
			URI:    in.URI,
			Digest: tea.EvidenceDigest{Algorithm: "sha-256", Value: in.DigestValue},
		}, nil
	})
}

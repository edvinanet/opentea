package tea

import "encoding/json"

// EvidenceBundle is the wire shape of oej's TEA Trust Architecture overlay's
// evidence bundle (evidence-bundle-schema.json), attachable to an Artifact
// format or a Collection when this server holds the full evidence locally.
// Mutually exclusive with EvidenceBundleRef on the same owner. UUID/
// OwnerType/OwnerUUID/OwnerVersion/Status are opentea-internal bookkeeping,
// not part of the schema's own object shape, and are omitted from the JSON
// encoding accordingly.
//
// Timestamps/Transparency are empty for a bundle whose Status is "draft" --
// a spec-conformant bundle requires at least one of each; that's populated
// starting Phase 2 (RFC 3161 timestamps) and Phase 3 (transparency-log
// entries) of the trust-architecture implementation plan.
type EvidenceBundle struct {
	UUID          string `json:"-"`
	BundleVersion string `json:"bundleVersion"`
	OwnerType     string `json:"-"`
	OwnerUUID     string `json:"-"`
	OwnerVersion  int    `json:"-"`
	Status        string `json:"-"`

	Object      EvidenceObjectRef   `json:"object"`
	Signature   EvidenceSignature   `json:"signature"`
	Certificate EvidenceCertificate `json:"certificate"`

	Timestamps   []EvidenceTimestamp    `json:"timestamps,omitempty"`
	Transparency []EvidenceTransparency `json:"transparency,omitempty"`
}

// EvidenceDigest is a named-algorithm digest value. The trust architecture
// mandates SHA-256 only (see internal/trust's package doc), so Algorithm is
// always "sha-256" in practice, but the field exists because the schema
// names it explicitly.
type EvidenceDigest struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"value"`
}

// EvidenceObjectRef identifies the TEA object an evidence bundle covers.
type EvidenceObjectRef struct {
	ObjectType string         `json:"objectType"`
	MediaType  string         `json:"mediaType,omitempty"`
	Location   string         `json:"location,omitempty"`
	Digest     EvidenceDigest `json:"digest"`
}

// EvidenceSignature is an evidence bundle's detached signature.
type EvidenceSignature struct {
	Format          string          `json:"format"`
	Value           string          `json:"value"`
	SignatureDigest *EvidenceDigest `json:"signatureDigest,omitempty"`
}

// EvidenceCertificate is the short-lived certificate that signed an
// evidence bundle's object, per internal/trust.BuildCertificate.
type EvidenceCertificate struct {
	Format      string          `json:"format"`
	Certificate string          `json:"certificate"`
	Chain       []string        `json:"chain,omitempty"`
	Fingerprint *EvidenceDigest `json:"fingerprint,omitempty"`
}

// EvidenceTimestamp is one RFC 3161 timestamp over an evidence bundle's
// signature. Populated starting Phase 2 of the trust-architecture
// implementation plan; the shape exists now so Phase 1's schema/wire format
// doesn't need to change when that phase lands.
type EvidenceTimestamp struct {
	Format               string `json:"format"`
	TSASubjectDN         string `json:"tsaSubjectDn,omitempty"`
	TSAPubkeyFingerprint string `json:"tsaPubkeyFingerprint,omitempty"`
	TSAURI               string `json:"tsaUri,omitempty"`
	Token                string `json:"token"`
	MessageImprintValue  string `json:"messageImprintValue,omitempty"`
}

// EvidenceTransparency is one transparency-log entry (Rekor, Sigsum, or
// SCITT) for an evidence bundle. Populated starting Phase 3 of the
// trust-architecture implementation plan. VerificationJSON is raw,
// system-specific JSON (Rekor's inclusion-proof shape and Sigsum's
// witness-signature shape differ entirely) embedded as-is on the wire,
// rather than a string field -- see 0006_trust.sql's
// evidence_bundle_transparency comment for why this isn't modeled as
// distinct columns/fields.
type EvidenceTransparency struct {
	System             string          `json:"system"`
	EvidenceType       string          `json:"evidenceType"`
	BindingType        string          `json:"bindingType"`
	BindingDigestValue string          `json:"bindingDigestValue"`
	VerificationJSON   json.RawMessage `json:"verification"`
}

// EvidenceBundleRef is the external-reference form of an evidence bundle
// (tea-trust-architecture 08-evidence-bundle.md Sec 10): the bundle itself
// is hosted elsewhere, only a URI and a digest over its RFC 8785 canonical
// JSON are carried on the owning Artifact format/Collection.
type EvidenceBundleRef struct {
	URI    string         `json:"uri"`
	Digest EvidenceDigest `json:"digest"`
}

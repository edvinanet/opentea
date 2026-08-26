package trust

import (
	"crypto/ed25519"
	"time"
)

// Fingerprint is the spec's identity value for a signing key: lowercase hex
// SHA-256 of the raw 32-byte Ed25519 public key (tea-trust-architecture
// 01-tea-trust-architecture.md Sec 9). A closed string type so a caller
// can't accidentally pass a raw, un-hashed value where a fingerprint is
// expected.
type Fingerprint string

// TrustDomain is the DNS-name component a certificate's SAN is built from:
// SAN = "<fingerprint>.<trustDomain>" (x509-profile.md Sec 4). Opaque here;
// DNS trust-anchor publication/validation is a later phase (see the
// trust-architecture plan's Phase 6).
type TrustDomain string

// ObjectType is the kind of TEA object an evidence bundle covers
// (evidence-bundle-schema.json's object.objectType). Only the two object
// kinds this phase signs are enumerated; discovery-document and
// cle-document are added when a later phase starts producing bundles for
// them.
type ObjectType string

const (
	ObjectTypeArtifact   ObjectType = "artifact"
	ObjectTypeCollection ObjectType = "tea-collection"
)

// SignatureFormat is the encoding of an evidence bundle's detached
// signature (evidence-bundle-schema.json's signature.format). jws-detached
// is this phase's only emitted format; the others are part of the schema's
// vocabulary and accepted so a later phase choosing one of them isn't a
// schema/migration change.
type SignatureFormat string

const (
	SignatureFormatCMSDetached  SignatureFormat = "cms-detached"
	SignatureFormatDSSEEnvelope SignatureFormat = "dsse-envelope"
	SignatureFormatJWSDetached  SignatureFormat = "jws-detached"
	SignatureFormatCOSESign1    SignatureFormat = "cose-sign1"
)

// CertificateFormat is the encoding of an evidence bundle's certificate.
type CertificateFormat string

const (
	CertificateFormatX509PEM CertificateFormat = "x509-pem"
	CertificateFormatX509DER CertificateFormat = "x509-der"
)

// MaxKeyValidity is the hard ceiling on an ephemeral signing key's validity
// window (x509-profile.md Sec 12: certificates are short-lived, "generally
// no more than one hour").
const MaxKeyValidity = time.Hour

// KeyPair is an ephemeral Ed25519 signing key. It is never persisted:
// callers generate one, sign with it, and Destroy it immediately after --
// tea-trust-architecture 01 Sec 9 requires key pairs not be reused across
// signing events.
type KeyPair struct {
	Public    ed25519.PublicKey
	Private   ed25519.PrivateKey
	NotBefore time.Time
	NotAfter  time.Time
}

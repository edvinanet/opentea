package trust

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// GenerateEphemeralKey creates a fresh Ed25519 key pair valid from now for
// validity, capped at MaxKeyValidity -- a caller asking for longer gets
// MaxKeyValidity, not an error, since "as long as possible, but no more
// than the ceiling" is the only sensible reading of the spec's limit.
func GenerateEphemeralKey(validity time.Duration) (KeyPair, error) {
	if validity <= 0 || validity > MaxKeyValidity {
		validity = MaxKeyValidity
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return KeyPair{}, fmt.Errorf("trust: generate ephemeral key: %w", err)
	}
	now := time.Now().UTC()
	return KeyPair{
		Public:    pub,
		Private:   priv,
		NotBefore: now,
		NotAfter:  now.Add(validity),
	}, nil
}

// Destroy zeroes kp.Private in place. Best-effort -- Go's garbage collector
// can leave copies of the original bytes elsewhere in memory -- but still
// required at every call site once signing is complete, per the "ephemeral,
// generate-per-event, never reused" key lifecycle rule.
func (kp *KeyPair) Destroy() {
	for i := range kp.Private {
		kp.Private[i] = 0
	}
}

// FingerprintOf returns the spec's identity value for pub: lowercase hex
// SHA-256 of the raw public key bytes.
func FingerprintOf(pub ed25519.PublicKey) Fingerprint {
	sum := sha256.Sum256(pub)
	return Fingerprint(hex.EncodeToString(sum[:]))
}

// Sign signs objectBytes with priv and returns the raw signature. Ed25519
// signs the message directly -- there is no separate hash-then-sign step
// for a caller to get wrong.
func Sign(priv ed25519.PrivateKey, objectBytes []byte) []byte {
	return ed25519.Sign(priv, objectBytes)
}

// Verify reports whether sig is a valid Ed25519 signature over objectBytes
// under pub. Pure function -- no DB/HTTP -- so it's directly unit-testable
// and directly reusable by a caller (e.g. an admin handler, or a consumer
// verifying a fetched bundle independently) without touching this package's
// other state.
func Verify(pub ed25519.PublicKey, objectBytes, sig []byte) bool {
	return ed25519.Verify(pub, objectBytes, sig)
}

// SHA256Hex returns the lowercase hex SHA-256 digest of b. The one digest
// primitive every other piece of this package (fingerprints, object
// digests, evidenceBundleRef digests) is built from -- the trust
// architecture spec mandates SHA-256 only, no alternative algorithm
// anywhere (01 Sec 9, 08 Sec 9.1).
func SHA256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

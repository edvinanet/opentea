// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package trust

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"testing"
	"time"

	"github.com/oej/opentea/pkg/tea"
)

// TestArtifactEvidenceVectorV1 is the published byte-exact interoperability
// test vector for design/publisher-service.md §9.7's "structure v1" (the
// exact bytes an artifact-evidence signature covers) -- the regression test
// for docs/security-review-publisher-design-260828.md finding 5, which
// flagged that nothing pinned down whether a signature covered the hex
// digest's ASCII characters or its 32 decoded bytes. This test is the
// source of truth §9.7's published vector is copied from -- if this test's
// expected values ever change, §9.7 must be updated to match, not the
// other way around.
//
// The Ed25519 key is derived from a fixed, non-secret seed (0x00..0x1f),
// not crypto/rand.Reader -- the point of a published vector is that any
// independent implementation can reproduce every value below exactly.
// Ed25519 signing is itself deterministic (RFC 8032: no random nonce,
// unlike ECDSA), so there is exactly one correct signatureValue for this
// key and digest.
func TestArtifactEvidenceVectorV1(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)

	const wantPublicKeyHex = "03a107bff3ce10be1d70dd18e74bc09967e4d6309ba50d5f1ddc8664125531b8"
	if got := hex.EncodeToString(pub); got != wantPublicKeyHex {
		t.Fatalf("public key = %s, want %s", got, wantPublicKeyHex)
	}
	const wantFingerprint = Fingerprint("56475aa75463474c0285df5dbf2bcab73da651358839e9b77481b2eab107708c")
	if got := FingerprintOf(pub); got != wantFingerprint {
		t.Fatalf("fingerprint = %s, want %s", got, wantFingerprint)
	}

	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	artifact := tea.Artifact{
		UUID:        "d4d9f54a-abcf-11ee-ac79-1a52914d44b1",
		Version:     1,
		Name:        "example-sbom.json",
		Type:        "BOM",
		CreatedDate: &created,
		Formats: []tea.ArtifactFormat{
			{MediaType: "application/vnd.cyclonedx+json"},
		},
	}

	// Step 1: RFC 8785 JCS canonicalization.
	canonical, err := Canonicalize(artifact)
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	const wantCanonical = `{"createdDate":"2026-01-01T00:00:00Z","formats":[{"mediaType":"application/vnd.cyclonedx+json"}],"name":"example-sbom.json","type":"BOM","uuid":"d4d9f54a-abcf-11ee-ac79-1a52914d44b1","version":1}`
	if got := string(canonical); got != wantCanonical {
		t.Fatalf("canonical JSON = %s, want %s", got, wantCanonical)
	}

	// Step 2: digest = SHA-256 of the canonical bytes, transported as hex
	// (this is "objectDigestValue"/"digestToSign" on the wire).
	digestHex := SHA256Hex(canonical)
	const wantDigestHex = "0339d2aa34db670a08263975373480d6a1dab14c7b45ed9e3b43bc635662b62c"
	if digestHex != wantDigestHex {
		t.Fatalf("digest = %s, want %s", digestHex, wantDigestHex)
	}

	// Step 3+4: the signature covers the 32 RAW DECODED digest bytes --
	// never the hex ASCII characters, never the canonical JSON directly.
	// "jws-detached" in this phase is a raw Ed25519 signature,
	// base64-standard-encoded, not RFC 7797 JWS compact serialization (see
	// §9.7 step 4 for why the label is reused anyway).
	digestBytes, err := hex.DecodeString(digestHex)
	if err != nil {
		t.Fatalf("hex.DecodeString: %v", err)
	}
	sig := Sign(priv, digestBytes)
	sigB64 := base64.StdEncoding.EncodeToString(sig)
	const wantSignatureB64 = "E+o+tG2jCLbCkVr5zUEQ8vyFYQKLOlPacO/Wj7jgB14Nn4ZRorYk52GNBkq9jZHuTQ+ua9MNxrNmASczA9xXAQ=="
	if sigB64 != wantSignatureB64 {
		t.Fatalf("signature = %s, want %s", sigB64, wantSignatureB64)
	}

	// Confirms the vector is actually self-consistent (a server presented
	// with exactly these values verifies them), and that signing twice
	// produces byte-identical output (the determinism claim above).
	if !Verify(pub, digestBytes, sig) {
		t.Fatal("published vector does not verify")
	}
	if sig2 := Sign(priv, digestBytes); string(sig2) != string(sig) {
		t.Fatal("Ed25519 signing was not deterministic across two calls with the same key and message")
	}
}

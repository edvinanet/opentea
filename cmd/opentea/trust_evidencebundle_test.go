package main

import (
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"testing"
	"time"

	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/internal/trust"
	"github.com/oej/opentea/pkg/tea"
)

// createTestArtifact creates a minimal artifact via /admin/v1/artifacts and
// returns its uuid/version, for use as an evidence-bundle owner.
func createTestArtifact(t *testing.T, srv *testServer) (uuid string, version int) {
	t.Helper()
	status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/artifacts", map[string]any{
		"type":    "BOM",
		"formats": []map[string]any{{"mediaType": "application/json"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("create artifact: status=%d body=%s", status, raw)
	}
	var a tea.Artifact
	decodeInto(t, raw, &a)
	return a.UUID, a.Version
}

// signedEvidenceRequest generates a fresh ephemeral key/certificate, signs
// digestHex's raw bytes, and returns the request body
// createEvidenceBundleForOwner expects.
func signedEvidenceRequest(t *testing.T, digestHex string) (body map[string]any, kp trust.KeyPair) {
	t.Helper()
	kp, err := trust.GenerateEphemeralKey(time.Hour)
	if err != nil {
		t.Fatalf("GenerateEphemeralKey: %v", err)
	}
	certPEM, err := trust.BuildCertificate(kp, "trust.example.com")
	if err != nil {
		t.Fatalf("BuildCertificate: %v", err)
	}
	digestBytes, err := hex.DecodeString(digestHex)
	if err != nil {
		t.Fatalf("decode digestHex: %v", err)
	}
	sig := trust.Sign(kp.Private, digestBytes)

	return map[string]any{
		"evidence": map[string]any{
			"objectDigestValue": digestHex,
			"signatureFormat":   "jws-detached",
			"signatureValue":    base64.StdEncoding.EncodeToString(sig),
			"certificatePem":    certPEM,
		},
	}, kp
}

// TestCreateArtifactEvidenceBundleRoundTrip drives the full Phase 1
// evidence-bundle flow through the real admin HTTP handlers (not just the
// repo layer, which internal/repo/evidencebundle_test.go already covers
// directly): create an artifact, sign a digest of it, POST the evidence,
// and independently re-verify what comes back from GET without trusting
// the server's own say-so.
func TestCreateArtifactEvidenceBundleRoundTrip(t *testing.T) {
	srv := newTestServer(t)
	artifactUUID, artifactVersion := createTestArtifact(t, srv)

	digestHex := trust.SHA256Hex([]byte("artifact object bytes"))
	body, kp := signedEvidenceRequest(t, digestHex)

	status, raw := jsonRequest(t, srv, http.MethodPost,
		"/admin/v1/artifacts/"+artifactUUID+"/1/evidenceBundle", body)
	if status != http.StatusCreated {
		t.Fatalf("create evidence bundle: status=%d body=%s", status, raw)
	}
	var created tea.EvidenceBundle
	decodeInto(t, raw, &created)

	// Status is internal bookkeeping (json:"-", see pkg/tea/trust.go), not
	// part of the HTTP response body -- check it via the repo layer instead.
	evidenceBundle, err := srv.repo.GetEvidenceBundleForOwner(t.Context(), "ARTIFACT", artifactUUID, artifactVersion)
	if err != nil {
		t.Fatalf("GetEvidenceBundleForOwner: %v", err)
	}
	if evidenceBundle.Status != "draft" {
		t.Fatalf("Status = %q, want draft", evidenceBundle.Status)
	}

	// Fetch back over HTTP and independently re-verify, proving round-trip
	// byte-fidelity through the DB rather than just trusting the server's
	// own verification at write time.
	status, raw = jsonRequest(t, srv, http.MethodGet, "/admin/v1/evidenceBundles/"+evidenceBundle.UUID, nil)
	if status != http.StatusOK {
		t.Fatalf("get evidence bundle: status=%d body=%s", status, raw)
	}
	var fetched tea.EvidenceBundle
	decodeInto(t, raw, &fetched)

	pub, err := trust.ParseCertificatePublicKey(fetched.Certificate.Certificate, time.Now())
	if err != nil {
		t.Fatalf("ParseCertificatePublicKey: %v", err)
	}
	if pub.Equal(kp.Public) == false {
		t.Fatal("returned certificate's public key does not match the signing key")
	}
	digestBytes, _ := hex.DecodeString(fetched.Object.Digest.Value)
	sigBytes, err := base64.StdEncoding.DecodeString(fetched.Signature.Value)
	if err != nil {
		t.Fatalf("decode returned signature: %v", err)
	}
	if !trust.Verify(pub, digestBytes, sigBytes) {
		t.Fatal("independently re-verifying the fetched signature/certificate/digest failed")
	}
}

// TestCreateArtifactEvidenceBundleRejectsFingerprintReuse confirms
// fingerprint reuse is rejected at the HTTP layer, not just the repo layer
// (internal/repo/evidencebundle_test.go already covers the repo layer
// directly).
func TestCreateArtifactEvidenceBundleRejectsFingerprintReuse(t *testing.T) {
	srv := newTestServer(t)
	kp, err := trust.GenerateEphemeralKey(time.Hour)
	if err != nil {
		t.Fatalf("GenerateEphemeralKey: %v", err)
	}
	certPEM, err := trust.BuildCertificate(kp, "trust.example.com")
	if err != nil {
		t.Fatalf("BuildCertificate: %v", err)
	}

	makeBody := func(digestHex string) map[string]any {
		digestBytes, _ := hex.DecodeString(digestHex)
		sig := trust.Sign(kp.Private, digestBytes)
		return map[string]any{
			"evidence": map[string]any{
				"objectDigestValue": digestHex,
				"signatureFormat":   "jws-detached",
				"signatureValue":    base64.StdEncoding.EncodeToString(sig),
				"certificatePem":    certPEM,
			},
		}
	}

	uuid1, _ := createTestArtifact(t, srv)
	status, raw := jsonRequest(t, srv, http.MethodPost,
		"/admin/v1/artifacts/"+uuid1+"/1/evidenceBundle", makeBody(trust.SHA256Hex([]byte("object 1"))))
	if status != http.StatusCreated {
		t.Fatalf("first evidence bundle: status=%d body=%s", status, raw)
	}

	uuid2, _ := createTestArtifact(t, srv)
	status, raw = jsonRequest(t, srv, http.MethodPost,
		"/admin/v1/artifacts/"+uuid2+"/1/evidenceBundle", makeBody(trust.SHA256Hex([]byte("object 2"))))
	if status != http.StatusBadRequest {
		t.Fatalf("second evidence bundle (reused fingerprint): status=%d, want 400; body=%s", status, raw)
	}
}

// TestCreateArtifactEvidenceBundleRejectsTamperedSignature is the
// verify-before-store regression test: spec 08-evidence-bundle.md Sec
// 5.4/15.3 requires a publisher API reject invalid evidence and MUST NOT
// store it. Flipping one byte of the signature must both fail the request
// and leave no row behind.
func TestCreateArtifactEvidenceBundleRejectsTamperedSignature(t *testing.T) {
	srv := newTestServer(t)
	artifactUUID, artifactVersion := createTestArtifact(t, srv)

	digestHex := trust.SHA256Hex([]byte("artifact object bytes"))
	body, _ := signedEvidenceRequest(t, digestHex)

	evidence := body["evidence"].(map[string]any)
	sigBytes, err := base64.StdEncoding.DecodeString(evidence["signatureValue"].(string))
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	sigBytes[0] ^= 0xff
	evidence["signatureValue"] = base64.StdEncoding.EncodeToString(sigBytes)

	status, raw := jsonRequest(t, srv, http.MethodPost,
		"/admin/v1/artifacts/"+artifactUUID+"/1/evidenceBundle", body)
	if status != http.StatusBadRequest {
		t.Fatalf("tampered evidence bundle: status=%d, want 400; body=%s", status, raw)
	}

	if _, err := srv.repo.GetEvidenceBundleForOwner(t.Context(), "ARTIFACT", artifactUUID, artifactVersion); err != repo.ErrNotFound {
		t.Fatalf("GetEvidenceBundleForOwner after rejected upload: err = %v, want ErrNotFound (row must not have been stored)", err)
	}
}

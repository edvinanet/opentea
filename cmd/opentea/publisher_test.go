// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/trust"
	"github.com/oej/opentea/pkg/tea"
)

// publisherRequest hits /publisher/v1 with a bearer credential (no session
// cookie -- /publisher/v1 doesn't use one, see internal/publisher's
// auth_middleware.go). An empty token sends no Authorization header at all.
func publisherRequest(t *testing.T, srv *testServer, method, path, token string, body any) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, srv.URL+path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp.StatusCode, respBody
}

// createPublisherCredential mints a /publisher/v1 bearer credential via the
// admin API and returns its raw token.
func createPublisherCredential(t *testing.T, srv *testServer, label, scope string) string {
	t.Helper()
	status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/publisherCredentials", map[string]any{
		"label": label, "scope": scope,
	})
	if status != http.StatusCreated {
		t.Fatalf("create publisher credential: status=%d body=%s", status, raw)
	}
	var resp struct {
		model.PublisherCredential
		Token string `json:"token"`
	}
	decodeInto(t, raw, &resp)
	if resp.Token == "" {
		t.Fatalf("create publisher credential: empty token in response %s", raw)
	}
	return resp.Token
}

// signDigest generates a fresh ephemeral key/certificate and signs digestHex
// -- mirrors trust_evidencebundle_test.go's signedEvidenceRequest, factored
// for reuse across both the artifact-evidence and collection-commit steps
// (each needs its own fresh key: internal/trust rejects fingerprint reuse).
func signDigest(t *testing.T, digestHex string) (signatureValue, certificatePEM string) {
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
	return base64.StdEncoding.EncodeToString(sig), certPEM
}

func TestPublisherCredentialScopeEnforcement(t *testing.T) {
	srv := newTestServer(t)
	full := createPublisherCredential(t, srv, "full-cred", model.PublisherScopeFull)
	cicd := createPublisherCredential(t, srv, "cicd-cred", model.PublisherScopeCICD)

	// No token at all -> 401.
	if status, _ := publisherRequest(t, srv, http.MethodPost, "/publisher/v1/products", "", map[string]any{"name": "x"}); status != http.StatusUnauthorized {
		t.Fatalf("no token: status=%d, want 401", status)
	}
	// Garbage token -> 401.
	if status, _ := publisherRequest(t, srv, http.MethodPost, "/publisher/v1/products", "not-a-real-token", map[string]any{"name": "x"}); status != http.StatusUnauthorized {
		t.Fatalf("invalid token: status=%d, want 401", status)
	}
	// cicd credential may not create products.
	if status, body := publisherRequest(t, srv, http.MethodPost, "/publisher/v1/products", cicd, map[string]any{"name": "x"}); status != http.StatusForbidden {
		t.Fatalf("cicd createProduct: status=%d body=%s, want 403", status, body)
	}
	// full credential may.
	if status, body := publisherRequest(t, srv, http.MethodPost, "/publisher/v1/products", full, map[string]any{"name": "x"}); status != http.StatusCreated {
		t.Fatalf("full createProduct: status=%d body=%s, want 201", status, body)
	}
}

// TestPublisherFullWorkflow drives the entire /publisher/v1 protocol
// through real HTTP handlers, both credential scopes, and real Ed25519
// signatures: create product/release/artifact, submit artifact evidence,
// assemble a collection draft, approve it, prepare+commit it, and confirm
// the resulting collection is independently readable via the real /tea/v1
// consumer API -- not just that /publisher/v1 claims success.
func TestPublisherFullWorkflow(t *testing.T) {
	srv := newTestServer(t)
	full := createPublisherCredential(t, srv, "full-cred", model.PublisherScopeFull)
	cicd := createPublisherCredential(t, srv, "cicd-cred", model.PublisherScopeCICD)

	status, raw := publisherRequest(t, srv, http.MethodPost, "/publisher/v1/products", full, map[string]any{"name": "Acme Widget"})
	if status != http.StatusCreated {
		t.Fatalf("createProduct: status=%d body=%s", status, raw)
	}
	var product tea.Product
	decodeInto(t, raw, &product)

	status, raw = publisherRequest(t, srv, http.MethodPost, "/publisher/v1/products/"+product.UUID+"/releases", full, map[string]any{
		"version": "1.0.0", "createdDate": "2026-07-01T00:00:00Z",
	})
	if status != http.StatusCreated {
		t.Fatalf("createProductRelease: status=%d body=%s", status, raw)
	}
	var release tea.ProductRelease
	decodeInto(t, raw, &release)

	status, raw = publisherRequest(t, srv, http.MethodPost, "/publisher/v1/artifacts", cicd, map[string]any{
		"type":    "BOM",
		"formats": []map[string]any{{"mediaType": "application/vnd.cyclonedx+json"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("createArtifact: status=%d body=%s", status, raw)
	}
	var artifact tea.Artifact
	decodeInto(t, raw, &artifact)

	// Artifact validation: prepare -> sign -> submit.
	status, raw = publisherRequest(t, srv, http.MethodPost,
		"/publisher/v1/artifacts/"+artifact.UUID+"/1/evidence/prepare", cicd, nil)
	if status != http.StatusOK {
		t.Fatalf("prepareArtifactEvidence: status=%d body=%s", status, raw)
	}
	var preparedArtifact struct {
		DigestToSign string `json:"digestToSign"`
	}
	decodeInto(t, raw, &preparedArtifact)
	sigValue, certPEM := signDigest(t, preparedArtifact.DigestToSign)

	status, raw = publisherRequest(t, srv, http.MethodPost, "/publisher/v1/artifacts/"+artifact.UUID+"/1/evidence", cicd, map[string]any{
		"objectDigestValue": preparedArtifact.DigestToSign,
		"signatureFormat":   "jws-detached",
		"signatureValue":    sigValue,
		"certificatePem":    certPEM,
	})
	if status != http.StatusCreated {
		t.Fatalf("submitArtifactEvidence: status=%d body=%s", status, raw)
	}

	// Collection assembly: cicd may draft, only full may approve.
	draftPath := "/publisher/v1/productReleases/" + release.UUID + "/collectionDraft"
	status, raw = publisherRequest(t, srv, http.MethodPut, draftPath, cicd, map[string]any{
		"actor":     "ci-pipeline",
		"artifacts": []map[string]any{{"uuid": artifact.UUID, "version": 1}},
	})
	if status != http.StatusOK {
		t.Fatalf("putCollectionDraft: status=%d body=%s", status, raw)
	}

	if status, body := publisherRequest(t, srv, http.MethodPost, draftPath+"/approve", cicd, map[string]any{"actor": "reviewer"}); status != http.StatusForbidden {
		t.Fatalf("cicd approve: status=%d body=%s, want 403", status, body)
	}
	status, raw = publisherRequest(t, srv, http.MethodPost, draftPath+"/approve", full, map[string]any{"actor": "reviewer"})
	if status != http.StatusOK {
		t.Fatalf("approveCollectionDraft: status=%d body=%s", status, raw)
	}

	// Collection signing + commit: prepare -> sign -> commit.
	status, raw = publisherRequest(t, srv, http.MethodPost, draftPath+"/prepareCommit", cicd, nil)
	if status != http.StatusOK {
		t.Fatalf("prepareCollectionCommit: status=%d body=%s", status, raw)
	}
	var prepared struct {
		DigestToSign string `json:"digestToSign"`
	}
	decodeInto(t, raw, &prepared)
	collectionSig, collectionCert := signDigest(t, prepared.DigestToSign)

	status, raw = publisherRequest(t, srv, http.MethodPost, draftPath+"/commit", cicd, map[string]any{
		"objectDigestValue": prepared.DigestToSign,
		"signatureFormat":   "jws-detached",
		"signatureValue":    collectionSig,
		"certificatePem":    collectionCert,
	})
	if status != http.StatusCreated {
		t.Fatalf("commitCollectionDraft: status=%d body=%s", status, raw)
	}
	var collection tea.Collection
	decodeInto(t, raw, &collection)
	if collection.Version != 1 {
		t.Fatalf("collection.Version = %d, want 1", collection.Version)
	}

	// The draft is gone.
	if status, _ := publisherRequest(t, srv, http.MethodGet, draftPath, cicd, nil); status != http.StatusNotFound {
		t.Fatalf("getCollectionDraft after commit: status=%d, want 404", status)
	}

	// Independently confirm via the real, unauthenticated /tea/v1 consumer
	// API -- not just that /publisher/v1 claims the commit succeeded.
	teaStatus, _, teaBody := teaRequest(t, srv, http.MethodGet, "/tea/v1/productRelease/"+release.UUID+"/collection/latest", "")
	if teaStatus != http.StatusOK {
		t.Fatalf("GET /tea/v1 collection/latest: status=%d body=%s", teaStatus, teaBody)
	}
	var readBack tea.Collection
	decodeInto(t, teaBody, &readBack)
	if readBack.Version != 1 || len(readBack.Artifacts) != 1 || readBack.Artifacts[0].UUID != artifact.UUID {
		t.Fatalf("readBack = %+v", readBack)
	}
}

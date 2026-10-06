// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/trust"
	"github.com/oej/opentea/pkg/tea"
	"github.com/oej/opentea/pkg/teapublisher"
)

// publisherUploadFile POSTs a multipart file upload to /publisher/v1 with a
// bearer credential and an extra "mediaType" form field, mirroring
// integration_test.go's uploadFile but for /publisher/v1's bearer auth and
// mediaType-based format addressing (not formatIndex).
func publisherUploadFile(t *testing.T, srv *testServer, path, token, mediaTypeField string, content []byte) (int, []byte) {
	t.Helper()
	return publisherUploadFileField(t, srv, path, token, "mediaType", mediaTypeField, content)
}

// publisherUploadFileField is publisherUploadFile generalized to write an
// arbitrary form field naming the target format -- "mediaType" (the usual
// case) or "formatId" (docs/security-review-publisher-design-260828.md
// finding 14's stable-id addressing, for when mediaType alone is
// ambiguous).
func publisherUploadFileField(t *testing.T, srv *testServer, path, token, fieldName, fieldValue string, content []byte) (int, []byte) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField(fieldName, fieldValue); err != nil {
		t.Fatalf("write %s field: %v", fieldName, err)
	}
	part, err := w.CreateFormFile("file", "artifact.bin")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write file content: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, srv.URL+path, &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp.StatusCode, respBody
}

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
		"version": "1.0.0",
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
	// Version 2, not 1: createProductRelease already created the
	// required initial empty v1 collection atomically
	// (docs/security-review-260923.md finding #7) -- this commit is the
	// release's first collection with real content.
	if collection.Version != 2 {
		t.Fatalf("collection.Version = %d, want 2", collection.Version)
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
	if readBack.Version != 2 || len(readBack.Artifacts) != 1 || readBack.Artifacts[0].UUID != artifact.UUID {
		t.Fatalf("readBack = %+v", readBack)
	}
}

// TestPublisherSubmitArtifactEvidenceIdempotentReplay is the regression
// test for docs/security-review-publisher-design-260828.md finding 13
// ("retried evidence submission creating duplicate evidence bundles"):
// resubmitting the exact same evidence package (the caller's connection
// dropped before it saw the first 201, say) must succeed again, not fail
// with a confusing fingerprint-reuse error -- see
// internal/repo.TestCreateEvidenceBundleIdempotentReplay for the repo-level
// proof that it's genuinely the same stored bundle, not a new one; this is
// the real-HTTP-round-trip confirmation that the fix is actually wired up.
func TestPublisherSubmitArtifactEvidenceIdempotentReplay(t *testing.T) {
	srv := newTestServer(t)
	cicd := createPublisherCredential(t, srv, "cicd-cred", model.PublisherScopeCICD)

	status, raw := publisherRequest(t, srv, http.MethodPost, "/publisher/v1/artifacts", cicd, map[string]any{
		"type":    "BOM",
		"formats": []map[string]any{{"mediaType": "application/vnd.cyclonedx+json"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("createArtifact: status=%d body=%s", status, raw)
	}
	var artifact tea.Artifact
	decodeInto(t, raw, &artifact)

	status, raw = publisherRequest(t, srv, http.MethodPost, "/publisher/v1/artifacts/"+artifact.UUID+"/1/evidence/prepare", cicd, nil)
	if status != http.StatusOK {
		t.Fatalf("prepareArtifactEvidence: status=%d body=%s", status, raw)
	}
	var prepared struct {
		DigestToSign string `json:"digestToSign"`
	}
	decodeInto(t, raw, &prepared)
	sigValue, certPEM := signDigest(t, prepared.DigestToSign)

	evidenceBody := map[string]any{
		"objectDigestValue": prepared.DigestToSign,
		"signatureFormat":   "jws-detached",
		"signatureValue":    sigValue,
		"certificatePem":    certPEM,
	}
	evidencePath := "/publisher/v1/artifacts/" + artifact.UUID + "/1/evidence"

	status, raw = publisherRequest(t, srv, http.MethodPost, evidencePath, cicd, evidenceBody)
	if status != http.StatusCreated {
		t.Fatalf("first submitArtifactEvidence: status=%d body=%s", status, raw)
	}

	// Exact retry -- must succeed, not hit a fingerprint-reuse error.
	status, raw = publisherRequest(t, srv, http.MethodPost, evidencePath, cicd, evidenceBody)
	if status != http.StatusCreated {
		t.Fatalf("retried submitArtifactEvidence: status=%d body=%s, want 201 (idempotent replay)", status, raw)
	}
}

// TestPublisherPutCollectionDraftExpectedRevisionConflict is the regression
// test for docs/security-review-publisher-design-260828.md finding 13's
// "ETag/If-Match, expected revision, or equivalent for draft replacement":
// a stale expectedRevision must be rejected (409), and a correct one must
// succeed -- real HTTP round trip confirmation of
// internal/repo.TestPutCollectionDraftExpectedRevisionConflict.
func TestPublisherPutCollectionDraftExpectedRevisionConflict(t *testing.T) {
	srv := newTestServer(t)
	full := createPublisherCredential(t, srv, "full-cred", model.PublisherScopeFull)

	status, raw := publisherRequest(t, srv, http.MethodPost, "/publisher/v1/products", full, map[string]any{"name": "Acme Widget"})
	if status != http.StatusCreated {
		t.Fatalf("createProduct: status=%d body=%s", status, raw)
	}
	var product tea.Product
	decodeInto(t, raw, &product)

	status, raw = publisherRequest(t, srv, http.MethodPost, "/publisher/v1/products/"+product.UUID+"/releases", full, map[string]any{"version": "1.0.0"})
	if status != http.StatusCreated {
		t.Fatalf("createProductRelease: status=%d body=%s", status, raw)
	}
	var release tea.ProductRelease
	decodeInto(t, raw, &release)

	draftPath := "/publisher/v1/productReleases/" + release.UUID + "/collectionDraft"
	status, raw = publisherRequest(t, srv, http.MethodPut, draftPath, full, map[string]any{
		"actor": "ci-pipeline", "expectedRevision": 0,
	})
	if status != http.StatusOK {
		t.Fatalf("putCollectionDraft with expectedRevision=0 on a fresh release: status=%d body=%s", status, raw)
	}

	// Stale expectedRevision (draft is now at revision 1) -- 409.
	if status, body := publisherRequest(t, srv, http.MethodPut, draftPath, full, map[string]any{
		"actor": "ci-pipeline", "expectedRevision": 0,
	}); status != http.StatusConflict {
		t.Fatalf("stale expectedRevision=0: status=%d body=%s, want 409", status, body)
	}

	// Correct expectedRevision (1) -- succeeds.
	status, raw = publisherRequest(t, srv, http.MethodPut, draftPath, full, map[string]any{
		"actor": "ci-pipeline", "expectedRevision": 1,
	})
	if status != http.StatusOK {
		t.Fatalf("correct expectedRevision=1: status=%d body=%s", status, raw)
	}
	var draft teapublisher.CollectionDraft
	decodeInto(t, raw, &draft)
	if draft.Revision != 2 {
		t.Fatalf("Revision = %d, want 2", draft.Revision)
	}
}

// TestPublisherCreateReleaseCreatedDateServerAssigned is the regression test
// for docs/security-review-publisher-design-260828.md finding 11: createProductRelease/
// createComponentRelease used to require (and trust) a caller-supplied
// createdDate, letting a faulty or malicious publisher back-date or
// future-date a record TEA 1.0's own spec documents as "the time the object
// was created in TEA" -- a server-assigned fact, not a manufacturer
// assertion (unlike releaseDate, which genuinely is one, and is deliberately
// left untouched by this fix). Submits a createdDate far in the past and
// confirms the server ignores it, stamping its own current time instead, for
// both product releases and component releases.
func TestPublisherCreateReleaseCreatedDateServerAssigned(t *testing.T) {
	srv := newTestServer(t)
	full := createPublisherCredential(t, srv, "full-cred", model.PublisherScopeFull)
	before := time.Now().Add(-time.Minute)
	backdated := "2020-01-01T00:00:00Z"

	status, raw := publisherRequest(t, srv, http.MethodPost, "/publisher/v1/products", full, map[string]any{"name": "Acme Widget"})
	if status != http.StatusCreated {
		t.Fatalf("createProduct: status=%d body=%s", status, raw)
	}
	var product tea.Product
	decodeInto(t, raw, &product)

	status, raw = publisherRequest(t, srv, http.MethodPost, "/publisher/v1/products/"+product.UUID+"/releases", full, map[string]any{
		"version":     "1.0.0",
		"createdDate": backdated,
	})
	if status != http.StatusCreated {
		t.Fatalf("createProductRelease: status=%d body=%s", status, raw)
	}
	var release tea.ProductRelease
	decodeInto(t, raw, &release)
	if release.CreatedDate.Before(before) {
		t.Fatalf("productRelease.createdDate = %v, want server-assigned (>= %v), not the submitted %q", release.CreatedDate, before, backdated)
	}

	status, raw = publisherRequest(t, srv, http.MethodPost, "/publisher/v1/components", full, map[string]any{"name": "Acme Component"})
	if status != http.StatusCreated {
		t.Fatalf("createComponent: status=%d body=%s", status, raw)
	}
	var component tea.Component
	decodeInto(t, raw, &component)

	status, raw = publisherRequest(t, srv, http.MethodPost, "/publisher/v1/components/"+component.UUID+"/releases", full, map[string]any{
		"version":     "1.0.0",
		"createdDate": backdated,
	})
	if status != http.StatusCreated {
		t.Fatalf("createComponentRelease: status=%d body=%s", status, raw)
	}
	var componentRelease tea.ComponentRelease
	decodeInto(t, raw, &componentRelease)
	if componentRelease.CreatedDate.Before(before) {
		t.Fatalf("componentRelease.createdDate = %v, want server-assigned (>= %v), not the submitted %q", componentRelease.CreatedDate, before, backdated)
	}
}

// TestPublisherSelfApprovalRejectedAcrossDifferentActorNames is the
// regression test for the finding that maker-checker self-approval
// prevention only compared caller-supplied "actor" JSON strings, never the
// authenticated credential that made each request -- a holder of one
// full-scope credential could draft as one actor name and approve as a
// different one and the old check would let it through
// (docs/security-review-260923.md finding #4). Uses the SAME publisher
// credential for both the draft and the approval, with two different actor
// names ("alice" drafts, "bob" approves) -- must still be rejected as
// self-approval.
func TestPublisherSelfApprovalRejectedAcrossDifferentActorNames(t *testing.T) {
	srv := newTestServer(t)
	full := createPublisherCredential(t, srv, "full-cred", model.PublisherScopeFull)

	status, raw := publisherRequest(t, srv, http.MethodPost, "/publisher/v1/products", full, map[string]any{"name": "Acme Widget"})
	if status != http.StatusCreated {
		t.Fatalf("createProduct: status=%d body=%s", status, raw)
	}
	var product tea.Product
	decodeInto(t, raw, &product)

	status, raw = publisherRequest(t, srv, http.MethodPost, "/publisher/v1/products/"+product.UUID+"/releases", full, map[string]any{
		"version": "1.0.0",
	})
	if status != http.StatusCreated {
		t.Fatalf("createProductRelease: status=%d body=%s", status, raw)
	}
	var release tea.ProductRelease
	decodeInto(t, raw, &release)

	draftPath := "/publisher/v1/productReleases/" + release.UUID + "/collectionDraft"
	status, raw = publisherRequest(t, srv, http.MethodPut, draftPath, full, map[string]any{
		"actor":     "alice",
		"artifacts": []map[string]any{},
	})
	if status != http.StatusOK {
		t.Fatalf("putCollectionDraft: status=%d body=%s", status, raw)
	}

	status, raw = publisherRequest(t, srv, http.MethodPost, draftPath+"/approve", full, map[string]any{"actor": "bob"})
	if status != http.StatusForbidden {
		t.Fatalf("approve with the same credential, different actor name: status=%d body=%s, want 403", status, raw)
	}
}

// TestPublisherCreateArtifactMaxFormats is the regression test for
// docs/security-review-publisher-design-260828.md finding 14's "maximum
// format count": createArtifact's formats array was previously unbounded.
func TestPublisherCreateArtifactMaxFormats(t *testing.T) {
	srv := newTestServer(t)
	cicd := createPublisherCredential(t, srv, "cicd-cred", model.PublisherScopeCICD)

	tooMany := make([]map[string]any, 51)
	for i := range tooMany {
		tooMany[i] = map[string]any{"mediaType": "application/vnd.cyclonedx+json"}
	}
	if status, body := publisherRequest(t, srv, http.MethodPost, "/publisher/v1/artifacts", cicd, map[string]any{
		"type": "BOM", "formats": tooMany,
	}); status != http.StatusBadRequest {
		t.Fatalf("51 formats: status=%d body=%s, want 400", status, body)
	}

	exactlyMax := make([]map[string]any, 50)
	for i := range exactlyMax {
		exactlyMax[i] = map[string]any{"mediaType": "application/vnd.cyclonedx+json"}
	}
	if status, body := publisherRequest(t, srv, http.MethodPost, "/publisher/v1/artifacts", cicd, map[string]any{
		"type": "BOM", "formats": exactlyMax,
	}); status != http.StatusCreated {
		t.Fatalf("50 formats (at the limit): status=%d body=%s, want 201", status, body)
	}
}

// TestPublisherUploadArtifactFileByMediaType covers security-review fix 14
// (docs/security-review-publisher-design-260828.md): uploadArtifactFile
// addresses a format by mediaType, not a positional index. Also confirms
// the ambiguous-mediaType and unknown-mediaType rejections, that the
// response now carries the computed checksum/size instead of a bare 204,
// and that a genuinely ambiguous mediaType -- still rejected on its own --
// is resolved by addressing the same format with its stable formatId
// instead (ArtifactCreated.FormatIDs).
func TestPublisherUploadArtifactFileByMediaType(t *testing.T) {
	srv := newTestServer(t)
	cicd := createPublisherCredential(t, srv, "cicd-cred", model.PublisherScopeCICD)

	status, raw := publisherRequest(t, srv, http.MethodPost, "/publisher/v1/artifacts", cicd, map[string]any{
		"type": "BOM",
		"formats": []map[string]any{
			{"mediaType": "application/vnd.cyclonedx+json"},
			{"mediaType": "application/vnd.cyclonedx+xml"},
			{"mediaType": "application/vnd.cyclonedx+xml"}, // duplicate on purpose, for the ambiguity case below
		},
	})
	if status != http.StatusCreated {
		t.Fatalf("createArtifact: status=%d body=%s", status, raw)
	}
	var created teapublisher.ArtifactCreated
	decodeInto(t, raw, &created)
	artifact := created.Artifact
	if len(created.FormatIDs) != 3 || created.FormatIDs[0] == "" || created.FormatIDs[1] == "" || created.FormatIDs[2] == "" {
		t.Fatalf("FormatIDs = %v, want 3 non-empty ids", created.FormatIDs)
	}
	if created.FormatIDs[1] == created.FormatIDs[2] {
		t.Fatalf("FormatIDs[1] == FormatIDs[2] (%q): the two duplicate-mediaType formats must still have distinct ids", created.FormatIDs[1])
	}

	uploadPath := "/publisher/v1/artifacts/" + artifact.UUID + "/1/files"

	// Unknown mediaType -> 404.
	if status, body := publisherUploadFile(t, srv, uploadPath, cicd, "application/does-not-exist", []byte("x")); status != http.StatusNotFound {
		t.Fatalf("unknown mediaType: status=%d body=%s, want 404", status, body)
	}
	// Ambiguous mediaType (two formats share it) -> 400, even now that
	// formatId exists -- mediaType addressing itself doesn't become
	// unambiguous just because an alternative exists.
	if status, body := publisherUploadFile(t, srv, uploadPath, cicd, "application/vnd.cyclonedx+xml", []byte("x")); status != http.StatusBadRequest {
		t.Fatalf("ambiguous mediaType: status=%d body=%s, want 400", status, body)
	}
	// Unambiguous mediaType -> 200, with the computed checksum/size, not a
	// bare 204.
	status, body := publisherUploadFile(t, srv, uploadPath, cicd, "application/vnd.cyclonedx+json", []byte(`{"ok":true}`))
	if status != http.StatusOK {
		t.Fatalf("upload by mediaType: status=%d body=%s, want 200", status, body)
	}
	var uploaded teapublisher.ArtifactFileUploaded
	decodeInto(t, body, &uploaded)
	wantSHA256Sum := sha256.Sum256([]byte(`{"ok":true}`))
	wantSHA256 := hex.EncodeToString(wantSHA256Sum[:])
	if uploaded.SHA256 != wantSHA256 || uploaded.Size != int64(len(`{"ok":true}`)) {
		t.Fatalf("uploaded = %+v, want sha256=%s size=%d", uploaded, wantSHA256, len(`{"ok":true}`))
	}

	// The genuinely ambiguous XML format IS now addressable -- by its
	// stable formatId instead of its shared mediaType -- resolving the
	// exact case the 400 above shows mediaType alone still can't.
	status, body = publisherUploadFileField(t, srv, uploadPath, cicd, "formatId", created.FormatIDs[2], []byte("<xml2/>"))
	if status != http.StatusOK {
		t.Fatalf("upload by formatId (ambiguous mediaType format): status=%d body=%s, want 200", status, body)
	}

	got, err := srv.repo.GetArtifactByVersion(t.Context(), artifact.UUID, 1)
	if err != nil {
		t.Fatalf("GetArtifactByVersion: %v", err)
	}
	// TEA 1.0 (spec/openapi.yaml): url is reserved for genuinely external
	// locations -- self-hosted content uploaded above must leave it empty.
	if got.Formats[0].URL != "" || len(got.Formats[0].Checksums) == 0 {
		t.Fatalf("json format (index 0) not populated as expected (want empty URL, real checksum): %+v", got.Formats[0])
	}
	// Formats[1] (the first xml duplicate) was never addressed by either
	// upload above -- still untouched. Formats[2] (addressed by formatId)
	// now has real content, proving the upload landed on the specific
	// format named by id, not merely "some xml format."
	if got.Formats[1].URL != "" || len(got.Formats[1].Checksums) != 0 {
		t.Fatalf("xml format (index 1) should be untouched: %+v", got.Formats[1])
	}
	if got.Formats[2].URL != "" || len(got.Formats[2].Checksums) == 0 {
		t.Fatalf("xml format (index 2) not populated as expected (want empty URL, real checksum): %+v", got.Formats[2])
	}
}

// TestPublisherCreateArtifactVersion is the regression test for
// docs/security-review-publisher-design-260828.md finding 4: there was no
// operation to add a new version to an already-existing artifact uuid,
// only mint a brand new identity. Covers the missing-uuid 404, a
// successful version-2 creation with completely different fields than
// version 1 (nothing carried forward), and the optional previousVersion
// optimistic-concurrency check (stale -> 409, correct -> 201).
func TestPublisherCreateArtifactVersion(t *testing.T) {
	srv := newTestServer(t)
	cicd := createPublisherCredential(t, srv, "cicd-cred", model.PublisherScopeCICD)

	// Unknown artifact uuid -> 404.
	unknownUUID := "00000000-0000-4000-8000-000000000000"
	if status, body := publisherRequest(t, srv, http.MethodPost, "/publisher/v1/artifacts/"+unknownUUID+"/versions", cicd, map[string]any{
		"type":    "BOM",
		"formats": []map[string]any{{"mediaType": "application/vnd.cyclonedx+json"}},
	}); status != http.StatusNotFound {
		t.Fatalf("createArtifactVersion for unknown uuid: status=%d body=%s, want 404", status, body)
	}

	status, raw := publisherRequest(t, srv, http.MethodPost, "/publisher/v1/artifacts", cicd, map[string]any{
		"name":    "sbom-draft.json",
		"type":    "BOM",
		"formats": []map[string]any{{"mediaType": "application/vnd.cyclonedx+json"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("createArtifact: status=%d body=%s", status, raw)
	}
	var v1 tea.Artifact
	decodeInto(t, raw, &v1)

	versionsPath := "/publisher/v1/artifacts/" + v1.UUID + "/versions"

	// A stale previousVersion is rejected with 409, not silently accepted.
	if status, body := publisherRequest(t, srv, http.MethodPost, versionsPath, cicd, map[string]any{
		"type":            "BOM",
		"formats":         []map[string]any{{"mediaType": "application/vnd.cyclonedx+xml"}},
		"previousVersion": 5,
	}); status != http.StatusConflict {
		t.Fatalf("createArtifactVersion with stale previousVersion: status=%d body=%s, want 409", status, body)
	}

	// The correct previousVersion (1) succeeds; nothing from v1 is carried
	// forward -- completely different name/type/format set.
	status, raw = publisherRequest(t, srv, http.MethodPost, versionsPath, cicd, map[string]any{
		"name":            "sbom-corrected.json",
		"type":            "VULNERABILITIES",
		"formats":         []map[string]any{{"mediaType": "application/vnd.cyclonedx+xml"}},
		"previousVersion": 1,
	})
	if status != http.StatusCreated {
		t.Fatalf("createArtifactVersion: status=%d body=%s", status, raw)
	}
	var v2 tea.Artifact
	decodeInto(t, raw, &v2)
	if v2.UUID != v1.UUID {
		t.Fatalf("v2.UUID = %q, want the same identity as v1 (%q)", v2.UUID, v1.UUID)
	}
	if v2.Version != 2 {
		t.Fatalf("v2.Version = %d, want 2", v2.Version)
	}
	if v2.Name != "sbom-corrected.json" || v2.Type != "VULNERABILITIES" || len(v2.Formats) != 1 || v2.Formats[0].MediaType != "application/vnd.cyclonedx+xml" {
		t.Fatalf("v2 = %+v, want fresh fields, nothing carried forward from v1", v2)
	}

	// v1 is independently still fetchable and untouched, via the real
	// /tea/v1 consumer API, not just the repo layer.
	teaStatus, _, teaBody := teaRequest(t, srv, http.MethodGet, "/tea/v1/artifact/"+v1.UUID+"/1", "")
	if teaStatus != http.StatusOK {
		t.Fatalf("GET /tea/v1 artifact v1: status=%d body=%s", teaStatus, teaBody)
	}
	var readBackV1 tea.Artifact
	decodeInto(t, teaBody, &readBackV1)
	if readBackV1.Name != "sbom-draft.json" || readBackV1.Type != "BOM" {
		t.Fatalf("readBackV1 = %+v, want v1 unchanged by creating v2", readBackV1)
	}

	// A missing type/no formats body is rejected the same way createArtifact's is.
	if status, body := publisherRequest(t, srv, http.MethodPost, versionsPath, cicd, map[string]any{
		"type": "BOM",
	}); status != http.StatusBadRequest {
		t.Fatalf("createArtifactVersion with no formats: status=%d body=%s, want 400", status, body)
	}
}

// TestPublisherUploadArtifactFileRejectedAfterEvidence covers
// security-review fix 1 (docs/security-review-publisher-design-260828.md):
// once evidence has been submitted for an artifact version, uploading a
// new file for it is rejected rather than silently invalidating the
// signature's meaning.
func TestPublisherUploadArtifactFileRejectedAfterEvidence(t *testing.T) {
	srv := newTestServer(t)
	cicd := createPublisherCredential(t, srv, "cicd-cred", model.PublisherScopeCICD)

	status, raw := publisherRequest(t, srv, http.MethodPost, "/publisher/v1/artifacts", cicd, map[string]any{
		"type":    "BOM",
		"formats": []map[string]any{{"mediaType": "application/vnd.cyclonedx+json"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("createArtifact: status=%d body=%s", status, raw)
	}
	var artifact tea.Artifact
	decodeInto(t, raw, &artifact)
	uploadPath := "/publisher/v1/artifacts/" + artifact.UUID + "/1/files"

	if status, body := publisherUploadFile(t, srv, uploadPath, cicd, "application/vnd.cyclonedx+json", []byte(`{"ok":true}`)); status != http.StatusOK {
		t.Fatalf("initial upload: status=%d body=%s, want 200", status, body)
	}

	status, raw = publisherRequest(t, srv, http.MethodPost, "/publisher/v1/artifacts/"+artifact.UUID+"/1/evidence/prepare", cicd, nil)
	if status != http.StatusOK {
		t.Fatalf("prepareArtifactEvidence: status=%d body=%s", status, raw)
	}
	var prepared struct {
		DigestToSign string `json:"digestToSign"`
	}
	decodeInto(t, raw, &prepared)
	sigValue, certPEM := signDigest(t, prepared.DigestToSign)

	status, raw = publisherRequest(t, srv, http.MethodPost, "/publisher/v1/artifacts/"+artifact.UUID+"/1/evidence", cicd, map[string]any{
		"objectDigestValue": prepared.DigestToSign,
		"signatureFormat":   "jws-detached",
		"signatureValue":    sigValue,
		"certificatePem":    certPEM,
	})
	if status != http.StatusCreated {
		t.Fatalf("submitArtifactEvidence: status=%d body=%s", status, raw)
	}

	// A second upload must now be rejected, not silently accepted.
	if status, body := publisherUploadFile(t, srv, uploadPath, cicd, "application/vnd.cyclonedx+json", []byte(`{"tampered":true}`)); status != http.StatusConflict {
		t.Fatalf("upload after evidence: status=%d body=%s, want 409", status, body)
	}
}

// TestPublisherUploadArtifactSignatureFile covers TEA 1.0 conformance's new
// local signature hosting: a detached signature uploaded via
// /publisher/v1's signature/files operation round-trips through the
// consumer API's signature download endpoint, and -- unlike content
// (TestPublisherUploadArtifactFileRejectedAfterEvidence) -- remains
// uploadable even after evidence has been submitted, since a signature
// upload never changes the content checksum evidence attests to.
func TestPublisherUploadArtifactSignatureFile(t *testing.T) {
	srv := newTestServer(t)
	cicd := createPublisherCredential(t, srv, "cicd-cred", model.PublisherScopeCICD)

	status, raw := publisherRequest(t, srv, http.MethodPost, "/publisher/v1/artifacts", cicd, map[string]any{
		"type":    "BOM",
		"formats": []map[string]any{{"mediaType": "application/vnd.cyclonedx+json"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("createArtifact: status=%d body=%s", status, raw)
	}
	var artifact tea.Artifact
	decodeInto(t, raw, &artifact)

	if status, body := publisherUploadFile(t, srv, "/publisher/v1/artifacts/"+artifact.UUID+"/1/files", cicd, "application/vnd.cyclonedx+json", []byte(`{"ok":true}`)); status != http.StatusOK {
		t.Fatalf("content upload: status=%d body=%s, want 200", status, body)
	}

	sigPath := "/publisher/v1/artifacts/" + artifact.UUID + "/1/signature/files"

	// Unknown mediaType -> 404, same selection rule as content.
	if status, body := publisherUploadFile(t, srv, sigPath, cicd, "application/does-not-exist", []byte("x")); status != http.StatusNotFound {
		t.Fatalf("unknown mediaType: status=%d body=%s, want 404", status, body)
	}

	signature := []byte("fake-detached-signature-bytes")
	if status, body := publisherUploadFile(t, srv, sigPath, cicd, "application/vnd.cyclonedx+json", signature); status != http.StatusOK {
		t.Fatalf("signature upload: status=%d body=%s, want 200", status, body)
	}

	// Prepare and submit evidence for the artifact -- content is now
	// frozen, but the signature must remain uploadable.
	status, raw = publisherRequest(t, srv, http.MethodPost, "/publisher/v1/artifacts/"+artifact.UUID+"/1/evidence/prepare", cicd, nil)
	if status != http.StatusOK {
		t.Fatalf("prepareArtifactEvidence: status=%d body=%s", status, raw)
	}
	var prepared struct {
		DigestToSign string `json:"digestToSign"`
	}
	decodeInto(t, raw, &prepared)
	sigValue, certPEM := signDigest(t, prepared.DigestToSign)
	status, raw = publisherRequest(t, srv, http.MethodPost, "/publisher/v1/artifacts/"+artifact.UUID+"/1/evidence", cicd, map[string]any{
		"objectDigestValue": prepared.DigestToSign,
		"signatureFormat":   "jws-detached",
		"signatureValue":    sigValue,
		"certificatePem":    certPEM,
	})
	if status != http.StatusCreated {
		t.Fatalf("submitArtifactEvidence: status=%d body=%s", status, raw)
	}
	replacementSig := []byte("replacement-signature-bytes")
	if status, body := publisherUploadFile(t, srv, sigPath, cicd, "application/vnd.cyclonedx+json", replacementSig); status != http.StatusOK {
		t.Fatalf("signature upload after evidence: status=%d body=%s, want 200 (unlike content, not frozen)", status, body)
	}

	// Round-trip via the consumer API's own signature download endpoint.
	dlResp, err := http.Get(srv.URL + "/tea/v1/artifact/" + artifact.UUID + "/latest/signature/download?mediaType=" + url.QueryEscape("application/vnd.cyclonedx+json"))
	if err != nil {
		t.Fatalf("GET signature download: %v", err)
	}
	dlBody, _ := io.ReadAll(dlResp.Body)
	_ = dlResp.Body.Close()
	if dlResp.StatusCode != http.StatusOK {
		t.Fatalf("GET signature download: status=%d body=%s", dlResp.StatusCode, dlBody)
	}
	if string(dlBody) != string(replacementSig) {
		t.Fatalf("downloaded signature = %q, want %q (the replacement, most recent upload)", dlBody, replacementSig)
	}
}

// TestPublisherCreateComponentIdentifierConflict covers security-review fix
// 12 (docs/security-review-publisher-design-260828.md): createComponent
// enforces identifier uniqueness server-side rather than relying on
// find-before-create, and (v0.16, this session) the 409 body is the
// existing component itself, not just a message -- so a caller doesn't have
// to separately re-search for what it just collided with.
func TestPublisherCreateComponentIdentifierConflict(t *testing.T) {
	srv := newTestServer(t)
	full := createPublisherCredential(t, srv, "full-cred", model.PublisherScopeFull)

	body := map[string]any{
		"name":        "acme-widget-core",
		"identifiers": []map[string]any{{"idType": "PURL", "idValue": "pkg:generic/acme-widget-core"}},
	}
	status, raw := publisherRequest(t, srv, http.MethodPost, "/publisher/v1/components", full, body)
	if status != http.StatusCreated {
		t.Fatalf("first createComponent: status=%d body=%s", status, raw)
	}
	var first tea.Component
	decodeInto(t, raw, &first)

	// Same identifier, different name -- still a conflict; the 409 body is
	// the first component (its real uuid/name), not the second request's.
	body["name"] = "acme-widget-core-fork"
	status, raw = publisherRequest(t, srv, http.MethodPost, "/publisher/v1/components", full, body)
	if status != http.StatusConflict {
		t.Fatalf("duplicate identifier: status=%d body=%s, want 409", status, raw)
	}
	var conflicting tea.Component
	decodeInto(t, raw, &conflicting)
	if conflicting.UUID != first.UUID || conflicting.Name != "acme-widget-core" {
		t.Fatalf("409 body = %+v, want the existing component %+v", conflicting, first)
	}

	// No identifiers at all -- nothing to conflict on, both succeed.
	if status, raw := publisherRequest(t, srv, http.MethodPost, "/publisher/v1/components", full, map[string]any{"name": "no-identifiers-a"}); status != http.StatusCreated {
		t.Fatalf("no-identifiers create 1: status=%d body=%s", status, raw)
	}
	if status, raw := publisherRequest(t, srv, http.MethodPost, "/publisher/v1/components", full, map[string]any{"name": "no-identifiers-a"}); status != http.StatusCreated {
		t.Fatalf("no-identifiers create 2: status=%d body=%s", status, raw)
	}
}

// TestPublisherFindComponentsHasNext is the regression test for
// docs/security-review-publisher-design-260828.md finding 12's "paginate
// and constrain component search": findComponents used to return a bare
// array, silently truncated at the server's own fixed limit with no way
// for a caller to tell more existed. Creates one more component than
// internal/publisher.findComponentsLimit allows and confirms hasNext is set.
func TestPublisherFindComponentsHasNext(t *testing.T) {
	srv := newTestServer(t)
	full := createPublisherCredential(t, srv, "full-cred", model.PublisherScopeFull)

	const limit = 1000 // internal/publisher.findComponentsLimit
	for i := 0; i < limit+1; i++ {
		body := map[string]any{"name": "dup-search-target"}
		if status, raw := publisherRequest(t, srv, http.MethodPost, "/publisher/v1/components", full, body); status != http.StatusCreated {
			t.Fatalf("createComponent %d: status=%d body=%s", i, status, raw)
		}
	}

	status, raw := publisherRequest(t, srv, http.MethodGet, "/publisher/v1/components?q=dup-search-target", full, nil)
	if status != http.StatusOK {
		t.Fatalf("findComponents: status=%d body=%s", status, raw)
	}
	var results teapublisher.ComponentSearchResults
	decodeInto(t, raw, &results)
	if len(results.Results) != limit {
		t.Fatalf("len(results) = %d, want %d (capped at the limit)", len(results.Results), limit)
	}
	if !results.HasNext {
		t.Fatal("hasNext = false, want true: limit+1 matching components exist")
	}
}

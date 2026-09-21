// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/openteapublisher"
	"github.com/oej/opentea/pkg/teapublisher"
	"github.com/oej/opentea/pkg/teapublisherclient"
)

// TestCICDAPIProxiesToRealTarget is the real payoff proof for
// design/publisher-service.md §18.11: it stands up a real
// internal/publisher server (the "target"), a real opentea-publisher
// server pointed at it via a stored Target holding a full-scoped
// credential, mints opentea-publisher's own, narrower cicd_credential,
// and drives an artifact + a collection-draft operation through
// opentea-publisher's /cicdapi/v1 -- confirming the *target* actually
// received and processed each call (fetched back via a full-scoped
// pkg/teapublisherclient afterward), not just that opentea-publisher
// itself returned 200. This lives here, not in
// internal/openteapublisher's own test suite, because internal/publisher
// has no test-server helper of its own -- this package's newTestServer/
// createPublisherCredential (publisher_test.go) are the only ones that
// exist, the same reason cmd/opentea/publisherclient_test.go already
// crosses this exact package boundary.
func TestCICDAPIProxiesToRealTarget(t *testing.T) {
	target := newTestServer(t)
	fullToken := createPublisherCredential(t, target, "full-cred", model.PublisherScopeFull)
	ctx := context.Background()

	fullClient := teapublisherclient.NewClient(target.URL+"/publisher/v1", fullToken)
	product, err := fullClient.CreateProduct(ctx, teapublisher.ProductCreate{Name: "Acme Widget"})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	release, err := fullClient.CreateProductRelease(ctx, product.UUID, teapublisher.ProductReleaseCreate{
		Version:     "1.0.0",
		CreatedDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateProductRelease: %v", err)
	}

	sqlDB, err := openteapublisher.Open(filepath.Join(t.TempDir(), "publisher.db"))
	if err != nil {
		t.Fatalf("openteapublisher.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	r := openteapublisher.New(sqlDB)

	openteaTarget, err := r.CreateTarget(ctx, "test target", target.URL+"/publisher/v1", fullToken)
	if err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}
	_, cicdToken, err := r.CreateCICDCredential(ctx, openteaTarget.UUID, "CI pipeline")
	if err != nil {
		t.Fatalf("CreateCICDCredential: %v", err)
	}

	opSrv := httptest.NewServer(openteapublisher.NewRouter(r, openteapublisher.Config{RootURL: "http://opentea-publisher.example"}))
	t.Cleanup(opSrv.Close)

	// Artifact create, through the proxy.
	status, artifactBody := cicdJSONRequest(t, opSrv, cicdToken, http.MethodPost, "/cicdapi/v1/artifacts", teapublisher.ArtifactCreate{
		Type:    "BOM",
		Formats: []teapublisher.ArtifactFormatCreate{{MediaType: "application/vnd.cyclonedx+json"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("cicdapi CreateArtifact: status=%d body=%s", status, artifactBody)
	}
	var artifact minimalArtifact
	if err := json.Unmarshal(artifactBody, &artifact); err != nil {
		t.Fatalf("unmarshal artifact: %v (body: %s)", err, artifactBody)
	}
	if artifact.UUID == "" {
		t.Fatalf("artifact response missing uuid: %s", artifactBody)
	}

	// Collection-draft PUT, through the proxy.
	status, draftBody := cicdJSONRequest(t, opSrv, cicdToken, http.MethodPut, "/cicdapi/v1/productReleases/"+release.UUID+"/collectionDraft", teapublisher.CollectionDraftArtifactList{
		Actor:     "ci-pipeline",
		Artifacts: []teapublisher.ArtifactVersionRef{{UUID: artifact.UUID, Version: 1}},
	})
	if status != http.StatusOK {
		t.Fatalf("cicdapi PutCollectionDraft: status=%d body=%s", status, draftBody)
	}
	if !strings.Contains(string(draftBody), artifact.UUID) {
		t.Fatalf("draft response doesn't mention the artifact: %s", draftBody)
	}

	// Confirm the *target* really has both -- not just that
	// opentea-publisher said 200. Fetched with the full-scoped credential
	// directly against the target, bypassing opentea-publisher entirely.
	draft, err := fullClient.GetProductReleaseCollectionDraft(ctx, release.UUID)
	if err != nil {
		t.Fatalf("target GetProductReleaseCollectionDraft: %v", err)
	}
	if len(draft.Artifacts) != 1 || draft.Artifacts[0].UUID != artifact.UUID {
		t.Fatalf("target's own collection draft = %+v, want the proxied artifact", draft)
	}
}

// TestCICDAPISignatureUploadProxiesToRealTarget covers the new
// /cicdapi/v1/.../signature/files proxy operation (added alongside the
// consumer-API artifact download endpoints, TEA 1.0 conformance) with the
// same "prove it against the real other side" standard as
// TestCICDAPIProxiesToRealTarget above: uploads a detached signature
// through opentea-publisher's proxy, then confirms it landed on the real
// target server by downloading it back through the target's own consumer
// API (/tea/v1/.../signature/download), not just trusting the proxy's 204.
func TestCICDAPISignatureUploadProxiesToRealTarget(t *testing.T) {
	target := newTestServer(t)
	fullToken := createPublisherCredential(t, target, "full-cred", model.PublisherScopeFull)
	ctx := context.Background()

	fullClient := teapublisherclient.NewClient(target.URL+"/publisher/v1", fullToken)
	artifact, err := fullClient.CreateArtifact(ctx, teapublisher.ArtifactCreate{
		Type:    "BOM",
		Formats: []teapublisher.ArtifactFormatCreate{{MediaType: "application/vnd.cyclonedx+json"}},
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	sqlDB, err := openteapublisher.Open(filepath.Join(t.TempDir(), "publisher.db"))
	if err != nil {
		t.Fatalf("openteapublisher.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	r := openteapublisher.New(sqlDB)

	openteaTarget, err := r.CreateTarget(ctx, "test target", target.URL+"/publisher/v1", fullToken)
	if err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}
	_, cicdToken, err := r.CreateCICDCredential(ctx, openteaTarget.UUID, "CI pipeline")
	if err != nil {
		t.Fatalf("CreateCICDCredential: %v", err)
	}

	opSrv := httptest.NewServer(openteapublisher.NewRouter(r, openteapublisher.Config{RootURL: "http://opentea-publisher.example"}))
	t.Cleanup(opSrv.Close)

	signature := []byte("fake-detached-signature-bytes")
	status, body := cicdUploadFile(t, opSrv, cicdToken, "/cicdapi/v1/artifacts/"+artifact.UUID+"/1/signature/files", "application/vnd.cyclonedx+json", signature)
	if status != http.StatusNoContent {
		t.Fatalf("cicdapi signature upload: status=%d body=%s", status, body)
	}

	dlResp, err := http.Get(target.URL + "/tea/v1/artifact/" + artifact.UUID + "/latest/signature/download?mediaType=" + url.QueryEscape("application/vnd.cyclonedx+json"))
	if err != nil {
		t.Fatalf("GET signature download against target: %v", err)
	}
	dlBody, _ := io.ReadAll(dlResp.Body)
	_ = dlResp.Body.Close()
	if dlResp.StatusCode != http.StatusOK {
		t.Fatalf("GET signature download against target: status=%d body=%s", dlResp.StatusCode, dlBody)
	}
	if string(dlBody) != string(signature) {
		t.Fatalf("downloaded signature = %q, want %q", dlBody, signature)
	}
}

// cicdUploadFile POSTs a multipart file upload (with a "mediaType" form
// field) to an opentea-publisher /cicdapi/v1 server, mirroring
// publisher_test.go's publisherUploadFile but for a bare *httptest.Server
// (opentea-publisher's own test servers aren't wrapped in this package's
// testServer type).
func cicdUploadFile(t *testing.T, srv *httptest.Server, token, path, mediaType string, content []byte) (int, []byte) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("mediaType", mediaType); err != nil {
		t.Fatalf("write mediaType field: %v", err)
	}
	part, err := w.CreateFormFile("file", "signature.bin")
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
	req.Header.Set("Authorization", "Bearer "+token)
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

// TestCICDAPIRejectsFullScopedOperationEvenIfAttempted confirms an
// operation the /cicdapi/v1 mux doesn't expose fails structurally, even
// with a *valid* cicd token -- covered more thoroughly (every excluded
// path, with only a nonsense token) by
// internal/openteapublisher's own TestCICDAPIRouteSurfaceExcludesFullScopedOperations;
// this confirms a real, currently-valid credential doesn't change the
// outcome, since that route simply isn't registered.
func TestCICDAPIRejectsFullScopedOperationEvenIfAttempted(t *testing.T) {
	sqlDB, err := openteapublisher.Open(filepath.Join(t.TempDir(), "publisher.db"))
	if err != nil {
		t.Fatalf("openteapublisher.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	r := openteapublisher.New(sqlDB)

	ctx := context.Background()
	tgt, err := r.CreateTarget(ctx, "test target", "https://tea.example.com/publisher/v1", "irrelevant")
	if err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}
	_, cicdToken, err := r.CreateCICDCredential(ctx, tgt.UUID, "CI pipeline")
	if err != nil {
		t.Fatalf("CreateCICDCredential: %v", err)
	}

	opSrv := httptest.NewServer(openteapublisher.NewRouter(r, openteapublisher.Config{RootURL: "http://opentea-publisher.example"}))
	t.Cleanup(opSrv.Close)

	status, body := cicdJSONRequest(t, opSrv, cicdToken, http.MethodPost, "/cicdapi/v1/products", map[string]any{"name": "x"})
	if status != http.StatusNotFound {
		t.Fatalf("POST /cicdapi/v1/products with a valid cicd token: status=%d body=%s, want 404 (route doesn't exist)", status, body)
	}
}

// minimalArtifact is a minimal local decode target for the artifact-create
// response -- avoids importing pkg/tea just for one field in this test
// file (uuid is all that's needed to chain into the next call).
type minimalArtifact struct {
	UUID string `json:"uuid"`
}

func cicdJSONRequest(t *testing.T, srv *httptest.Server, token, method, path string, body any) (int, []byte) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp.StatusCode, respBody
}

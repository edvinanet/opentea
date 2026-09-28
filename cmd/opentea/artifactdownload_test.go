// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/pkg/tea"
)

// TestArtifactDownloadSelfHostedContent covers the core download-endpoint
// machinery (TEA 1.0, spec/openapi.yaml) against real self-hosted content
// uploaded via /admin/v1: 200 with the expected headers, HEAD returning the
// same headers with no body, a matching If-None-Match producing 304, and
// the versioned ("/1/download") and "latest" endpoints agreeing.
func TestArtifactDownloadSelfHostedContent(t *testing.T) {
	srv := newTestServer(t)

	status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/artifacts", map[string]any{
		"type":        "BOM",
		"createdDate": "2026-07-01T00:00:00Z",
		"formats":     []map[string]any{{"mediaType": "application/vnd.cyclonedx+json"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("create artifact: status=%d body=%s", status, raw)
	}
	var artifact tea.Artifact
	decodeInto(t, raw, &artifact)

	content := []byte(`{"bomFormat":"CycloneDX"}`)
	status, raw = uploadFile(t, srv, "/admin/v1/artifacts/"+artifact.UUID+"/1/files", "sbom.json", content, "application/vnd.cyclonedx+json")
	if status != http.StatusOK {
		t.Fatalf("upload artifact file: status=%d body=%s", status, raw)
	}

	q := "?mediaType=" + url.QueryEscape("application/vnd.cyclonedx+json")

	resp, err := http.Get(srv.URL + "/tea/v1/artifact/" + artifact.UUID + "/latest/download" + q)
	if err != nil {
		t.Fatalf("GET download: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET download: status=%d body=%s", resp.StatusCode, body)
	}
	if string(body) != string(content) {
		t.Fatalf("body = %q, want %q", body, content)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/vnd.cyclonedx+json" {
		t.Fatalf("Content-Type = %q, want application/vnd.cyclonedx+json", ct)
	}
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("ETag header missing")
	}
	if cl := resp.Header.Get("Content-Location"); cl == "" {
		t.Fatal("Content-Location header missing")
	}
	if cc := resp.Header.Get("Cache-Control"); cc == "" {
		t.Fatal("Cache-Control header missing")
	}
	sum := sha256.Sum256(content)
	wantReprDigest := "sha-256=:" + base64.StdEncoding.EncodeToString(sum[:]) + ":"
	if rd := resp.Header.Get("Repr-Digest"); rd != wantReprDigest {
		t.Fatalf("Repr-Digest = %q, want %q (RFC 9530, computed from the real uploaded bytes)", rd, wantReprDigest)
	}

	// HEAD: same headers, no body.
	headReq, err := http.NewRequest(http.MethodHead, srv.URL+"/tea/v1/artifact/"+artifact.UUID+"/latest/download"+q, nil)
	if err != nil {
		t.Fatalf("new HEAD request: %v", err)
	}
	headResp, err := http.DefaultClient.Do(headReq)
	if err != nil {
		t.Fatalf("HEAD download: %v", err)
	}
	headBody, _ := io.ReadAll(headResp.Body)
	_ = headResp.Body.Close()
	if headResp.StatusCode != http.StatusOK {
		t.Fatalf("HEAD download: status=%d", headResp.StatusCode)
	}
	if len(headBody) != 0 {
		t.Fatalf("HEAD body = %q, want empty", headBody)
	}
	if headResp.Header.Get("ETag") != etag {
		t.Fatalf("HEAD ETag = %q, want %q", headResp.Header.Get("ETag"), etag)
	}

	// Conditional: matching If-None-Match -> 304.
	condReq, err := http.NewRequest(http.MethodGet, srv.URL+"/tea/v1/artifact/"+artifact.UUID+"/latest/download"+q, nil)
	if err != nil {
		t.Fatalf("new conditional request: %v", err)
	}
	condReq.Header.Set("If-None-Match", etag)
	condResp, err := http.DefaultClient.Do(condReq)
	if err != nil {
		t.Fatalf("conditional GET: %v", err)
	}
	_ = condResp.Body.Close()
	if condResp.StatusCode != http.StatusNotModified {
		t.Fatalf("conditional GET status = %d, want 304", condResp.StatusCode)
	}
	if rd := condResp.Header.Get("Repr-Digest"); rd != wantReprDigest {
		t.Fatalf("304 Repr-Digest = %q, want %q (representation metadata headers apply to 304 too)", rd, wantReprDigest)
	}

	// The versioned endpoint agrees with "latest".
	verResp, err := http.Get(srv.URL + "/tea/v1/artifact/" + artifact.UUID + "/1/download" + q)
	if err != nil {
		t.Fatalf("GET versioned download: %v", err)
	}
	verBody, _ := io.ReadAll(verResp.Body)
	_ = verResp.Body.Close()
	if verResp.StatusCode != http.StatusOK || string(verBody) != string(content) {
		t.Fatalf("versioned download: status=%d body=%q", verResp.StatusCode, verBody)
	}
}

// TestLatestArtifactDownloadRequiresRevalidation is the regression test
// for the finding that "latest" content/signature downloads used the same
// Cache-Control as everything else (a 60s freshness window plus
// stale-while-revalidate), even though "latest" resolves to a different
// revision the instant a new one is published and the spec requires
// revalidation before reuse specifically because of that
// (docs/security-review-260923.md finding #13) -- stale-while-revalidate's
// entire purpose is avoiding synchronous revalidation, which is exactly
// what's forbidden here. Confirms both /latest endpoints now use
// `no-cache` with no `stale-while-revalidate`/`immutable`/`max-age`, and
// that the *versioned* endpoint (a deliberately separate, unaffected
// tradeoff -- see downloadArtifactByVersion's own doc comment) is
// untouched by this fix.
func TestLatestArtifactDownloadRequiresRevalidation(t *testing.T) {
	srv := newTestServer(t)

	status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/artifacts", map[string]any{
		"type":        "BOM",
		"createdDate": "2026-07-01T00:00:00Z",
		"formats":     []map[string]any{{"mediaType": "application/vnd.cyclonedx+json"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("create artifact: status=%d body=%s", status, raw)
	}
	var artifact tea.Artifact
	decodeInto(t, raw, &artifact)

	content := []byte(`{"bomFormat":"CycloneDX"}`)
	if status, raw := uploadFile(t, srv, "/admin/v1/artifacts/"+artifact.UUID+"/1/files", "sbom.json", content, "application/vnd.cyclonedx+json"); status != http.StatusOK {
		t.Fatalf("upload content: status=%d body=%s", status, raw)
	}
	signature := []byte("fake-detached-signature-bytes")
	if status, raw := uploadFile(t, srv, "/admin/v1/artifacts/"+artifact.UUID+"/1/signature", "sbom.sig", signature, "application/octet-stream"); status != http.StatusOK {
		t.Fatalf("upload signature: status=%d body=%s", status, raw)
	}

	assertLatestCacheControl := func(t *testing.T, path string) {
		t.Helper()
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: status=%d, want 200", path, resp.StatusCode)
		}
		cc := resp.Header.Get("Cache-Control")
		if !strings.Contains(cc, "no-cache") {
			t.Fatalf("GET %s: Cache-Control = %q, want it to contain \"no-cache\"", path, cc)
		}
		for _, forbidden := range []string{"stale-while-revalidate", "immutable", "max-age"} {
			if strings.Contains(cc, forbidden) {
				t.Fatalf("GET %s: Cache-Control = %q, must not contain %q (the spec requires synchronous revalidation for \"latest\")", path, cc, forbidden)
			}
		}
	}
	assertLatestCacheControl(t, "/tea/v1/artifact/"+artifact.UUID+"/latest/download")
	assertLatestCacheControl(t, "/tea/v1/artifact/"+artifact.UUID+"/latest/signature/download")

	// The versioned endpoint is a deliberately separate tradeoff, unaffected.
	verResp, err := http.Get(srv.URL + "/tea/v1/artifact/" + artifact.UUID + "/1/download")
	if err != nil {
		t.Fatalf("GET versioned download: %v", err)
	}
	_ = verResp.Body.Close()
	if cc := verResp.Header.Get("Cache-Control"); !strings.Contains(cc, "stale-while-revalidate") {
		t.Fatalf("versioned download Cache-Control = %q, want it unaffected (still stale-while-revalidate)", cc)
	}
}

// TestArtifactDownloadErrorCases covers the spec's distinct error
// conditions: 406 for a mediaType matching no format, 404 (existence-
// hiding, OBJECT_UNKNOWN) for a concealed/nonexistent artifact -- extending
// to the signature sub-resource too -- and 404 SIGNATURE_NOT_FOUND
// (deliberately not existence-hiding) for a real format with no signature.
func TestArtifactDownloadErrorCases(t *testing.T) {
	srv := newTestServer(t)

	status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/artifacts", map[string]any{
		"type":        "BOM",
		"createdDate": "2026-07-01T00:00:00Z",
		"formats":     []map[string]any{{"mediaType": "application/vnd.cyclonedx+json"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("create artifact: status=%d body=%s", status, raw)
	}
	var artifact tea.Artifact
	decodeInto(t, raw, &artifact)

	status, raw = uploadFile(t, srv, "/admin/v1/artifacts/"+artifact.UUID+"/1/files", "sbom.json", []byte(`{}`), "application/vnd.cyclonedx+json")
	if status != http.StatusOK {
		t.Fatalf("upload artifact file: status=%d body=%s", status, raw)
	}

	// 406: mediaType matches no format.
	resp, err := http.Get(srv.URL + "/tea/v1/artifact/" + artifact.UUID + "/latest/download?mediaType=" + url.QueryEscape("application/does-not-exist"))
	if err != nil {
		t.Fatalf("GET 406 case: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotAcceptable {
		t.Fatalf("status = %d, want 406, body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "NO_ACCEPTABLE_FORMAT") {
		t.Fatalf("body = %s, want NO_ACCEPTABLE_FORMAT", body)
	}

	// 404 OBJECT_UNKNOWN: nonexistent artifact, both for content and signature.
	nonexistent := "00000000-0000-4000-8000-000000000000"
	for _, path := range []string{
		"/tea/v1/artifact/" + nonexistent + "/latest/download",
		"/tea/v1/artifact/" + nonexistent + "/latest/signature/download",
	} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %s: status = %d, want 404, body=%s", path, resp.StatusCode, body)
		}
		if !strings.Contains(string(body), "OBJECT_UNKNOWN") {
			t.Fatalf("GET %s: body = %s, want OBJECT_UNKNOWN", path, body)
		}
	}

	// 404 SIGNATURE_NOT_FOUND: real format, no signature published -- not
	// existence-hiding, a distinct spec-defined signal.
	sigResp, err := http.Get(srv.URL + "/tea/v1/artifact/" + artifact.UUID + "/latest/signature/download?mediaType=" + url.QueryEscape("application/vnd.cyclonedx+json"))
	if err != nil {
		t.Fatalf("GET signature: %v", err)
	}
	sigBody, _ := io.ReadAll(sigResp.Body)
	_ = sigResp.Body.Close()
	if sigResp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET signature: status = %d, want 404, body=%s", sigResp.StatusCode, sigBody)
	}
	if !strings.Contains(string(sigBody), "SIGNATURE_NOT_FOUND") {
		t.Fatalf("GET signature: body = %s, want SIGNATURE_NOT_FOUND", sigBody)
	}
}

// TestArtifactDownloadContentNegotiation is the regression test for the
// finding that Accept-header format negotiation stripped everything after
// the first ";" without checking the q-value, so a format explicitly
// marked unacceptable (q=0) could still be selected, an application/*
// wildcard range never matched anything, and a higher-quality-weighted
// alternative was ignored in favor of whichever format happened to appear
// first (docs/security-review-260923.md finding #12) -- each case below
// reproduces exactly one of the review's own three examples.
func TestArtifactDownloadContentNegotiation(t *testing.T) {
	srv := newTestServer(t)

	status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/artifacts", map[string]any{
		"type":        "BOM",
		"createdDate": "2026-07-01T00:00:00Z",
		"formats": []map[string]any{
			{"mediaType": "application/vnd.cyclonedx+json"},
			{"mediaType": "application/vnd.cyclonedx+xml"},
		},
	})
	if status != http.StatusCreated {
		t.Fatalf("create artifact: status=%d body=%s", status, raw)
	}
	var artifact tea.Artifact
	decodeInto(t, raw, &artifact)

	if status, raw := uploadFile(t, srv, "/admin/v1/artifacts/"+artifact.UUID+"/1/files?formatIndex=0", "sbom.json", []byte(`{"json":true}`), "application/vnd.cyclonedx+json"); status != http.StatusOK {
		t.Fatalf("upload json format: status=%d body=%s", status, raw)
	}
	if status, raw := uploadFile(t, srv, "/admin/v1/artifacts/"+artifact.UUID+"/1/files?formatIndex=1", "sbom.xml", []byte(`<xml/>`), "application/vnd.cyclonedx+xml"); status != http.StatusOK {
		t.Fatalf("upload xml format: status=%d body=%s", status, raw)
	}

	getWithAccept := func(t *testing.T, accept string) (int, string, http.Header) {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, srv.URL+"/tea/v1/artifact/"+artifact.UUID+"/latest/download", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Accept", accept)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body), resp.Header
	}

	t.Run("q=0 explicitly excludes a format that would otherwise be the only candidate", func(t *testing.T) {
		// Only the json format exists in this Accept header at all, and
		// it's explicitly marked unacceptable -- must 406, not silently
		// serve it anyway.
		status, body, _ := getWithAccept(t, "application/vnd.cyclonedx+json;q=0")
		if status != http.StatusNotAcceptable {
			t.Fatalf("status = %d, want 406, body=%s", status, body)
		}
	})

	t.Run("application/* wildcard matches a subtype it didn't name explicitly", func(t *testing.T) {
		status, _, headers := getWithAccept(t, "application/*")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		if ct := headers.Get("Content-Type"); ct != "application/vnd.cyclonedx+json" && ct != "application/vnd.cyclonedx+xml" {
			t.Fatalf("Content-Type = %q, want one of the two application/* formats", ct)
		}
	})

	t.Run("higher quality weight wins even when it's not the first format", func(t *testing.T) {
		status, _, headers := getWithAccept(t, "application/vnd.cyclonedx+json;q=0.5, application/vnd.cyclonedx+xml;q=0.9")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		if ct := headers.Get("Content-Type"); ct != "application/vnd.cyclonedx+xml" {
			t.Fatalf("Content-Type = %q, want application/vnd.cyclonedx+xml (higher q)", ct)
		}
	})

	// Signature downloads must ignore Accept entirely (this parameter's
	// own spec text: "the server selects a format of its choice", not
	// Accept-driven). Publishes a signature for the json format only,
	// then sends an Accept header that unambiguously prefers xml -- under
	// ordinary content negotiation this would select xml (which has no
	// signature, 404), but the signature endpoint must ignore Accept and
	// pick json (its first/only real candidate) regardless, succeeding.
	t.Run("signature download ignores Accept entirely", func(t *testing.T) {
		signature := []byte("fake-detached-signature-bytes")
		status, raw := uploadFile(t, srv, "/admin/v1/artifacts/"+artifact.UUID+"/1/signature?formatIndex=0", "sbom.sig", signature, "application/octet-stream")
		if status != http.StatusOK {
			t.Fatalf("upload signature for json format: status=%d body=%s", status, raw)
		}

		req, err := http.NewRequest(http.MethodGet, srv.URL+"/tea/v1/artifact/"+artifact.UUID+"/latest/signature/download", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Accept", "application/vnd.cyclonedx+xml")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200 (Accept preferring xml must not steer this away from json's published signature), body=%s", resp.StatusCode, body)
		}
		if !strings.Contains(resp.Header.Get("Content-Location"), url.QueryEscape("application/vnd.cyclonedx+json")) {
			t.Fatalf("Content-Location = %q, want the json format selected despite Accept preferring xml", resp.Header.Get("Content-Location"))
		}
		if !bytes.Equal(body, signature) {
			t.Fatalf("body = %q, want the json format's published signature", body)
		}
	})
}

// TestArtifactDownloadExternalRedirect covers the 302 redirect for
// externally-hosted content -- unconditional, never gated by
// If-None-Match, since redirect is outside conditional-request semantics
// per the spec. External-url formats aren't reachable through /admin/v1's
// create-then-upload flow at all (confirmed: ArtifactFormatInput carries no
// URL field, so a "url" in the create JSON body is silently ignored) --
// only bundle import (internal/repo.ImportArtifact) can produce one, so
// this seeds it directly through the repo, the same way
// internal/bundle/import.go itself would.
func TestArtifactDownloadExternalRedirect(t *testing.T) {
	srv := newTestServer(t)
	ctx := t.Context()

	created, err := srv.repo.ImportArtifact(ctx, repo.ImportArtifactInput{
		UUID: "11111111-1111-4111-8111-111111111111", Version: 1, Type: "BOM",
		Formats: []repo.ImportArtifactFormatInput{
			{MediaType: "application/vnd.cyclonedx+json", URL: "https://example.com/external-sbom.json"},
		},
	})
	if err != nil || !created {
		t.Fatalf("ImportArtifact: created=%v err=%v", created, err)
	}

	noRedirect := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	dlResp, err := noRedirect.Get(srv.URL + "/tea/v1/artifact/11111111-1111-4111-8111-111111111111/latest/download?mediaType=" + url.QueryEscape("application/vnd.cyclonedx+json"))
	if err != nil {
		t.Fatalf("GET external download: %v", err)
	}
	_ = dlResp.Body.Close()
	if dlResp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", dlResp.StatusCode)
	}
	if loc := dlResp.Header.Get("Location"); loc != "https://example.com/external-sbom.json" {
		t.Fatalf("Location = %q, want the external url", loc)
	}
	if rd := dlResp.Header.Get("Repr-Digest"); rd != "" {
		t.Fatalf("Repr-Digest = %q, want absent -- this server has no local representation to digest for an external redirect", rd)
	}

	// The redirect is unconditional: even a (deliberately bogus, since
	// external content has no server-known ETag to match) If-None-Match
	// doesn't turn it into a 304 -- redirect sits outside conditional-
	// request semantics per the spec.
	condReq, err := http.NewRequest(http.MethodGet, srv.URL+"/tea/v1/artifact/11111111-1111-4111-8111-111111111111/latest/download?mediaType="+url.QueryEscape("application/vnd.cyclonedx+json"), nil)
	if err != nil {
		t.Fatalf("new conditional request: %v", err)
	}
	condReq.Header.Set("If-None-Match", `"bogus"`)
	condResp, err := noRedirect.Do(condReq)
	if err != nil {
		t.Fatalf("conditional GET: %v", err)
	}
	_ = condResp.Body.Close()
	if condResp.StatusCode != http.StatusFound {
		t.Fatalf("conditional GET on external url: status = %d, want 302 (redirect isn't conditional)", condResp.StatusCode)
	}
}

// TestArtifactSignatureUploadAndDownloadAdmin proves local signature
// hosting end to end through /admin/v1: upload a detached signature,
// retrieve it via the signature download endpoint, and confirm content and
// signature bytes are independently addressable (uploading one doesn't
// disturb the other).
func TestArtifactSignatureUploadAndDownloadAdmin(t *testing.T) {
	srv := newTestServer(t)

	status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/artifacts", map[string]any{
		"type":        "BOM",
		"createdDate": "2026-07-01T00:00:00Z",
		"formats":     []map[string]any{{"mediaType": "application/vnd.cyclonedx+json"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("create artifact: status=%d body=%s", status, raw)
	}
	var artifact tea.Artifact
	decodeInto(t, raw, &artifact)

	content := []byte(`{"bomFormat":"CycloneDX"}`)
	status, raw = uploadFile(t, srv, "/admin/v1/artifacts/"+artifact.UUID+"/1/files", "sbom.json", content, "application/vnd.cyclonedx+json")
	if status != http.StatusOK {
		t.Fatalf("upload content: status=%d body=%s", status, raw)
	}

	signature := []byte("fake-detached-signature-bytes")
	status, raw = uploadFile(t, srv, "/admin/v1/artifacts/"+artifact.UUID+"/1/signature", "sbom.sig", signature, "application/octet-stream")
	if status != http.StatusOK {
		t.Fatalf("upload signature: status=%d body=%s", status, raw)
	}
	var updated tea.Artifact
	decodeInto(t, raw, &updated)
	// Uploading a signature must not touch the format's content URL/checksums.
	if updated.Formats[0].URL != "" {
		t.Fatalf("Formats[0].URL = %q after signature upload, want still empty", updated.Formats[0].URL)
	}
	if len(updated.Formats[0].Checksums) != 1 {
		t.Fatalf("Formats[0].Checksums = %+v after signature upload, want unchanged", updated.Formats[0].Checksums)
	}

	q := "?mediaType=" + url.QueryEscape("application/vnd.cyclonedx+json")

	sigResp, err := http.Get(srv.URL + "/tea/v1/artifact/" + artifact.UUID + "/latest/signature/download" + q)
	if err != nil {
		t.Fatalf("GET signature download: %v", err)
	}
	sigBody, _ := io.ReadAll(sigResp.Body)
	_ = sigResp.Body.Close()
	if sigResp.StatusCode != http.StatusOK {
		t.Fatalf("GET signature download: status=%d body=%s", sigResp.StatusCode, sigBody)
	}
	if string(sigBody) != string(signature) {
		t.Fatalf("signature body = %q, want %q", sigBody, signature)
	}
	sigSum := sha256.Sum256(signature)
	wantSigReprDigest := "sha-256=:" + base64.StdEncoding.EncodeToString(sigSum[:]) + ":"
	if rd := sigResp.Header.Get("Repr-Digest"); rd != wantSigReprDigest {
		t.Fatalf("signature Repr-Digest = %q, want %q", rd, wantSigReprDigest)
	}

	// Content is still independently retrievable, unchanged.
	contentResp, err := http.Get(srv.URL + "/tea/v1/artifact/" + artifact.UUID + "/latest/download" + q)
	if err != nil {
		t.Fatalf("GET content download: %v", err)
	}
	contentBody, _ := io.ReadAll(contentResp.Body)
	_ = contentResp.Body.Close()
	if contentResp.StatusCode != http.StatusOK || string(contentBody) != string(content) {
		t.Fatalf("content download: status=%d body=%q", contentResp.StatusCode, contentBody)
	}
}

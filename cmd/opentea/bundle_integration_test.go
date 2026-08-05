package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/oej/opentea/internal/authn"
	"github.com/oej/opentea/pkg/tea"
)

func uploadBundle(t *testing.T, srv *testServer, path string, zipBytes []byte) (int, []byte) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreatePart(map[string][]string{
		"Content-Disposition": {`form-data; name="bundle"; filename="product.zip"`},
		"Content-Type":        {"application/zip"},
	})
	if err != nil {
		t.Fatalf("create multipart part: %v", err)
	}
	if _, err := part.Write(zipBytes); err != nil {
		t.Fatalf("write multipart content: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, srv.URL+path, &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: authn.SessionCookieName, Value: srv.sessionToken})
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

// TestBundleExportImportRoundTrip builds a full product (release, component,
// component release + distribution file, artifact + file, collection, CLE
// event) on one server via /admin/v1, exports it, imports the bundle into a
// second, completely independent server, and verifies the data is readable
// there via the public /tea/v1 read API -- exercising the real HTTP export
// and import endpoints end to end, not just the internal/bundle package
// directly.
func TestBundleExportImportRoundTrip(t *testing.T) {
	src := newTestServer(t)

	status, raw := jsonRequest(t, src, http.MethodPost, "/admin/v1/products", map[string]any{
		"name":        "Acme Widget",
		"identifiers": []tea.Identifier{{IDType: "PURL", IDValue: "pkg:generic/acme-widget"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("create product: status=%d body=%s", status, raw)
	}
	var product tea.Product
	decodeInto(t, raw, &product)

	status, raw = jsonRequest(t, src, http.MethodPost, "/admin/v1/products/"+product.UUID+"/releases", map[string]any{
		"version":     "1.0.0",
		"createdDate": "2026-07-01T00:00:00Z",
	})
	if status != http.StatusCreated {
		t.Fatalf("create product release: status=%d body=%s", status, raw)
	}
	var productRelease tea.ProductRelease
	decodeInto(t, raw, &productRelease)

	status, raw = jsonRequest(t, src, http.MethodPost, "/admin/v1/components", map[string]any{"name": "acme-widget-core"})
	if status != http.StatusCreated {
		t.Fatalf("create component: status=%d body=%s", status, raw)
	}
	var component tea.Component
	decodeInto(t, raw, &component)

	status, raw = jsonRequest(t, src, http.MethodPost, "/admin/v1/components/"+component.UUID+"/releases", map[string]any{
		"version":     "1.0.0",
		"createdDate": "2026-07-01T00:00:00Z",
	})
	if status != http.StatusCreated {
		t.Fatalf("create component release: status=%d body=%s", status, raw)
	}
	var componentRelease tea.ComponentRelease
	decodeInto(t, raw, &componentRelease)

	status, raw = jsonRequest(t, src, http.MethodPost, "/admin/v1/componentReleases/"+componentRelease.UUID+"/distributions", map[string]any{
		"description": "linux/amd64 tarball",
	})
	if status != http.StatusCreated {
		t.Fatalf("create distribution: status=%d body=%s", status, raw)
	}
	var dist tea.ReleaseDistribution
	decodeInto(t, raw, &dist)

	status, raw = uploadFile(t, src, "/admin/v1/distributions/"+dist.DistributionID+"/files", "widget.tar.gz", []byte("fake tarball bytes"), "application/gzip")
	if status != http.StatusOK {
		t.Fatalf("upload distribution file: status=%d body=%s", status, raw)
	}

	status, raw = jsonRequest(t, src, http.MethodPost, "/admin/v1/artifacts", map[string]any{
		"type":    "BOM",
		"formats": []map[string]any{{"mediaType": "application/vnd.cyclonedx+json"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("create artifact: status=%d body=%s", status, raw)
	}
	var artifact tea.Artifact
	decodeInto(t, raw, &artifact)

	status, raw = uploadFile(t, src, "/admin/v1/artifacts/"+artifact.UUID+"/1/files", "sbom.json", []byte(`{"bomFormat":"CycloneDX"}`), "application/vnd.cyclonedx+json")
	if status != http.StatusOK {
		t.Fatalf("upload artifact format file: status=%d body=%s", status, raw)
	}

	componentReleaseUUID := componentRelease.UUID
	status, raw = jsonRequest(t, src, http.MethodPost, "/admin/v1/productReleases/"+productRelease.UUID+"/components", tea.ComponentRef{
		UUID: component.UUID, Release: &componentReleaseUUID,
	})
	if status != http.StatusOK {
		t.Fatalf("link component: status=%d body=%s", status, raw)
	}

	status, raw = jsonRequest(t, src, http.MethodPost, "/admin/v1/componentReleases/"+componentRelease.UUID+"/collections", map[string]any{
		"artifacts": []map[string]any{{"uuid": artifact.UUID, "version": artifact.Version}},
	})
	if status != http.StatusCreated {
		t.Fatalf("create collection: status=%d body=%s", status, raw)
	}

	status, raw = jsonRequest(t, src, http.MethodPost, "/admin/v1/products/"+product.UUID+"/cle/events", map[string]any{
		"type":      "released",
		"effective": "2026-07-01T00:00:00Z",
		"published": "2026-07-01T00:00:00Z",
	})
	if status != http.StatusCreated {
		t.Fatalf("create CLE event: status=%d body=%s", status, raw)
	}

	// Export from the source server.
	req, err := http.NewRequest(http.MethodGet, src.URL+"/admin/v1/products/"+product.UUID+"/export", nil)
	if err != nil {
		t.Fatalf("new export request: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: authn.SessionCookieName, Value: src.sessionToken})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET export: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET export: status=%d body=%s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("Content-Type = %q, want application/zip", ct)
	}
	zipBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read export body: %v", err)
	}
	if len(zipBytes) == 0 {
		t.Fatal("export produced an empty body")
	}

	// Import into a second, completely independent server.
	dst := newTestServer(t)
	status, raw = uploadBundle(t, dst, "/admin/v1/products/import", zipBytes)
	if status != http.StatusOK {
		t.Fatalf("import bundle: status=%d body=%s", status, raw)
	}
	var result struct {
		ProductCreated bool
		Created        map[string]int
	}
	decodeInto(t, raw, &result)
	if !result.ProductCreated {
		t.Fatalf("ProductCreated = false, result=%+v", result)
	}

	// Verify via the destination server's own public read API.
	resp2, err := http.Get(dst.URL + "/tea/v1/product/" + product.UUID)
	if err != nil {
		t.Fatalf("GET /tea/v1/product: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()
	body2, _ := io.ReadAll(resp2.Body)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("GET /tea/v1/product: status=%d body=%s", resp2.StatusCode, body2)
	}
	var readBackProduct tea.Product
	if err := json.Unmarshal(body2, &readBackProduct); err != nil {
		t.Fatalf("decode product: %v (body: %s)", err, body2)
	}
	if readBackProduct.Name != "Acme Widget" {
		t.Fatalf("Name = %q, want %q", readBackProduct.Name, "Acme Widget")
	}

	resp3, err := http.Get(dst.URL + "/tea/v1/componentRelease/" + componentRelease.UUID + "/collection/latest")
	if err != nil {
		t.Fatalf("GET .../collection/latest: %v", err)
	}
	defer func() { _ = resp3.Body.Close() }()
	body3, _ := io.ReadAll(resp3.Body)
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("GET /tea/v1/collection: status=%d body=%s", resp3.StatusCode, body3)
	}
	var readBackCollection tea.Collection
	if err := json.Unmarshal(body3, &readBackCollection); err != nil {
		t.Fatalf("decode collection: %v (body: %s)", err, body3)
	}
	if len(readBackCollection.Artifacts) != 1 {
		t.Fatalf("Artifacts = %+v, want 1", readBackCollection.Artifacts)
	}
	if len(readBackCollection.Artifacts[0].Formats) != 1 || readBackCollection.Artifacts[0].Formats[0].URL == "" {
		t.Fatalf("Artifacts[0].Formats = %+v, want one format with a rewritten URL", readBackCollection.Artifacts[0].Formats)
	}

	// Re-importing the identical bundle into the destination server must be
	// a complete no-op (idempotent), not an error or a duplicate.
	status, raw = uploadBundle(t, dst, "/admin/v1/products/import", zipBytes)
	if status != http.StatusOK {
		t.Fatalf("second import: status=%d body=%s", status, raw)
	}
	var result2 struct {
		ProductCreated bool
		Created        map[string]int
	}
	decodeInto(t, raw, &result2)
	if result2.ProductCreated {
		t.Fatalf("ProductCreated = true on second import, want false (idempotent), result=%+v", result2)
	}
}

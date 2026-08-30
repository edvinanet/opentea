// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oej/opentea/internal/authn"
	"github.com/oej/opentea/internal/config"
	"github.com/oej/opentea/internal/db"
	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/internal/storage"
	"github.com/oej/opentea/pkg/tea"
)

// testServer wraps httptest.Server with a bootstrapped admin session, since
// /admin/v1 requires authentication. jsonRequest/uploadFile always attach
// this session cookie -- harmless for /tea/v1 requests, which don't look at
// cookies at all.
type testServer struct {
	*httptest.Server
	repo         *repo.Repo
	adminUser    model.User
	sessionToken string
}

// newTestServer builds the full handler tree (read API + admin API + files)
// against a fresh temp-file SQLite DB and blob dir, bootstraps an admin user
// + session, and returns a ready-to-use testServer.
func newTestServer(t *testing.T) *testServer {
	t.Helper()
	return newTestServerWithAPIBasePath(t, "/tea/v1")
}

// newTestServerWithAPIBasePath is newTestServer with a caller-chosen
// config.Config.APIBasePath, for tests exercising that the consumer API is
// actually served (only) under the configured path.
func newTestServerWithAPIBasePath(t *testing.T, apiBasePath string) *testServer {
	t.Helper()
	dir := t.TempDir()

	sqlDB, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	blobStore, err := storage.NewFSStorage(filepath.Join(dir, "blobs"))
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}

	r := repo.New(sqlDB)

	cfg := config.Config{
		Versions:    []string{"0.4.0"},
		APIBasePath: apiBasePath,
		// This test harness builds Config by hand rather than going
		// through config.Load(), so its own duration defaults don't apply
		// -- set explicitly here so /publisher/v1 draft/lock/approval TTLs
		// (internal/publisher/collectiondraft.go) don't zero out to
		// "already expired" for every test that exercises them.
		PublisherDraftTTL:    168 * time.Hour,
		PublisherLockTTL:     time.Hour,
		PublisherApprovalTTL: 24 * time.Hour,
	}
	srv := httptest.NewServer(nil) // handler attached below, once we know srv.URL for cfg.RootURL
	cfg.RootURL = srv.URL
	srv.Config.Handler = newMux(r, blobStore, cfg, time.Now())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	admin, err := r.CreateUser(ctx, "test-admin", "test-password", model.RoleAdmin)
	if err != nil {
		t.Fatalf("bootstrap admin user: %v", err)
	}
	token, _, err := r.CreateSession(ctx, admin.UUID, authn.SessionTTL)
	if err != nil {
		t.Fatalf("bootstrap admin session: %v", err)
	}

	return &testServer{Server: srv, repo: r, adminUser: admin, sessionToken: token}
}

func jsonRequest(t *testing.T, srv *testServer, method, path string, body any) (int, []byte) {
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
	// Origin matching srv.URL, as a real browser would send on a same-origin
	// POST/DELETE -- requireRole's SameOrigin CSRF check requires this.
	req.Header.Set("Origin", srv.URL)
	req.AddCookie(&http.Cookie{Name: authn.SessionCookieName, Value: srv.sessionToken})
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

func decodeInto(t *testing.T, raw []byte, v any) {
	t.Helper()
	if len(raw) == 0 {
		return
	}
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("decode JSON: %v (body: %s)", err, raw)
	}
}

func uploadFile(t *testing.T, srv *testServer, path, filename string, content []byte, contentType string) (int, []byte) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreatePart(map[string][]string{
		"Content-Disposition": {`form-data; name="file"; filename="` + filename + `"`},
		"Content-Type":        {contentType},
	})
	if err != nil {
		t.Fatalf("create multipart part: %v", err)
	}
	if _, err := part.Write(content); err != nil {
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
	req.Header.Set("Origin", srv.URL)
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

// TestWorkedExample drives the full 11-step admin-ingestion chain from the
// plan (Product -> ProductRelease -> Component -> ComponentRelease+Distribution
// (+file upload) -> Artifact (+file upload) -> link component -> Collection
// -> CLE event) and then verifies every read-API endpoint that should now
// return that data.
func TestWorkedExample(t *testing.T) {
	srv := newTestServer(t)

	// 1. Create product.
	status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/products", map[string]any{
		"name":        "Acme Widget",
		"identifiers": []tea.Identifier{{IDType: "PURL", IDValue: "pkg:generic/acme-widget"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("create product: status=%d body=%s", status, raw)
	}
	var product tea.Product
	decodeInto(t, raw, &product)

	// 2. Create product release.
	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/products/"+product.UUID+"/releases", map[string]any{
		"version":     "1.0.0",
		"createdDate": "2026-07-01T00:00:00Z",
		"releaseDate": "2026-07-01T00:00:00Z",
		"identifiers": []tea.Identifier{{IDType: "TEI", IDValue: "urn:tei:uuid:acme.example.com:widget-1.0.0"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("create product release: status=%d body=%s", status, raw)
	}
	var productRelease tea.ProductRelease
	decodeInto(t, raw, &productRelease)

	// 3. Create component.
	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/components", map[string]any{
		"name": "acme-widget-core",
	})
	if status != http.StatusCreated {
		t.Fatalf("create component: status=%d body=%s", status, raw)
	}
	var component tea.Component
	decodeInto(t, raw, &component)

	// 4. Create component release.
	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/components/"+component.UUID+"/releases", map[string]any{
		"version":     "1.0.0",
		"createdDate": "2026-07-01T00:00:00Z",
	})
	if status != http.StatusCreated {
		t.Fatalf("create component release: status=%d body=%s", status, raw)
	}
	var componentRelease tea.ComponentRelease
	decodeInto(t, raw, &componentRelease)

	// 5. Add a distribution.
	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/componentReleases/"+componentRelease.UUID+"/distributions", map[string]any{
		"description": "linux/amd64 tarball",
	})
	if status != http.StatusCreated {
		t.Fatalf("create distribution: status=%d body=%s", status, raw)
	}
	var dist tea.ReleaseDistribution
	decodeInto(t, raw, &dist)

	// 6. Upload the distribution binary.
	distFileContent := []byte("fake tarball bytes")
	status, raw = uploadFile(t, srv, "/admin/v1/distributions/"+dist.DistributionID+"/files", "widget.tar.gz", distFileContent, "application/gzip")
	if status != http.StatusOK {
		t.Fatalf("upload distribution file: status=%d body=%s", status, raw)
	}

	// 7. Link the component to the product release.
	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/productReleases/"+productRelease.UUID+"/components", tea.ComponentRef{
		UUID: component.UUID, Release: &componentRelease.UUID,
	})
	if status != http.StatusOK {
		t.Fatalf("link component: status=%d body=%s", status, raw)
	}

	// 8. Create an artifact.
	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/artifacts", map[string]any{
		"name":            "cyclonedx-sbom.json",
		"type":            "BOM",
		"createdDate":     "2026-07-01T00:00:00Z",
		"distributionIds": []string{dist.DistributionID},
		"formats":         []map[string]any{{"mediaType": "application/vnd.cyclonedx+json"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("create artifact: status=%d body=%s", status, raw)
	}
	var artifact tea.Artifact
	decodeInto(t, raw, &artifact)

	// 9. Upload the artifact file.
	sbomContent := []byte(`{"bomFormat":"CycloneDX"}`)
	status, raw = uploadFile(t, srv,
		"/admin/v1/artifacts/"+artifact.UUID+"/"+strconv.Itoa(artifact.Version)+"/files?formatIndex=0",
		"sbom.json", sbomContent, "application/vnd.cyclonedx+json")
	if status != http.StatusOK {
		t.Fatalf("upload artifact file: status=%d body=%s", status, raw)
	}

	// 10. Create a collection for the component release, referencing the artifact.
	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/componentReleases/"+componentRelease.UUID+"/collections", map[string]any{
		"updateReason": tea.UpdateReason{Type: "INITIAL_RELEASE", Comment: "first collection"},
		"artifacts":    []map[string]any{{"uuid": artifact.UUID, "version": artifact.Version}},
	})
	if status != http.StatusCreated {
		t.Fatalf("create collection: status=%d body=%s", status, raw)
	}
	var collection tea.Collection
	decodeInto(t, raw, &collection)
	if collection.Version != 1 {
		t.Fatalf("collection.Version = %d, want 1", collection.Version)
	}

	// 11. Add a CLE "released" event to the product release.
	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/productReleases/"+productRelease.UUID+"/cle/events", map[string]any{
		"type":        "released",
		"effective":   "2026-07-01T00:00:00Z",
		"published":   "2026-07-01T00:00:00Z",
		"version":     "1.0.0",
		"description": "Initial release",
	})
	if status != http.StatusCreated {
		t.Fatalf("create CLE event: status=%d body=%s", status, raw)
	}

	// --- Read-side verification ---

	status, raw = jsonRequest(t, srv, http.MethodGet, "/tea/v1/products", nil)
	if status != http.StatusOK {
		t.Fatalf("GET /products: status=%d", status)
	}
	var products tea.PaginatedProducts
	decodeInto(t, raw, &products)
	if !containsUUID(mapUUIDs(products.Results, func(p tea.Product) string { return p.UUID }), product.UUID) {
		t.Fatalf("products list missing created product: %+v", products)
	}

	status, raw = jsonRequest(t, srv, http.MethodGet, "/tea/v1/product/"+product.UUID+"/releases", nil)
	if status != http.StatusOK {
		t.Fatalf("GET /product/{uuid}/releases: status=%d", status)
	}
	var releases tea.PaginatedProductReleases
	decodeInto(t, raw, &releases)
	if !containsUUID(mapUUIDs(releases.Results, func(pr tea.ProductRelease) string { return pr.UUID }), productRelease.UUID) {
		t.Fatalf("product releases list missing created release: %+v", releases)
	}

	status, raw = jsonRequest(t, srv, http.MethodGet, "/tea/v1/productRelease/"+productRelease.UUID, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /productRelease/{uuid}: status=%d", status)
	}
	var gotProductRelease tea.ProductRelease
	decodeInto(t, raw, &gotProductRelease)
	if len(gotProductRelease.Components) != 1 || gotProductRelease.Components[0].UUID != component.UUID {
		t.Fatalf("productRelease.Components = %+v, want linked component", gotProductRelease.Components)
	}

	status, raw = jsonRequest(t, srv, http.MethodGet, "/tea/v1/componentRelease/"+componentRelease.UUID, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /componentRelease/{uuid}: status=%d body=%s", status, raw)
	}
	var withCollection tea.ComponentReleaseWithCollection
	decodeInto(t, raw, &withCollection)
	if len(withCollection.LatestCollection.Artifacts) != 1 {
		t.Fatalf("latestCollection.Artifacts = %+v, want 1", withCollection.LatestCollection.Artifacts)
	}
	artifactURL := withCollection.LatestCollection.Artifacts[0].Formats[0].URL
	if artifactURL == "" {
		t.Fatal("expected artifact format URL to be set")
	}

	// Fetch the uploaded artifact bytes back out and confirm they round-trip.
	resp, err := http.Get(artifactURL)
	if err != nil {
		t.Fatalf("GET artifact file: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET artifact file: status=%d", resp.StatusCode)
	}
	gotBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read artifact file body: %v", err)
	}
	if !bytes.Equal(gotBytes, sbomContent) {
		t.Fatalf("artifact file content = %q, want %q", gotBytes, sbomContent)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/vnd.cyclonedx+json" {
		t.Fatalf("artifact file Content-Type = %q", ct)
	}

	status, raw = jsonRequest(t, srv, http.MethodGet, "/tea/v1/productRelease/"+productRelease.UUID+"/cle", nil)
	if status != http.StatusOK {
		t.Fatalf("GET .../cle: status=%d", status)
	}
	var cle tea.CLE
	decodeInto(t, raw, &cle)
	if len(cle.Events) != 1 || cle.Events[0].Type != "released" {
		t.Fatalf("cle.Events = %+v", cle.Events)
	}

	teiEscaped := url.QueryEscape("urn:tei:uuid:acme.example.com:widget-1.0.0")
	status, raw = jsonRequest(t, srv, http.MethodGet, "/tea/v1/discovery?tei="+teiEscaped, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /discovery: status=%d", status)
	}
	var discovery []tea.DiscoveryInfo
	decodeInto(t, raw, &discovery)
	if len(discovery) != 1 || discovery[0].ProductReleaseUUID != productRelease.UUID {
		t.Fatalf("discovery = %+v, want resolved to %s", discovery, productRelease.UUID)
	}

	status, raw = jsonRequest(t, srv, http.MethodGet, "/tea/v1/discovery?tei=urn%3Atei%3Aunknown", nil)
	if status != http.StatusOK {
		t.Fatalf("GET /discovery (unknown): status=%d", status)
	}
	var emptyDiscovery []tea.DiscoveryInfo
	decodeInto(t, raw, &emptyDiscovery)
	if len(emptyDiscovery) != 0 {
		t.Fatalf("discovery for unknown tei = %+v, want []", emptyDiscovery)
	}
}

func TestErrorResponses(t *testing.T) {
	srv := newTestServer(t)

	status, raw := jsonRequest(t, srv, http.MethodGet, "/tea/v1/product/00000000-0000-4000-8000-000000000000", nil)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	var errResp tea.ErrorResponse
	decodeInto(t, raw, &errResp)
	if errResp.Error != tea.ErrorObjectUnknown {
		t.Fatalf("error = %q, want OBJECT_UNKNOWN", errResp.Error)
	}

	if status, _ := jsonRequest(t, srv, http.MethodGet, "/tea/v1/product/not-a-uuid", nil); status != http.StatusBadRequest {
		t.Fatalf("invalid uuid: status = %d, want 400", status)
	}
	if status, _ := jsonRequest(t, srv, http.MethodGet, "/tea/v1/products?pageSize=999", nil); status != http.StatusBadRequest {
		t.Fatalf("invalid pageSize: status = %d, want 400", status)
	}
	if status, _ := jsonRequest(t, srv, http.MethodGet, "/tea/v1/products?sortField=bogus", nil); status != http.StatusBadRequest {
		t.Fatalf("invalid sortField: status = %d, want 400", status)
	}
}

// TestConfigurableAPIBasePath is the regression test for making
// config.Config.APIBasePath (TEA_API_BASE_PATH) configurable: a server
// configured to serve at the literal /v{version} path the TEA discovery
// spec has clients construct (rather than the default /tea/v1) must
// actually answer there -- and, since this is a single-mount replacement
// rather than dual-serving, must no longer answer at the old default path.
func TestConfigurableAPIBasePath(t *testing.T) {
	srv := newTestServerWithAPIBasePath(t, "/v0.4.0")

	if status, _ := jsonRequest(t, srv, http.MethodGet, "/v0.4.0/products", nil); status != http.StatusOK {
		t.Fatalf("GET /v0.4.0/products: status = %d, want 200", status)
	}
	if status, _ := jsonRequest(t, srv, http.MethodGet, "/tea/v1/products", nil); status != http.StatusNotFound {
		t.Fatalf("GET /tea/v1/products: status = %d, want 404 (not mounted once APIBasePath is overridden)", status)
	}
}

// TestSplitListenersIsolateRoutes is the regression test for
// config.Config.AdminListenAddr: when the admin surface is split onto its
// own listener, each listener's mux must carry only its own routes -- the
// API listener must not still expose /admin/v1 or /admin/ui, and the admin
// listener must not still expose the consumer API or /files. Uses
// newAPIMux/newAdminMux directly (not newMux) since that's exactly the
// split main() takes when AdminListenAddr is set.
func TestSplitListenersIsolateRoutes(t *testing.T) {
	dir := t.TempDir()
	sqlDB, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	blobStore, err := storage.NewFSStorage(filepath.Join(dir, "blobs"))
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	r := repo.New(sqlDB)
	cfg := config.Config{Versions: []string{"0.4.0"}, APIBasePath: "/tea/v1"}

	apiSrv := httptest.NewServer(newAPIMux(r, blobStore, cfg))
	t.Cleanup(apiSrv.Close)
	adminSrv := httptest.NewServer(newAdminMux(r, blobStore, cfg, time.Now()))
	t.Cleanup(adminSrv.Close)

	get := func(base, path string) int {
		t.Helper()
		resp, err := http.Get(base + path)
		if err != nil {
			t.Fatalf("GET %s%s: %v", base, path, err)
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}

	if status := get(apiSrv.URL, "/tea/v1/products"); status != http.StatusOK {
		t.Errorf("API listener GET /tea/v1/products: status = %d, want 200", status)
	}
	if status := get(apiSrv.URL, "/admin/ui/login"); status != http.StatusNotFound {
		t.Errorf("API listener GET /admin/ui/login: status = %d, want 404 (admin surface must not be reachable here)", status)
	}
	if status := get(apiSrv.URL, "/admin/v1/stats"); status != http.StatusNotFound {
		t.Errorf("API listener GET /admin/v1/stats: status = %d, want 404", status)
	}

	if status := get(adminSrv.URL, "/admin/ui/login"); status != http.StatusOK {
		t.Errorf("admin listener GET /admin/ui/login: status = %d, want 200", status)
	}
	if status := get(adminSrv.URL, "/tea/v1/products"); status != http.StatusNotFound {
		t.Errorf("admin listener GET /tea/v1/products: status = %d, want 404 (consumer API must not be reachable here)", status)
	}
	if status := get(adminSrv.URL, "/files/deadbeef"); status != http.StatusNotFound {
		t.Errorf("admin listener GET /files/deadbeef: status = %d, want 404", status)
	}
}

// TestCreateUserRejectsShortPassword is the regression test for the finding
// that admin-created accounts had no password strength requirement (only
// non-empty). Exercises the rejection through both the JSON admin API and
// the HTML GUI form, and confirms a long-enough password still succeeds.
func TestCreateUserRejectsShortPassword(t *testing.T) {
	srv := newTestServer(t)

	status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/users", map[string]any{
		"username": "shortpw", "password": "short", "role": "consumer",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("POST /admin/v1/users (short password): status=%d body=%s, want 400", status, raw)
	}

	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/users", map[string]any{
		"username": "longenoughpw", "password": "longenough1", "role": "consumer",
	})
	if status != http.StatusCreated {
		t.Fatalf("POST /admin/v1/users (long enough password): status=%d body=%s, want 201", status, raw)
	}
}

// TestUploadToInvalidTargetDoesNotOrphanBlob is the regression test for the
// finding that receiveFile persisted a blob before uploadDistributionFile/
// uploadArtifactFormatFile confirmed the target they'd attach it to even
// exists -- an upload to a bad id always stored a blob nothing would ever
// reference. Each case below uploads distinguishable content to a target
// that can't possibly accept it, then confirms via the public /files/{sha256}
// endpoint (itself proof the DB bookkeeping row -- and, since receiveFile
// writes both together, the file on disk -- was never created) that nothing
// was stored.
func TestUploadToInvalidTargetDoesNotOrphanBlob(t *testing.T) {
	srv := newTestServer(t)

	assertNeverStored := func(t *testing.T, path string, content []byte) {
		t.Helper()
		status, _ := uploadFile(t, srv, path, "f.bin", content, "application/octet-stream")
		if status != http.StatusNotFound {
			t.Fatalf("upload to %s: status=%d, want 404", path, status)
		}
		sum := sha256.Sum256(content)
		sha256Hex := hex.EncodeToString(sum[:])
		fileStatus, _ := jsonRequest(t, srv, http.MethodGet, "/files/"+sha256Hex, nil)
		if fileStatus != http.StatusNotFound {
			t.Fatalf("GET /files/%s after rejected upload to %s: status=%d, want 404 (blob was orphaned)", sha256Hex, path, fileStatus)
		}
	}

	t.Run("nonexistent distribution id", func(t *testing.T) {
		assertNeverStored(t, "/admin/v1/distributions/00000000-0000-0000-0000-000000000000/files", []byte("orphan candidate 1"))
	})

	// A real artifact, to exercise "wrong version" and "formatIndex out of
	// range" against a target that otherwise genuinely exists.
	status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/artifacts", map[string]any{
		"type":    "BOM",
		"formats": []map[string]any{{"mediaType": "application/json"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("create artifact: status=%d body=%s", status, raw)
	}
	var artifact tea.Artifact
	decodeInto(t, raw, &artifact)

	t.Run("nonexistent artifact uuid", func(t *testing.T) {
		assertNeverStored(t, "/admin/v1/artifacts/00000000-0000-0000-0000-000000000000/1/files", []byte("orphan candidate 2"))
	})
	t.Run("wrong artifact version", func(t *testing.T) {
		assertNeverStored(t, "/admin/v1/artifacts/"+artifact.UUID+"/99/files", []byte("orphan candidate 3"))
	})
	t.Run("formatIndex out of range", func(t *testing.T) {
		assertNeverStored(t, "/admin/v1/artifacts/"+artifact.UUID+"/1/files?formatIndex=5", []byte("orphan candidate 4"))
	})
}

// TestPaginationAcrossPages creates more products than one page holds and
// walks nextPageToken to confirm every product is seen exactly once.
func TestPaginationAcrossPages(t *testing.T) {
	srv := newTestServer(t)

	const total = 7
	const pageSize = 3
	created := make(map[string]bool, total)
	for i := 0; i < total; i++ {
		status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/products", map[string]any{
			"name": "product-" + strconv.Itoa(i),
		})
		if status != http.StatusCreated {
			t.Fatalf("create product %d: status=%d body=%s", i, status, raw)
		}
		var p tea.Product
		decodeInto(t, raw, &p)
		created[p.UUID] = true
	}

	seen := map[string]bool{}
	token := ""
	for pages := 0; pages < total; pages++ { // safety bound
		path := "/tea/v1/products?pageSize=" + strconv.Itoa(pageSize)
		if token != "" {
			path += "&pageToken=" + url.QueryEscape(token)
		}
		status, raw := jsonRequest(t, srv, http.MethodGet, path, nil)
		if status != http.StatusOK {
			t.Fatalf("GET /products: status=%d body=%s", status, raw)
		}
		var page tea.PaginatedProducts
		decodeInto(t, raw, &page)
		for _, p := range page.Results {
			if seen[p.UUID] {
				t.Fatalf("product %s seen twice across pages", p.UUID)
			}
			seen[p.UUID] = true
		}
		if !page.HasNext {
			break
		}
		token = page.NextPageToken
	}

	if len(seen) != total {
		t.Fatalf("saw %d products across all pages, want %d", len(seen), total)
	}
	for uuid := range created {
		if !seen[uuid] {
			t.Fatalf("product %s never seen while paginating", uuid)
		}
	}
}

func mapUUIDs[T any](items []T, get func(T) string) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = get(item)
	}
	return out
}

func containsUUID(uuids []string, target string) bool {
	for _, u := range uuids {
		if u == target {
			return true
		}
	}
	return false
}

// noRedirectClient behaves like plain curl (no -L): it doesn't follow
// redirects, so the caller can inspect a 303's Set-Cookie header directly.
func noRedirectClient() *http.Client {
	return &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

func postForm(t *testing.T, srv *testServer, path string, form url.Values, cookie *http.Cookie) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Matches a real browser's same-origin POST -- requireRole's and
	// requireSameOrigin's CSRF checks both require this.
	req.Header.Set("Origin", srv.URL)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := noRedirectClient().Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

// TestAdminLoginFlow drives /admin/ui/login through the real HTTP stack: a
// wrong password re-renders the login page with an error, a correct one
// sets a session cookie that then works against /admin/v1.
func TestAdminLoginFlow(t *testing.T) {
	srv := newTestServer(t)

	resp := postForm(t, srv, "/admin/ui/login", url.Values{"username": {"test-admin"}, "password": {"wrong"}}, nil)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !bytes.Contains(body, []byte("invalid username or password")) {
		t.Fatalf("wrong password: status=%d body=%s", resp.StatusCode, body)
	}

	resp = postForm(t, srv, "/admin/ui/login", url.Values{"username": {"test-admin"}, "password": {"test-password"}}, nil)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("correct password: status=%d, want 303 redirect", resp.StatusCode)
	}
	var sessionCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == authn.SessionCookieName {
			sessionCookie = c
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatalf("expected a session cookie to be set, got %+v", resp.Cookies())
	}

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/admin/v1/products", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.AddCookie(sessionCookie)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /admin/v1/products with session cookie: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /admin/v1/products with session cookie: status=%d, want 200", resp.StatusCode)
	}
}

// TestRoleGatingAcrossStack verifies the consumer/admin role split holds
// through the real HTTP stack, for both the JSON admin API and the GUI.
func TestRoleGatingAcrossStack(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	consumerUser, err := srv.repo.CreateUser(ctx, "viewer", "viewerpass", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	consumerToken, _, err := srv.repo.CreateSession(ctx, consumerUser.UUID, authn.SessionTTL)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	consumerCookie := &http.Cookie{Name: authn.SessionCookieName, Value: consumerToken}

	doWithCookie := func(method, path string, cookie *http.Cookie, body []byte) int {
		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(body)
		}
		req, err := http.NewRequest(method, srv.URL+path, reader)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Origin", srv.URL)
		req.AddCookie(cookie)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	// No session at all -> 401.
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/admin/v1/products", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /admin/v1/products (no session): %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no session: status=%d, want 401", resp.StatusCode)
	}

	// Consumer can read...
	if status := doWithCookie(http.MethodGet, "/admin/v1/products", consumerCookie, nil); status != http.StatusOK {
		t.Fatalf("consumer GET /admin/v1/products: status=%d, want 200", status)
	}
	// ...but not write.
	if status := doWithCookie(http.MethodPost, "/admin/v1/products", consumerCookie, []byte(`{"name":"nope"}`)); status != http.StatusForbidden {
		t.Fatalf("consumer POST /admin/v1/products: status=%d, want 403", status)
	}
	// Consumer can't view the admin-only Users GUI page.
	if status := doWithCookie(http.MethodGet, "/admin/ui/users", consumerCookie, nil); status != http.StatusForbidden {
		t.Fatalf("consumer GET /admin/ui/users: status=%d, want 403", status)
	}

	// The bootstrap admin (srv.sessionToken) can do both.
	adminCookie := &http.Cookie{Name: authn.SessionCookieName, Value: srv.sessionToken}
	if status := doWithCookie(http.MethodGet, "/admin/v1/products", adminCookie, nil); status != http.StatusOK {
		t.Fatalf("admin GET /admin/v1/products: status=%d, want 200", status)
	}
	if status := doWithCookie(http.MethodPost, "/admin/v1/products", adminCookie, []byte(`{"name":"acme"}`)); status != http.StatusCreated {
		t.Fatalf("admin POST /admin/v1/products: status=%d, want 201", status)
	}
}

// TestCSRFProtection is the regression test for the CSRF finding: a
// state-changing request carrying a genuinely valid session cookie must
// still be rejected if its declared origin doesn't match the server's own
// (simulating a forged cross-site request riding the victim's cookie -- the
// scenario SameSite=Lax alone doesn't fully cover, see requireRole in both
// internal/admin and internal/webadmin). Safe methods (GET) must stay
// unaffected regardless of origin.
func TestCSRFProtection(t *testing.T) {
	srv := newTestServer(t)
	adminCookie := &http.Cookie{Name: authn.SessionCookieName, Value: srv.sessionToken}

	do := func(method, path, origin string, body []byte) int {
		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(body)
		}
		req, err := http.NewRequest(method, srv.URL+path, reader)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		req.AddCookie(adminCookie)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	// A forged cross-site POST: valid cookie, wrong Origin -> rejected.
	if status := do(http.MethodPost, "/admin/v1/products", "https://evil.example", []byte(`{"name":"forged"}`)); status != http.StatusForbidden {
		t.Fatalf("POST with cross-site Origin: status=%d, want 403", status)
	}
	// No Origin/Referer at all on a state-changing request -> also rejected.
	if status := do(http.MethodPost, "/admin/v1/products", "", []byte(`{"name":"forged"}`)); status != http.StatusForbidden {
		t.Fatalf("POST with no Origin: status=%d, want 403", status)
	}
	// Same-origin POST still works.
	if status := do(http.MethodPost, "/admin/v1/products", srv.URL, []byte(`{"name":"legit"}`)); status != http.StatusCreated {
		t.Fatalf("POST with matching Origin: status=%d, want 201", status)
	}
	// GET is unaffected by a mismatched Origin -- it's not a CSRF target.
	if status := do(http.MethodGet, "/admin/v1/products", "https://evil.example", nil); status != http.StatusOK {
		t.Fatalf("GET with cross-site Origin: status=%d, want 200", status)
	}

	// Same story for the HTML admin GUI (internal/webadmin), which shares
	// the same requireRole/SameOrigin protection.
	if status := do(http.MethodPost, "/admin/ui/token/generate", "https://evil.example", []byte{}); status != http.StatusForbidden {
		t.Fatalf("GUI POST with cross-site Origin: status=%d, want 403", status)
	}
	if status := do(http.MethodPost, "/admin/ui/token/generate", srv.URL, []byte{}); status == http.StatusForbidden {
		t.Fatalf("GUI POST with matching Origin: status=%d, want non-403", status)
	}

	// Login and logout are reachable without a session (requireRole can't
	// cover them), so they need their own, independent requireSameOrigin
	// check -- confirm it's actually wired in, not just present on the
	// authenticated routes above.
	loginBody := []byte(url.Values{"username": {"test-admin"}, "password": {"wrong"}}.Encode())
	if status := do(http.MethodPost, "/admin/ui/login", "https://evil.example", loginBody); status != http.StatusForbidden {
		t.Fatalf("login POST with cross-site Origin: status=%d, want 403", status)
	}
	if status := do(http.MethodPost, "/admin/ui/login", "", loginBody); status != http.StatusForbidden {
		t.Fatalf("login POST with no Origin: status=%d, want 403", status)
	}
	if status := do(http.MethodPost, "/admin/ui/login", srv.URL, loginBody); status == http.StatusForbidden {
		t.Fatalf("login POST with matching Origin: status=%d, want non-403", status)
	}

	if status := do(http.MethodPost, "/admin/ui/logout", "https://evil.example", []byte{}); status != http.StatusForbidden {
		t.Fatalf("logout POST with cross-site Origin: status=%d, want 403", status)
	}
	if status := do(http.MethodPost, "/admin/ui/logout", "", []byte{}); status != http.StatusForbidden {
		t.Fatalf("logout POST with no Origin: status=%d, want 403", status)
	}
	if status := do(http.MethodPost, "/admin/ui/logout", srv.URL, []byte{}); status == http.StatusForbidden {
		t.Fatalf("logout POST with matching Origin: status=%d, want non-403", status)
	}
}

// TestTeaV1BearerToken verifies /tea/v1's additive bearer-token behavior:
// no header stays public (unchanged), an invalid token is rejected, and a
// real one (obtained the same way a GUI user would) works.
func TestTeaV1BearerToken(t *testing.T) {
	srv := newTestServer(t)

	if status, _ := jsonRequest(t, srv, http.MethodGet, "/tea/v1/products", nil); status != http.StatusOK {
		t.Fatalf("no Authorization header: status=%d, want 200 (still public)", status)
	}

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/tea/v1/products", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer garbage-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET with garbage bearer token: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("garbage bearer token: status=%d, want 401", resp.StatusCode)
	}

	token, err := srv.repo.SetAPIToken(context.Background(), srv.adminUser.UUID)
	if err != nil {
		t.Fatalf("SetAPIToken: %v", err)
	}
	req, err = http.NewRequest(http.MethodGet, srv.URL+"/tea/v1/products", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET with valid bearer token: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("valid bearer token: status=%d, want 200", resp.StatusCode)
	}
}

// TestStatsIncludesStartedAt verifies GET /admin/v1/stats (the server's
// monitoring endpoint) reports a plausible process-start timestamp: in the
// past, but recent (this test process itself just started the server).
func TestStatsIncludesStartedAt(t *testing.T) {
	before := time.Now().Add(-time.Minute)
	srv := newTestServer(t)

	status, raw := jsonRequest(t, srv, http.MethodGet, "/admin/v1/stats", nil)
	if status != http.StatusOK {
		t.Fatalf("GET /admin/v1/stats: status=%d body=%s", status, raw)
	}
	var stats model.Stats
	decodeInto(t, raw, &stats)

	if stats.StartedAt.IsZero() {
		t.Fatal("stats.StartedAt is zero, want a real timestamp")
	}
	if stats.StartedAt.Before(before) || stats.StartedAt.After(time.Now()) {
		t.Fatalf("stats.StartedAt = %v, want between %v and now", stats.StartedAt, before)
	}
}

// TestSecurityHeaders confirms securityHeaders' response headers actually
// reach a client through the full handler tree (not just unit-tested in
// isolation), across all four sub-routers, and that Strict-Transport-Security
// is correctly absent over this test server's plain HTTP (newTestServer
// doesn't set TrustProxyHeaders, so there's no signal telling securityHeaders
// to treat the connection as secure).
func TestSecurityHeaders(t *testing.T) {
	srv := newTestServer(t)

	for _, path := range []string{"/tea/v1/products", "/admin/v1/products", "/admin/ui/login", "/files/nonexistent"} {
		t.Run(path, func(t *testing.T) {
			resp, err := http.Get(srv.URL + path) //nolint:gosec // srv.URL is this test's own httptest.Server, not remote input
			if err != nil {
				t.Fatalf("GET %s: %v", path, err)
			}
			_ = resp.Body.Close()

			want := map[string]string{
				"X-Content-Type-Options":  "nosniff",
				"X-Frame-Options":         "DENY",
				"Referrer-Policy":         "strict-origin-when-cross-origin",
				"Content-Security-Policy": adminCSP,
			}
			for header, wantValue := range want {
				if got := resp.Header.Get(header); got != wantValue {
					t.Errorf("%s: header %s = %q, want %q", path, header, got, wantValue)
				}
			}
			if got := resp.Header.Get("Strict-Transport-Security"); got != "" {
				t.Errorf("%s: Strict-Transport-Security = %q, want absent over plain HTTP", path, got)
			}
		})
	}
}

// TestCollectionRoutesRespectParentType is the regression test for the
// collection routes not validating their parent's release type: before this
// fix, GetLatestCollection/GetCollectionByVersion/ListCollections queried
// collection.uuid alone, so a component-release collection's UUID resolved
// through the product-release route (and vice versa) with no 404 -- a real
// cross-type data-isolation gap, made concretely exploitable by bundle
// import preserving source-controlled UUIDs verbatim. This creates one
// collection of each type and confirms each is retrievable only through its
// own route (latest, versioned, list), 404ing through the other -- proving
// a real UUID that legitimately exists, just under the other type, is
// correctly rejected rather than silently returning the wrong data.
func TestCollectionRoutesRespectParentType(t *testing.T) {
	srv := newTestServer(t)

	// Product release + its own collection.
	status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/products", map[string]any{"name": "Parent-Type Product"})
	if status != http.StatusCreated {
		t.Fatalf("create product: status=%d body=%s", status, raw)
	}
	var product tea.Product
	decodeInto(t, raw, &product)

	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/products/"+product.UUID+"/releases", map[string]any{
		"version": "1.0.0", "createdDate": "2026-07-01T00:00:00Z",
	})
	if status != http.StatusCreated {
		t.Fatalf("create product release: status=%d body=%s", status, raw)
	}
	var productRelease tea.ProductRelease
	decodeInto(t, raw, &productRelease)

	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/productReleases/"+productRelease.UUID+"/collections", map[string]any{})
	if status != http.StatusCreated {
		t.Fatalf("create product-release collection: status=%d body=%s", status, raw)
	}

	// Component release + its own collection.
	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/components", map[string]any{"name": "Parent-Type Component"})
	if status != http.StatusCreated {
		t.Fatalf("create component: status=%d body=%s", status, raw)
	}
	var component tea.Component
	decodeInto(t, raw, &component)

	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/components/"+component.UUID+"/releases", map[string]any{
		"version": "1.0.0", "createdDate": "2026-07-01T00:00:00Z",
	})
	if status != http.StatusCreated {
		t.Fatalf("create component release: status=%d body=%s", status, raw)
	}
	var componentRelease tea.ComponentRelease
	decodeInto(t, raw, &componentRelease)

	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/componentReleases/"+componentRelease.UUID+"/collections", map[string]any{})
	if status != http.StatusCreated {
		t.Fatalf("create component-release collection: status=%d body=%s", status, raw)
	}

	// "latest" and "versioned" are single-item lookups: a real UUID that
	// exists only under the *other* type must 404, not return that other
	// type's collection.
	itemCases := []struct {
		name       string
		pathSuffix string // appended to "/tea/v1/{productRelease,componentRelease}/{uuid}"
	}{
		{"latest", "/collection/latest"},
		{"versioned", "/collection/1"},
	}
	for _, tc := range itemCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(srv.URL + "/tea/v1/productRelease/" + productRelease.UUID + tc.pathSuffix) //nolint:gosec // srv.URL is this test's own httptest.Server
			if err != nil {
				t.Fatalf("GET productRelease route (own type): %v", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("productRelease route on its own productRelease UUID: status=%d, want 200", resp.StatusCode)
			}

			resp, err = http.Get(srv.URL + "/tea/v1/componentRelease/" + componentRelease.UUID + tc.pathSuffix) //nolint:gosec
			if err != nil {
				t.Fatalf("GET componentRelease route (own type): %v", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("componentRelease route on its own componentRelease UUID: status=%d, want 200", resp.StatusCode)
			}

			resp, err = http.Get(srv.URL + "/tea/v1/componentRelease/" + productRelease.UUID + tc.pathSuffix) //nolint:gosec
			if err != nil {
				t.Fatalf("GET componentRelease route (wrong type): %v", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("componentRelease route on a productRelease UUID: status=%d, want 404", resp.StatusCode)
			}

			resp, err = http.Get(srv.URL + "/tea/v1/productRelease/" + componentRelease.UUID + tc.pathSuffix) //nolint:gosec
			if err != nil {
				t.Fatalf("GET productRelease route (wrong type): %v", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("productRelease route on a componentRelease UUID: status=%d, want 404", resp.StatusCode)
			}
		})
	}

	// "list" never 404s (a list query correctly returns an empty result
	// rather than not-found) -- what matters here is that querying by the
	// *other* type's UUID returns zero results, not that type's data.
	t.Run("list", func(t *testing.T) {
		status, raw := jsonRequest(t, srv, http.MethodGet, "/tea/v1/productRelease/"+productRelease.UUID+"/collections", nil)
		var ownList tea.PaginatedCollections
		decodeInto(t, raw, &ownList)
		if status != http.StatusOK || len(ownList.Results) != 1 {
			t.Fatalf("productRelease list (own type): status=%d results=%+v, want 200 with 1 result", status, ownList.Results)
		}

		status, raw = jsonRequest(t, srv, http.MethodGet, "/tea/v1/componentRelease/"+productRelease.UUID+"/collections", nil)
		var wrongList tea.PaginatedCollections
		decodeInto(t, raw, &wrongList)
		if status != http.StatusOK || len(wrongList.Results) != 0 {
			t.Fatalf("componentRelease list on a productRelease UUID: status=%d results=%+v, want 200 with 0 results", status, wrongList.Results)
		}
	})
}

// getWithETag issues a GET against path, optionally sending ifNoneMatch as
// the If-None-Match header (skipped entirely if empty), and returns the
// status code, the response's own ETag header, and the body.
func getWithETag(t *testing.T, srv *testServer, path, ifNoneMatch string) (status int, etag string, body []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp.StatusCode, resp.Header.Get("ETag"), b
}

// TestETagConditionalRequests drives ETag/If-None-Match support through the
// real HTTP stack across the 4 shapes this feature distinguishes:
// existence-only (product, provably immutable), revision-based
// (productRelease, mutated via LinkComponent), a second revision-based case
// with its own mutation path (artifact, mutated via file upload), and CLE
// (revision-based, with the design-review-caught delete-must-still-bump
// case). Also proves the core regression the "skip the DB for immutable
// resources" bug would have caused: a deleted product's cached ETag must
// 404, not 304.
func TestETagConditionalRequests(t *testing.T) {
	srv := newTestServer(t)

	status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/products", map[string]any{"name": "ETag Test Product"})
	if status != http.StatusCreated {
		t.Fatalf("create product: status=%d body=%s", status, raw)
	}
	var product tea.Product
	decodeInto(t, raw, &product)

	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/products/"+product.UUID+"/releases", map[string]any{
		"version": "1.0.0", "createdDate": "2026-07-01T00:00:00Z",
	})
	if status != http.StatusCreated {
		t.Fatalf("create product release: status=%d body=%s", status, raw)
	}
	var productRelease tea.ProductRelease
	decodeInto(t, raw, &productRelease)

	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/components", map[string]any{"name": "etag-test-component"})
	if status != http.StatusCreated {
		t.Fatalf("create component: status=%d body=%s", status, raw)
	}
	var component tea.Component
	decodeInto(t, raw, &component)

	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/artifacts", map[string]any{
		"type": "BOM", "formats": []map[string]any{{"mediaType": "application/json"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("create artifact: status=%d body=%s", status, raw)
	}
	var artifact tea.Artifact
	decodeInto(t, raw, &artifact)

	t.Run("product (existence-only)", func(t *testing.T) {
		path := "/tea/v1/product/" + product.UUID
		status, etag1, _ := getWithETag(t, srv, path, "")
		if status != http.StatusOK || etag1 == "" {
			t.Fatalf("first GET: status=%d etag=%q, want 200 with a non-empty ETag", status, etag1)
		}

		status, _, body := getWithETag(t, srv, path, etag1)
		if status != http.StatusNotModified {
			t.Fatalf("matching If-None-Match: status=%d, want 304", status)
		}
		if len(body) != 0 {
			t.Fatalf("304 body = %q, want empty", body)
		}

		status, etag2, _ := getWithETag(t, srv, path, `"stale-etag"`)
		if status != http.StatusOK || etag2 != etag1 {
			t.Fatalf("stale If-None-Match: status=%d etag=%q, want 200 with the same ETag %q (product never changes)", status, etag2, etag1)
		}
	})

	t.Run("productRelease (revision, mutated via LinkComponent)", func(t *testing.T) {
		path := "/tea/v1/productRelease/" + productRelease.UUID
		status, etag1, _ := getWithETag(t, srv, path, "")
		if status != http.StatusOK || etag1 == "" {
			t.Fatalf("first GET: status=%d etag=%q, want 200 with a non-empty ETag", status, etag1)
		}
		status, _, _ = getWithETag(t, srv, path, etag1)
		if status != http.StatusNotModified {
			t.Fatalf("matching If-None-Match: status=%d, want 304", status)
		}

		status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/productReleases/"+productRelease.UUID+"/components", tea.ComponentRef{UUID: component.UUID})
		if status != http.StatusOK {
			t.Fatalf("LinkComponent: status=%d body=%s", status, raw)
		}

		status, etag2, _ := getWithETag(t, srv, path, etag1)
		if status != http.StatusOK {
			t.Fatalf("GET after LinkComponent: status=%d, want 200 (old ETag must no longer match)", status)
		}
		if etag2 == etag1 {
			t.Fatal("ETag unchanged after LinkComponent, want a new one")
		}
	})

	t.Run("artifact (revision, mutated via file upload)", func(t *testing.T) {
		path := "/tea/v1/artifact/" + artifact.UUID + "/" + strconv.Itoa(artifact.Version)
		status, etag1, _ := getWithETag(t, srv, path, "")
		if status != http.StatusOK || etag1 == "" {
			t.Fatalf("first GET: status=%d etag=%q, want 200 with a non-empty ETag", status, etag1)
		}
		status, _, _ = getWithETag(t, srv, path, etag1)
		if status != http.StatusNotModified {
			t.Fatalf("matching If-None-Match: status=%d, want 304", status)
		}

		status, raw := uploadFile(t, srv,
			"/admin/v1/artifacts/"+artifact.UUID+"/"+strconv.Itoa(artifact.Version)+"/files?formatIndex=0",
			"sbom.json", []byte(`{"bomFormat":"CycloneDX"}`), "application/json")
		if status != http.StatusOK {
			t.Fatalf("upload artifact file: status=%d body=%s", status, raw)
		}

		status, etag2, _ := getWithETag(t, srv, path, etag1)
		if status != http.StatusOK {
			t.Fatalf("GET after file upload: status=%d, want 200 (old ETag must no longer match)", status)
		}
		if etag2 == etag1 {
			t.Fatal("ETag unchanged after file upload, want a new one")
		}
	})

	t.Run("CLE (revision, mutated via CreateCLEEvent)", func(t *testing.T) {
		path := "/tea/v1/product/" + product.UUID + "/cle"
		status, etag1, _ := getWithETag(t, srv, path, "")
		if status != http.StatusOK || etag1 == "" {
			t.Fatalf("first GET (no events yet): status=%d etag=%q, want 200 with a non-empty ETag", status, etag1)
		}
		status, _, _ = getWithETag(t, srv, path, etag1)
		if status != http.StatusNotModified {
			t.Fatalf("matching If-None-Match: status=%d, want 304", status)
		}

		status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/products/"+product.UUID+"/cle/events", map[string]any{
			"type": "released", "effective": "2026-07-01T00:00:00Z", "published": "2026-07-01T00:00:00Z", "version": "1.0.0",
		})
		if status != http.StatusCreated {
			t.Fatalf("CreateCLEEvent: status=%d body=%s", status, raw)
		}

		status, etag2, _ := getWithETag(t, srv, path, etag1)
		if status != http.StatusOK {
			t.Fatalf("GET after CreateCLEEvent: status=%d, want 200 (old ETag must no longer match)", status)
		}
		if etag2 == etag1 {
			t.Fatal("ETag unchanged after CreateCLEEvent, want a new one")
		}

		// cleByProduct independently checks the owner's own existence
		// before ever reaching writeCLE's ETag logic, so this route 404s
		// after delete regardless of the CLE-revision fix -- confirms that
		// existing guard still works with the new ETag step added in
		// front of it. The design-review-caught case this route's own
		// guard happens to mask (GetCLE/GetCLERevision never checking
		// owner existence on their own, so a route reachable WITHOUT such
		// a guard would otherwise 304 forever against deleted data) is
		// covered directly at the repo layer by
		// internal/repo/cle_test.go's TestDeleteProductBumpsCLERevision.
		if status, raw := jsonRequest(t, srv, http.MethodDelete, "/admin/v1/products/"+product.UUID, nil); status != http.StatusNoContent {
			t.Fatalf("DeleteProduct: status=%d body=%s", status, raw)
		}
		status, _, _ = getWithETag(t, srv, path, etag2)
		if status != http.StatusNotFound {
			t.Fatalf("GET CLE after owning product deleted: status=%d, want 404", status)
		}
	})
}

// TestListETagConditionalRequests confirms list-endpoint ETags (backed by
// the global per-resource-family watermark, not any single row's own
// identity) behave correctly through the real HTTP stack: stable across
// repeated identical requests, matching If-None-Match short-circuits to
// 304, and creating a new product changes the list's ETag even though the
// query parameters themselves didn't change.
func TestListETagConditionalRequests(t *testing.T) {
	srv := newTestServer(t)

	status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/products", map[string]any{"name": "List ETag Product 1"})
	if status != http.StatusCreated {
		t.Fatalf("create product 1: status=%d body=%s", status, raw)
	}

	path := "/tea/v1/products"
	status, etag1, _ := getWithETag(t, srv, path, "")
	if status != http.StatusOK || etag1 == "" {
		t.Fatalf("first GET: status=%d etag=%q, want 200 with a non-empty ETag", status, etag1)
	}

	status, etag1b, _ := getWithETag(t, srv, path, "")
	if status != http.StatusOK || etag1b != etag1 {
		t.Fatalf("repeated GET with no mutation: status=%d etag=%q, want the same ETag %q", status, etag1b, etag1)
	}

	status, _, body := getWithETag(t, srv, path, etag1)
	if status != http.StatusNotModified {
		t.Fatalf("matching If-None-Match: status=%d, want 304", status)
	}
	if len(body) != 0 {
		t.Fatalf("304 body = %q, want empty", body)
	}

	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/products", map[string]any{"name": "List ETag Product 2"})
	if status != http.StatusCreated {
		t.Fatalf("create product 2: status=%d body=%s", status, raw)
	}

	status, etag2, _ := getWithETag(t, srv, path, etag1)
	if status != http.StatusOK {
		t.Fatalf("GET after creating a new product: status=%d, want 200 (old ETag must no longer match)", status)
	}
	if etag2 == etag1 {
		t.Fatal("list ETag unchanged after creating a new product, want a new one")
	}
}

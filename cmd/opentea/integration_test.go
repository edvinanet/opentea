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

	cfg := config.Config{Versions: []string{"0.4.0"}}
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

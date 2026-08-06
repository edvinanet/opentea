package webadmin

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oej/opentea/internal/authn"
	"github.com/oej/opentea/internal/config"
	"github.com/oej/opentea/internal/db"
	"github.com/oej/opentea/internal/idgen"
	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/pkg/tea"
)

func newTestServer(t *testing.T) (*httptest.Server, *repo.Repo) {
	t.Helper()
	return newTestServerWithConfig(t, config.Config{RootURL: "http://example.test"})
}

// newTestServerWithConfig is newTestServer with a caller-chosen config --
// e.g. TrustProxyHeaders, for tests exercising proxy-aware behavior that
// depends on it.
func newTestServerWithConfig(t *testing.T, cfg config.Config) (*httptest.Server, *repo.Repo) {
	t.Helper()
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	r := repo.New(sqlDB)
	srv := httptest.NewServer(NewRouter(r, cfg))
	t.Cleanup(srv.Close)
	return srv, r
}

// seedBrowseData creates one product (with a release, linking one pinned
// component release), one CLE event, and one collection+artifact -- enough
// for every field the browsing page templates touch to actually be
// populated (empty slices/nil pointers exercise their own branches fine on
// their own, but this catches typos in fields that only render when data is
// present).
func seedBrowseData(t *testing.T, ctx context.Context, r *repo.Repo) (productUUID, productReleaseUUID, componentUUID, componentReleaseUUID string) {
	t.Helper()

	product, err := r.CreateProduct(ctx, "Test Product", []tea.Identifier{{IDType: "PURL", IDValue: "pkg:generic/test-product"}})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	release, err := r.CreateProductRelease(ctx, product.UUID, repo.ProductReleaseInput{
		Version: "1.0.0", CreatedDate: time.Now().UTC().Truncate(time.Second),
	})
	if err != nil {
		t.Fatalf("CreateProductRelease: %v", err)
	}

	component, err := r.CreateComponent(ctx, "libfoo", []tea.Identifier{{IDType: "PURL", IDValue: "pkg:generic/libfoo"}})
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	componentRelease, err := r.CreateComponentRelease(ctx, component.UUID, repo.ComponentReleaseInput{
		Version: "9.9.9", CreatedDate: time.Now().UTC().Truncate(time.Second),
	})
	if err != nil {
		t.Fatalf("CreateComponentRelease: %v", err)
	}
	releaseUUID := componentRelease.UUID
	if _, err := r.LinkComponent(ctx, release.UUID, tea.ComponentRef{UUID: component.UUID, Release: &releaseUUID}); err != nil {
		t.Fatalf("LinkComponent: %v", err)
	}

	artifact, err := r.CreateArtifact(ctx, repo.ArtifactInput{
		Type: "BOM", Formats: []repo.ArtifactFormatInput{{MediaType: "application/json"}},
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	if _, err := r.CreateCollectionForComponentRelease(ctx, componentRelease.UUID, repo.CollectionInput{
		Artifacts: []repo.ArtifactRef{{UUID: artifact.UUID, Version: artifact.Version}},
	}); err != nil {
		t.Fatalf("CreateCollectionForComponentRelease: %v", err)
	}

	if _, err := r.CreateCLEEvent(ctx, repo.OwnerProduct, product.UUID, repo.CLEEventInput{
		Type: "released", Effective: time.Now().UTC().Truncate(time.Second), Published: time.Now().UTC().Truncate(time.Second),
	}); err != nil {
		t.Fatalf("CreateCLEEvent: %v", err)
	}

	return product.UUID, release.UUID, component.UUID, componentRelease.UUID
}

// TestPagesRenderWithoutError exercises every template (login, dashboard,
// users, token, plus the "new token" and "form error" branches) end to end,
// to catch template parse/execute errors that only surface once a handler
// actually runs (html/template.Must only catches parse errors, not
// execute-time field-name typos).
func TestPagesRenderWithoutError(t *testing.T) {
	ctx := context.Background()
	srv, r := newTestServer(t)

	productUUID, productReleaseUUID, componentUUID, componentReleaseUUID := seedBrowseData(t, ctx, r)

	admin, err := r.CreateUser(ctx, "admin", "adminpass1", model.RoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, _, err := r.CreateSession(ctx, admin.UUID, authn.SessionTTL)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	client := srv.Client()
	get := func(path string, cookie string) (int, string) {
		req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: authn.SessionCookieName, Value: cookie})
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}
	post := func(path, cookie string, form map[string]string) (int, string) {
		vals := url.Values{}
		for k, v := range form {
			vals.Set(k, v)
		}
		req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(vals.Encode()))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		// Matches newTestServer's cfg.RootURL, not srv.URL (the httptest
		// listener's real address) -- requireRole's SameOrigin check
		// compares Origin against cfg.RootURL, not the actual listener.
		req.Header.Set("Origin", "http://example.test")
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: authn.SessionCookieName, Value: cookie})
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}

	if status, body := get("/admin/ui/login", ""); status != http.StatusOK {
		t.Fatalf("GET /admin/ui/login: status=%d body=%s", status, body)
	}

	if status, body := get("/admin/ui/", token); status != http.StatusOK {
		t.Fatalf("GET /admin/ui/: status=%d body=%s", status, body)
	}

	if status, body := get("/admin/ui/users", token); status != http.StatusOK {
		t.Fatalf("GET /admin/ui/users: status=%d body=%s", status, body)
	}

	// Trigger the "users" page error re-render branch (duplicate username).
	status, respBody := post("/admin/ui/users", token, map[string]string{"username": "admin", "password": "longenough1", "role": "consumer"})
	if status != http.StatusOK {
		t.Fatalf("POST /admin/ui/users (duplicate): status=%d body=%s", status, respBody)
	}
	if !strings.Contains(respBody, "username already taken") {
		t.Fatalf("expected the duplicate-username error, got body=%s", respBody)
	}

	if status, body := get("/admin/ui/token", token); status != http.StatusOK {
		t.Fatalf("GET /admin/ui/token: status=%d body=%s", status, body)
	}
	if status, body := post("/admin/ui/token/generate", token, nil); status != http.StatusOK {
		t.Fatalf("POST /admin/ui/token/generate: status=%d body=%s", status, body)
	}
	// Re-fetch the token page: exercises the "TokenInfo set, NewToken empty" branch.
	if status, body := get("/admin/ui/token", token); status != http.StatusOK {
		t.Fatalf("GET /admin/ui/token (after generating): status=%d body=%s", status, body)
	}

	// Login with a bad password renders the error branch of login.html.
	if status, body := post("/admin/ui/login", "", map[string]string{"username": "admin", "password": "wrong"}); status != http.StatusOK {
		t.Fatalf("POST /admin/ui/login (wrong password): status=%d body=%s", status, body)
	}

	// Data browsing pages -- list pages first (empty-linked-components/
	// empty-collections branches don't apply here since seedBrowseData
	// populates everything), then detail pages for the seeded product,
	// product release, component, and component release.
	for _, path := range []string{
		"/admin/ui/products",
		"/admin/ui/products/" + productUUID,
		"/admin/ui/productReleases/" + productReleaseUUID,
		"/admin/ui/components",
		"/admin/ui/components/" + componentUUID,
		"/admin/ui/componentReleases/" + componentReleaseUUID,
	} {
		if status, body := get(path, token); status != http.StatusOK {
			t.Fatalf("GET %s: status=%d body=%s", path, status, body)
		}
	}
}

// TestBrowsePagesNotFound confirms an invalid/missing uuid in a detail
// page's path renders 404, not a 500 -- these are the first webadmin
// handlers to take a uuid from the URL path, so this exercises a case none
// of the existing tests cover.
func TestBrowsePagesNotFound(t *testing.T) {
	ctx := context.Background()
	srv, r := newTestServer(t)

	admin, err := r.CreateUser(ctx, "admin", "adminpass1", model.RoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, _, err := r.CreateSession(ctx, admin.UUID, authn.SessionTTL)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	missing := idgen.New()
	for _, path := range []string{
		"/admin/ui/products/" + missing,
		"/admin/ui/productReleases/" + missing,
		"/admin/ui/components/" + missing,
		"/admin/ui/componentReleases/" + missing,
	} {
		req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.AddCookie(&http.Cookie{Name: authn.SessionCookieName, Value: token})
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s: status=%d, want 404", path, resp.StatusCode)
		}
	}
}

// TestProductReleaseDetailPageAfterLinkedComponentDeleted confirms deleting
// a linked component doesn't break its release's detail page: the
// product_release_component link row cascades away with the component
// (schema FK: component_uuid ... ON DELETE CASCADE), so the release simply
// shows no linked components afterward, rather than a dangling reference.
//
// This does NOT exercise productReleaseDetailPage's own
// errors.Is(err, repo.ErrNotFound) handling around its per-ref GetComponent
// call (see productreleases.go) -- that branch only defends against a
// narrower race (a component deleted by a concurrent request in the window
// between this request's own GetProductRelease call and its subsequent
// GetComponent calls), which the FK cascade means can't be produced through
// the repo API in a single sequential test. The branch is still correct
// defensive code; it's just not practically unit-testable without invasive
// raw-SQL test plumbing to manufacture a dangling row.
func TestProductReleaseDetailPageAfterLinkedComponentDeleted(t *testing.T) {
	ctx := context.Background()
	srv, r := newTestServer(t)

	admin, err := r.CreateUser(ctx, "admin", "adminpass1", model.RoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, _, err := r.CreateSession(ctx, admin.UUID, authn.SessionTTL)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	product, err := r.CreateProduct(ctx, "Test Product", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	release, err := r.CreateProductRelease(ctx, product.UUID, repo.ProductReleaseInput{
		Version: "1.0.0", CreatedDate: time.Now().UTC().Truncate(time.Second),
	})
	if err != nil {
		t.Fatalf("CreateProductRelease: %v", err)
	}
	component, err := r.CreateComponent(ctx, "libfoo", nil)
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	if _, err := r.LinkComponent(ctx, release.UUID, tea.ComponentRef{UUID: component.UUID}); err != nil {
		t.Fatalf("LinkComponent: %v", err)
	}
	if err := r.DeleteComponent(ctx, component.UUID); err != nil {
		t.Fatalf("DeleteComponent: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/admin/ui/productReleases/"+release.UUID, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: authn.SessionCookieName, Value: token})
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET productRelease detail: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET productRelease detail: status=%d, want 200; body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "No linked components.") {
		t.Errorf("response body doesn't show the empty-linked-components state; body=%s", body)
	}
	if strings.Contains(string(body), "/admin/ui/components/"+component.UUID) {
		t.Errorf("response body still links to the deleted component; body=%s", body)
	}
}

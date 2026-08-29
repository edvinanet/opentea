// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/pkg/tea"
)

// teaRequest hits /tea/v1 (unauthenticated, unless bearerToken is set) and
// returns the status, response headers (for ETag/Cache-Control assertions),
// and body.
func teaRequest(t *testing.T, srv *testServer, method, path, bearerToken string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp.StatusCode, resp.Header, body
}

// createTemplateAndEntitlement is a test helper: creates a template with
// the given rules, activates its first revision, and scopes an entitlement
// to (subjectType, subjectID, resourceType, resourceID) referencing it.
// Returns the entitlement UUID.
func createTemplateAndEntitlement(t *testing.T, srv *testServer, rules []map[string]any, subjectType, subjectID, resourceType, resourceID string) string {
	t.Helper()
	status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/templates", map[string]any{
		"name":  t.Name() + "-template-" + resourceType + "-" + resourceID,
		"rules": rules,
	})
	if status != http.StatusCreated {
		t.Fatalf("POST /admin/v1/templates: status=%d body=%s", status, raw)
	}
	var tmpl struct {
		model.Template
		Revision model.TemplateRevision `json:"revision"`
	}
	decodeInto(t, raw, &tmpl)

	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/templates/"+tmpl.UUID+"/activate", map[string]any{"revision": tmpl.Revision.Revision})
	if status != http.StatusOK {
		t.Fatalf("POST .../activate: status=%d body=%s", status, raw)
	}

	body := map[string]any{
		"subjectType":       subjectType,
		"templateUuid":      tmpl.UUID,
		"templateRevision":  tmpl.Revision.Revision,
		"resourceType":      resourceType,
		"grantingAuthority": "test-suite",
	}
	if subjectID != "" {
		body["subjectId"] = subjectID
	}
	if resourceID != "" {
		body["resourceId"] = resourceID
	}
	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/entitlements", body)
	if status != http.StatusCreated {
		t.Fatalf("POST /admin/v1/entitlements: status=%d body=%s", status, raw)
	}
	var ent model.Entitlement
	decodeInto(t, raw, &ent)
	return ent.UUID
}

// TestTeaV1AuthzAnonymousDefaultAllowsRead confirms the migration-seeded
// bootstrap entitlement preserves today's "anyone can read anything"
// behavior on a fresh server -- the critical non-regression check before
// any admin narrows anything.
func TestTeaV1AuthzAnonymousDefaultAllowsRead(t *testing.T) {
	srv := newTestServer(t)

	status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/products", map[string]any{"name": "Widget"})
	if status != http.StatusCreated {
		t.Fatalf("POST /admin/v1/products: status=%d body=%s", status, raw)
	}
	var product tea.Product
	decodeInto(t, raw, &product)

	status, _, raw = teaRequest(t, srv, http.MethodGet, "/tea/v1/product/"+product.UUID, "")
	if status != http.StatusOK {
		t.Fatalf("anonymous GET /tea/v1/product/%s: status=%d body=%s, want 200", product.UUID, status, raw)
	}
}

// TestTeaV1AuthzCapabilityIndependence proves spec Sec 12.2: a
// release-scoped entitlement that grants release.read but denies
// collection.read must NOT leak collection access, even though the
// broader (migration-seeded) bootstrap entitlement would otherwise allow
// it -- the narrower, more specific restriction wins.
func TestTeaV1AuthzCapabilityIndependence(t *testing.T) {
	srv := newTestServer(t)

	status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/products", map[string]any{"name": "Widget"})
	if status != http.StatusCreated {
		t.Fatalf("POST /admin/v1/products: status=%d body=%s", status, raw)
	}
	var product tea.Product
	decodeInto(t, raw, &product)

	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/products/"+product.UUID+"/releases", map[string]any{
		"version":     "1.0",
		"createdDate": "2026-01-01T00:00:00Z",
	})
	if status != http.StatusCreated {
		t.Fatalf("POST .../releases: status=%d body=%s", status, raw)
	}
	var release tea.ProductRelease
	decodeInto(t, raw, &release)

	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/productReleases/"+release.UUID+"/collections", map[string]any{})
	if status != http.StatusCreated {
		t.Fatalf("POST .../collections: status=%d body=%s", status, raw)
	}

	createTemplateAndEntitlement(t, srv,
		[]map[string]any{
			{"capability": "release.read", "decision": "allow"},
			{"capability": "collection.read", "decision": "deny"},
		},
		"everyone", "", "product_release", release.UUID,
	)

	status, _, raw = teaRequest(t, srv, http.MethodGet, "/tea/v1/productRelease/"+release.UUID, "")
	if status != http.StatusOK {
		t.Fatalf("GET productRelease: status=%d body=%s, want 200 (release.read allowed)", status, raw)
	}

	status, _, raw = teaRequest(t, srv, http.MethodGet, "/tea/v1/productRelease/"+release.UUID+"/collection/latest", "")
	if status != http.StatusNotFound {
		t.Fatalf("GET collection/latest: status=%d body=%s, want 404 (collection.read denied, narrower than bootstrap's broader allow)", status, raw)
	}
}

// TestTeaV1AuthzPaginationFiltering proves spec Sec 18: list pagination
// must operate on the authorized set, not the underlying table -- a denied
// row must be excluded from results and not distort hasNext.
func TestTeaV1AuthzPaginationFiltering(t *testing.T) {
	srv := newTestServer(t)

	var products []tea.Product
	for i := 0; i < 3; i++ {
		status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/products", map[string]any{"name": "Widget"})
		if status != http.StatusCreated {
			t.Fatalf("POST /admin/v1/products: status=%d body=%s", status, raw)
		}
		var p tea.Product
		decodeInto(t, raw, &p)
		products = append(products, p)
	}

	// Deny product.read on the first-sorted product specifically --
	// narrower than the bootstrap's broad all_products allow, so it wins.
	createTemplateAndEntitlement(t, srv,
		[]map[string]any{{"capability": "product.read", "decision": "deny"}},
		"everyone", "", "product", products[0].UUID,
	)

	status, _, raw := teaRequest(t, srv, http.MethodGet, "/tea/v1/products?pageSize=100", "")
	if status != http.StatusOK {
		t.Fatalf("GET /tea/v1/products: status=%d body=%s", status, raw)
	}
	var page tea.PaginatedProducts
	decodeInto(t, raw, &page)
	for _, p := range page.Results {
		if p.UUID == products[0].UUID {
			t.Fatalf("denied product %s leaked into results: %+v", products[0].UUID, page.Results)
		}
	}
	if page.HasNext {
		t.Fatalf("HasNext = true with only %d authorized products and pageSize=100, want false", len(page.Results))
	}

	// pageSize=1 forces filterAuthorized's refetch loop to actually engage
	// when the very first underlying row is the denied one.
	status, _, raw = teaRequest(t, srv, http.MethodGet, "/tea/v1/products?pageSize=1&sortField=name&sortOrder=asc", "")
	if status != http.StatusOK {
		t.Fatalf("GET /tea/v1/products?pageSize=1: status=%d body=%s", status, raw)
	}
	var firstPage tea.PaginatedProducts
	decodeInto(t, raw, &firstPage)
	if len(firstPage.Results) != 1 {
		t.Fatalf("Results = %+v, want exactly 1 authorized product", firstPage.Results)
	}
	if firstPage.Results[0].UUID == products[0].UUID {
		t.Fatalf("denied product %s returned in a filtered page", products[0].UUID)
	}
}

// TestTeaV1AuthzPerPrincipalCaching proves the ETag/Cache-Control fix: two
// different authenticated principals with different effective entitlements
// must get different ETags for the identical URL, and an authenticated
// response must be marked private (never safe for a shared cache), while
// an anonymous one stays public.
func TestTeaV1AuthzPerPrincipalCaching(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/products", map[string]any{"name": "Widget"})
	if status != http.StatusCreated {
		t.Fatalf("POST /admin/v1/products: status=%d body=%s", status, raw)
	}
	var product tea.Product
	decodeInto(t, raw, &product)

	alice, err := srv.repo.CreateUser(ctx, "alice", "password123", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser alice: %v", err)
	}
	aliceToken, err := srv.repo.SetAPIToken(ctx, alice.UUID)
	if err != nil {
		t.Fatalf("SetAPIToken alice: %v", err)
	}
	bob, err := srv.repo.CreateUser(ctx, "bob", "password123", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser bob: %v", err)
	}
	bobToken, err := srv.repo.SetAPIToken(ctx, bob.UUID)
	if err != nil {
		t.Fatalf("SetAPIToken bob: %v", err)
	}

	// Give alice (and only alice) a principal-scoped denial on this
	// product -- narrower than the bootstrap allow, so it wins for her.
	createTemplateAndEntitlement(t, srv,
		[]map[string]any{{"capability": "product.read", "decision": "deny"}},
		"principal", alice.UUID, "product", product.UUID,
	)

	anonStatus, anonHeaders, _ := teaRequest(t, srv, http.MethodGet, "/tea/v1/product/"+product.UUID, "")
	aliceStatus, aliceHeaders, _ := teaRequest(t, srv, http.MethodGet, "/tea/v1/product/"+product.UUID, aliceToken)
	bobStatus, bobHeaders, _ := teaRequest(t, srv, http.MethodGet, "/tea/v1/product/"+product.UUID, bobToken)

	if anonStatus != http.StatusOK {
		t.Errorf("anonymous status = %d, want 200", anonStatus)
	}
	if aliceStatus != http.StatusNotFound {
		t.Errorf("alice status = %d, want 404 (principal-scoped deny)", aliceStatus)
	}
	if bobStatus != http.StatusOK {
		t.Errorf("bob status = %d, want 200 (no entitlement targets him specifically)", bobStatus)
	}

	if got := anonHeaders.Get("Cache-Control"); !strings.Contains(got, "public") {
		t.Errorf("anonymous Cache-Control = %q, want it to contain \"public\"", got)
	}
	if got := bobHeaders.Get("Cache-Control"); !strings.Contains(got, "private") {
		t.Errorf("bob (authenticated) Cache-Control = %q, want it to contain \"private\"", got)
	}
	if anonHeaders.Get("ETag") == bobHeaders.Get("ETag") {
		t.Errorf("anonymous and authenticated ETags must differ (different cache partitions), both = %q", anonHeaders.Get("ETag"))
	}
	if bobHeaders.Get("ETag") == "" {
		t.Errorf("bob's ETag must not be empty")
	}
	// alice's response is a 404, which carries no ETag from s.conditional
	// (authorize returns before conditional runs) -- nothing to compare
	// there.
	_ = aliceHeaders
}

// TestTeaV1AuthzAdminAuditAtomic confirms an admin mutation produces a
// matching admin_audit_log row (spec Sec 22.5).
func TestTeaV1AuthzAdminAuditAtomic(t *testing.T) {
	srv := newTestServer(t)

	status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/templates", map[string]any{
		"name":  "Audited Template",
		"rules": []map[string]any{{"capability": "product.read", "decision": "allow"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("POST /admin/v1/templates: status=%d body=%s", status, raw)
	}
	var tmpl struct {
		model.Template
		Revision model.TemplateRevision `json:"revision"`
	}
	decodeInto(t, raw, &tmpl)

	entries, err := srv.repo.ListAdminAudit(context.Background(), "template", tmpl.UUID, 10)
	if err != nil {
		t.Fatalf("ListAdminAudit: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("audit entries = %+v, want exactly 1", entries)
	}
	if entries[0].Operation != "template.create" {
		t.Errorf("Operation = %q, want %q", entries[0].Operation, "template.create")
	}
	if entries[0].ActorUUID != srv.adminUser.UUID {
		t.Errorf("ActorUUID = %q, want %q", entries[0].ActorUUID, srv.adminUser.UUID)
	}
}

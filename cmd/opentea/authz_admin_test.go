// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"net/http"
	"testing"

	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/pkg/tea"
)

// TestAdminAuthzTemplateAndEntitlementCRUD drives the /admin/v1 templates/
// entitlements/productGroups surface end-to-end through the real HTTP
// stack: create a template, activate it, group a product, scope an
// entitlement to that group, and confirm the whole chain round-trips
// (including the DELETE-while-referenced 409 and the invalid-capability
// 400).
func TestAdminAuthzTemplateAndEntitlementCRUD(t *testing.T) {
	srv := newTestServer(t)

	status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/products", map[string]any{"name": "Widget"})
	if status != http.StatusCreated {
		t.Fatalf("POST /admin/v1/products: status=%d body=%s", status, raw)
	}
	var product tea.Product
	decodeInto(t, raw, &product)

	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/templates", map[string]any{
		"name": "Public Read",
		"rules": []map[string]any{
			{"capability": "product.read", "decision": "allow"},
			{"capability": "artifact.download", "artifactType": "BOM", "decision": "deny"},
		},
	})
	if status != http.StatusCreated {
		t.Fatalf("POST /admin/v1/templates: status=%d body=%s", status, raw)
	}
	var created struct {
		model.Template
		Revision model.TemplateRevision `json:"revision"`
	}
	decodeInto(t, raw, &created)
	if created.ActiveRevision != 0 {
		t.Fatalf("ActiveRevision = %d, want 0 before explicit activation", created.ActiveRevision)
	}
	if created.Revision.Revision != 1 || len(created.Revision.Rules) != 2 {
		t.Fatalf("Revision = %+v", created.Revision)
	}

	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/templates/"+created.UUID+"/activate", map[string]any{"revision": 1})
	if status != http.StatusOK {
		t.Fatalf("POST .../activate: status=%d body=%s", status, raw)
	}
	var activated model.Template
	decodeInto(t, raw, &activated)
	if activated.ActiveRevision != 1 {
		t.Fatalf("ActiveRevision = %d, want 1", activated.ActiveRevision)
	}

	status, raw = jsonRequest(t, srv, http.MethodGet, "/admin/v1/templates/"+created.UUID, nil)
	if status != http.StatusOK {
		t.Fatalf("GET template: status=%d body=%s", status, raw)
	}
	var fetched struct {
		model.Template
		ActiveRules []model.TemplateCapabilityRule `json:"activeRules"`
	}
	decodeInto(t, raw, &fetched)
	if len(fetched.ActiveRules) != 2 {
		t.Fatalf("ActiveRules = %+v, want 2 rules", fetched.ActiveRules)
	}

	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/productGroups", map[string]any{"name": "Enterprise"})
	if status != http.StatusCreated {
		t.Fatalf("POST /admin/v1/productGroups: status=%d body=%s", status, raw)
	}
	var group model.ProductGroup
	decodeInto(t, raw, &group)

	status, _ = jsonRequest(t, srv, http.MethodPost, "/admin/v1/productGroups/"+group.UUID+"/members", map[string]any{"productUuid": product.UUID})
	if status != http.StatusNoContent {
		t.Fatalf("POST .../members: status=%d", status)
	}

	status, raw = jsonRequest(t, srv, http.MethodGet, "/admin/v1/productGroups/"+group.UUID, nil)
	if status != http.StatusOK {
		t.Fatalf("GET productGroup: status=%d body=%s", status, raw)
	}
	var groupResp struct {
		model.ProductGroup
		Members []string `json:"members"`
	}
	decodeInto(t, raw, &groupResp)
	if len(groupResp.Members) != 1 || groupResp.Members[0] != product.UUID {
		t.Fatalf("Members = %+v, want [%s]", groupResp.Members, product.UUID)
	}

	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/entitlements", map[string]any{
		"subjectType":       "everyone",
		"templateUuid":      created.UUID,
		"templateRevision":  1,
		"resourceType":      "product_group",
		"resourceId":        group.UUID,
		"grantingAuthority": "test-suite",
	})
	if status != http.StatusCreated {
		t.Fatalf("POST /admin/v1/entitlements: status=%d body=%s", status, raw)
	}
	var entitlement model.Entitlement
	decodeInto(t, raw, &entitlement)
	if entitlement.Status != model.EntitlementActive {
		t.Fatalf("Status = %q, want active", entitlement.Status)
	}

	status, raw = jsonRequest(t, srv, http.MethodGet, "/admin/v1/entitlements?resourceType=product_group&resourceId="+group.UUID, nil)
	if status != http.StatusOK {
		t.Fatalf("GET entitlements filtered: status=%d body=%s", status, raw)
	}
	var filtered []model.Entitlement
	decodeInto(t, raw, &filtered)
	if len(filtered) != 1 || filtered[0].UUID != entitlement.UUID {
		t.Fatalf("filtered = %+v, want exactly [%s]", filtered, entitlement.UUID)
	}

	status, raw = jsonRequest(t, srv, http.MethodPatch, "/admin/v1/entitlements/"+entitlement.UUID+"/status", map[string]any{"status": "suspended"})
	if status != http.StatusOK {
		t.Fatalf("PATCH .../status: status=%d body=%s", status, raw)
	}
	var suspended model.Entitlement
	decodeInto(t, raw, &suspended)
	if suspended.Status != model.EntitlementSuspended || suspended.Revision != 2 {
		t.Fatalf("suspended = %+v, want status=suspended revision=2", suspended)
	}

	// Deleting the template while the entitlement still references it must
	// be rejected (409), not silently succeed or 500.
	status, raw = jsonRequest(t, srv, http.MethodDelete, "/admin/v1/templates/"+created.UUID, nil)
	if status != http.StatusConflict {
		t.Fatalf("DELETE template while in use: status=%d body=%s, want 409", status, raw)
	}

	status, _ = jsonRequest(t, srv, http.MethodDelete, "/admin/v1/entitlements/"+entitlement.UUID, nil)
	if status != http.StatusNoContent {
		t.Fatalf("DELETE entitlement: status=%d", status)
	}
	status, raw = jsonRequest(t, srv, http.MethodGet, "/admin/v1/entitlements/"+entitlement.UUID, nil)
	if status != http.StatusNotFound {
		t.Fatalf("GET deleted entitlement: status=%d body=%s, want 404", status, raw)
	}

	status, _ = jsonRequest(t, srv, http.MethodDelete, "/admin/v1/templates/"+created.UUID, nil)
	if status != http.StatusNoContent {
		t.Fatalf("DELETE template after entitlement removed: status=%d", status)
	}
}

func TestAdminAuthzInvalidCapabilityRejected(t *testing.T) {
	srv := newTestServer(t)

	status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/templates", map[string]any{
		"name": "Bad Template",
		"rules": []map[string]any{
			{"capability": "product.destroy", "decision": "allow"},
		},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("POST /admin/v1/templates (invalid capability): status=%d body=%s, want 400", status, raw)
	}
}

func TestAdminAuthzReleaseGroupCRUD(t *testing.T) {
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

	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/releaseGroups", map[string]any{"name": "LTS"})
	if status != http.StatusCreated {
		t.Fatalf("POST /admin/v1/releaseGroups: status=%d body=%s", status, raw)
	}
	var group model.ReleaseGroup
	decodeInto(t, raw, &group)

	status, _ = jsonRequest(t, srv, http.MethodPost, "/admin/v1/releaseGroups/"+group.UUID+"/members", map[string]any{"productReleaseUuid": release.UUID})
	if status != http.StatusNoContent {
		t.Fatalf("POST .../members: status=%d", status)
	}

	status, raw = jsonRequest(t, srv, http.MethodGet, "/admin/v1/releaseGroups/"+group.UUID, nil)
	if status != http.StatusOK {
		t.Fatalf("GET releaseGroup: status=%d body=%s", status, raw)
	}
	var groupResp struct {
		model.ReleaseGroup
		Members []string `json:"members"`
	}
	decodeInto(t, raw, &groupResp)
	if len(groupResp.Members) != 1 || groupResp.Members[0] != release.UUID {
		t.Fatalf("Members = %+v, want [%s]", groupResp.Members, release.UUID)
	}

	status, _ = jsonRequest(t, srv, http.MethodDelete, "/admin/v1/releaseGroups/"+group.UUID+"/members/"+release.UUID, nil)
	if status != http.StatusNoContent {
		t.Fatalf("DELETE member: status=%d", status)
	}
	status, _ = jsonRequest(t, srv, http.MethodDelete, "/admin/v1/releaseGroups/"+group.UUID, nil)
	if status != http.StatusNoContent {
		t.Fatalf("DELETE releaseGroup: status=%d", status)
	}
}

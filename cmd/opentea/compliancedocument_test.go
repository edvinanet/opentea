// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"net/http"
	"testing"

	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/pkg/tea"
)

// TestAdminComplianceDocumentValidation confirms internal/repo's new TEA
// 1.0 COMPLIANCE_DOCUMENT rules surface through /admin/v1 as 400, not 500
// or a silent success: valid on a component/componentRelease, rejected on
// a product regardless of value validity.
func TestAdminComplianceDocumentValidation(t *testing.T) {
	srv := newTestServer(t)

	status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/components", map[string]any{
		"name":        "Widget lib",
		"identifiers": []tea.Identifier{{IDType: "COMPLIANCE_DOCUMENT", IDValue: "SOC_2_TYPE_II"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("POST /admin/v1/components (valid): status=%d body=%s", status, raw)
	}
	var component tea.Component
	decodeInto(t, raw, &component)

	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/components/"+component.UUID+"/releases", map[string]any{
		"version":     "1.0.0",
		"createdDate": "2026-01-01T00:00:00Z",
		"identifiers": []tea.Identifier{{IDType: "COMPLIANCE_DOCUMENT", IDValue: "ISO_27001"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("POST .../releases (valid): status=%d body=%s", status, raw)
	}

	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/components", map[string]any{
		"name":        "Bad value",
		"identifiers": []tea.Identifier{{IDType: "COMPLIANCE_DOCUMENT", IDValue: "NOT_A_REAL_TYPE"}},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("POST /admin/v1/components (invalid value): status=%d body=%s, want 400", status, raw)
	}

	status, raw = jsonRequest(t, srv, http.MethodPost, "/admin/v1/products", map[string]any{
		"name":        "Wrong owner",
		"identifiers": []tea.Identifier{{IDType: "COMPLIANCE_DOCUMENT", IDValue: "GDPR"}},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("POST /admin/v1/products (wrong owner): status=%d body=%s, want 400", status, raw)
	}
}

// TestPublisherComplianceDocumentValidation mirrors the admin-side test
// against /publisher/v1 -- the fix lives in internal/repo, shared by both
// write APIs, but each handler needed its own error-mapping added, so
// this confirms that actually happened on the publisher side too.
func TestPublisherComplianceDocumentValidation(t *testing.T) {
	srv := newTestServer(t)
	full := createPublisherCredential(t, srv, "full-cred", model.PublisherScopeFull)

	status, raw := publisherRequest(t, srv, http.MethodPost, "/publisher/v1/components", full, map[string]any{
		"name":        "Widget lib",
		"identifiers": []tea.Identifier{{IDType: "COMPLIANCE_DOCUMENT", IDValue: "SOC_2_TYPE_II"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("POST /publisher/v1/components (valid): status=%d body=%s", status, raw)
	}

	status, raw = publisherRequest(t, srv, http.MethodPost, "/publisher/v1/products", full, map[string]any{
		"name":        "Wrong owner",
		"identifiers": []tea.Identifier{{IDType: "COMPLIANCE_DOCUMENT", IDValue: "GDPR"}},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("POST /publisher/v1/products (wrong owner): status=%d body=%s, want 400", status, raw)
	}
}

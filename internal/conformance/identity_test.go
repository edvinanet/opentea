// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package conformance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oej/opentea/pkg/tea"
	"github.com/oej/opentea/pkg/teaclient"
)

func fakeProductsServer(t *testing.T, byPURL map[string][]tea.Product) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idValue := r.URL.Query().Get("idValue")
		results := byPURL[idValue]
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"hasNext": false, "results": results}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestResolveProductByPURLExactlyOneMatch(t *testing.T) {
	want := tea.Product{UUID: "server-assigned-uuid", Name: "Acme Widget [test:aaaaaaaa-0000-0000-0000-000000000000]"}
	srv := fakeProductsServer(t, map[string][]tea.Product{"pkg:generic/acme-widget": {want}})

	r := NewResolver(teaclient.NewClient(srv.URL))
	got, err := r.ResolveProductByPURL(context.Background(), "pkg:generic/acme-widget")
	if err != nil {
		t.Fatalf("ResolveProductByPURL: %v", err)
	}
	if got.UUID != want.UUID {
		t.Fatalf("UUID = %q, want %q", got.UUID, want.UUID)
	}
}

func TestResolveProductByPURLNoMatch(t *testing.T) {
	srv := fakeProductsServer(t, map[string][]tea.Product{})

	r := NewResolver(teaclient.NewClient(srv.URL))
	_, err := r.ResolveProductByPURL(context.Background(), "pkg:generic/nonexistent")
	if err == nil {
		t.Fatal("expected an error for zero matches, got nil")
	}
}

func TestResolveProductByPURLAmbiguousMatch(t *testing.T) {
	srv := fakeProductsServer(t, map[string][]tea.Product{
		"pkg:generic/ambiguous": {{UUID: "a"}, {UUID: "b"}},
	})

	r := NewResolver(teaclient.NewClient(srv.URL))
	_, err := r.ResolveProductByPURL(context.Background(), "pkg:generic/ambiguous")
	if err == nil {
		t.Fatal("expected an error for more than one match, got nil")
	}
}

func TestPurlOf(t *testing.T) {
	ids := []tea.Identifier{
		{IDType: tea.IdentifierTypeCPE, IDValue: "cpe:something"},
		{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/x"},
	}
	purl, ok := purlOf(ids)
	if !ok || purl != "pkg:generic/x" {
		t.Fatalf("purlOf = %q, %v, want %q, true", purl, ok, "pkg:generic/x")
	}
	if _, ok := purlOf(nil); ok {
		t.Fatal("purlOf(nil) = ok, want false")
	}
}

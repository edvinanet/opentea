package teaclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oej/opentea/pkg/tea"
)

func newFakeServer(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewClient(srv.URL), srv
}

func TestGetProduct(t *testing.T) {
	client, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/product/abc-123" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %q", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tea.Product{UUID: "abc-123", Name: "Acme Widget", Identifiers: []tea.Identifier{}})
	})

	p, err := client.GetProduct(context.Background(), "abc-123")
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if p.UUID != "abc-123" || p.Name != "Acme Widget" {
		t.Fatalf("p = %+v", p)
	}
}

func TestGetProductNotFound(t *testing.T) {
	client, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(tea.ErrorResponse{Error: tea.ErrorObjectUnknown})
	})

	_, err := client.GetProduct(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !IsNotFound(err) {
		t.Fatalf("err = %v, want IsNotFound", err)
	}
}

func TestBearerTokenAttached(t *testing.T) {
	var gotHeader string
	_, srv := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(tea.PaginatedProducts{Results: []tea.Product{}})
	})

	client := NewClient(srv.URL, WithBearerToken("my-token"))
	if _, err := client.QueryProducts(context.Background(), ListParams{}); err != nil {
		t.Fatalf("QueryProducts: %v", err)
	}
	if gotHeader != "Bearer my-token" {
		t.Fatalf("Authorization header = %q, want %q", gotHeader, "Bearer my-token")
	}
}

func TestQueryProductsPagination(t *testing.T) {
	client, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("pageSize") != "2" {
			t.Errorf("pageSize = %q", q.Get("pageSize"))
		}
		if q.Get("pageToken") == "" {
			json.NewEncoder(w).Encode(tea.PaginatedProducts{
				PaginationDetails: tea.PaginationDetails{HasNext: true, NextPageToken: "page2"},
				Results:           []tea.Product{{UUID: "1", Name: "a"}, {UUID: "2", Name: "b"}},
			})
			return
		}
		json.NewEncoder(w).Encode(tea.PaginatedProducts{
			PaginationDetails: tea.PaginationDetails{HasNext: false},
			Results:           []tea.Product{{UUID: "3", Name: "c"}},
		})
	})

	all, err := ListAll(func(token string) ([]tea.Product, bool, string, error) {
		resp, err := client.QueryProducts(context.Background(), ListParams{PageSize: 2, PageToken: token})
		return resp.Results, resp.HasNext, resp.NextPageToken, err
	})
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("ListAll returned %d products, want 3", len(all))
	}
}

func TestDownloadAndVerifyChecksumOK(t *testing.T) {
	content := []byte("hello, tea client")
	sum := sha256.Sum256(content)
	hexSum := hex.EncodeToString(sum[:])

	client, srv := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	})

	format := tea.ArtifactFormat{
		URL:       srv.URL + "/files/whatever",
		Checksums: []tea.Checksum{{AlgType: tea.ChecksumTypeSHA256, AlgValue: hexSum}},
	}
	got, err := client.DownloadAndVerify(context.Background(), format)
	if err != nil {
		t.Fatalf("DownloadAndVerify: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("got %q, want %q", got, content)
	}
}

func TestDownloadAndVerifyChecksumMismatch(t *testing.T) {
	client, srv := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("tampered content"))
	})

	format := tea.ArtifactFormat{
		URL:       srv.URL + "/files/whatever",
		Checksums: []tea.Checksum{{AlgType: tea.ChecksumTypeSHA256, AlgValue: "0000000000000000000000000000000000000000000000000000000000000000"}},
	}
	if _, err := client.DownloadAndVerify(context.Background(), format); err == nil {
		t.Fatal("expected a checksum mismatch error")
	}
}

func TestDiscover(t *testing.T) {
	client, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tei") != "urn:tei:example" {
			t.Errorf("tei = %q", r.URL.Query().Get("tei"))
		}
		json.NewEncoder(w).Encode([]tea.DiscoveryInfo{{ProductReleaseUUID: "pr-1"}})
	})

	results, err := client.Discover(context.Background(), "urn:tei:example")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(results) != 1 || results[0].ProductReleaseUUID != "pr-1" {
		t.Fatalf("results = %+v", results)
	}
}

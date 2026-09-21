// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teaclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
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
		_ = json.NewEncoder(w).Encode(tea.Product{UUID: "abc-123", Name: "Acme Widget", Identifiers: []tea.Identifier{}})
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
		_ = json.NewEncoder(w).Encode(tea.ErrorResponse{Error: tea.ErrorObjectUnknown})
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
		_ = json.NewEncoder(w).Encode(tea.PaginatedProducts{Results: []tea.Product{}})
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
			_ = json.NewEncoder(w).Encode(tea.PaginatedProducts{
				PaginationDetails: tea.PaginationDetails{HasNext: true, NextPageToken: "page2"},
				Results:           []tea.Product{{UUID: "1", Name: "a"}, {UUID: "2", Name: "b"}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(tea.PaginatedProducts{
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
		_, _ = w.Write(content)
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
		_, _ = w.Write([]byte("tampered content"))
	})

	format := tea.ArtifactFormat{
		URL:       srv.URL + "/files/whatever",
		Checksums: []tea.Checksum{{AlgType: tea.ChecksumTypeSHA256, AlgValue: "0000000000000000000000000000000000000000000000000000000000000000"}},
	}
	if _, err := client.DownloadAndVerify(context.Background(), format); err == nil {
		t.Fatal("expected a checksum mismatch error")
	}
}

// TestDownloadAndVerifyRejectsOversizedResponse is the regression test for
// DownloadAndVerify's unbounded in-memory buffer: a response larger than
// maxDownloadAndVerifyBody must be rejected with a clear error, not
// buffered in full regardless of size.
func TestDownloadAndVerifyRejectsOversizedResponse(t *testing.T) {
	oversized := bytes.Repeat([]byte("a"), maxDownloadAndVerifyBody+1)
	client, srv := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(oversized)
	})

	format := tea.ArtifactFormat{URL: srv.URL + "/files/whatever"}
	if _, err := client.DownloadAndVerify(context.Background(), format); err == nil {
		t.Fatal("expected an error for a response exceeding maxDownloadAndVerifyBody")
	}
}

// TestDownloadAndVerifyToIgnoresDownloadAndVerifyLimit confirms
// DownloadAndVerifyTo's own contract is untouched by the limit added to
// DownloadAndVerify above: content larger than maxDownloadAndVerifyBody
// still streams through to io.Discard without error, since
// DownloadAndVerifyTo never buffers in memory at all.
func TestDownloadAndVerifyToIgnoresDownloadAndVerifyLimit(t *testing.T) {
	content := bytes.Repeat([]byte("a"), maxDownloadAndVerifyBody+1)
	sum := sha256.Sum256(content)
	hexSum := hex.EncodeToString(sum[:])

	client, srv := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(content)
	})

	format := tea.ArtifactFormat{
		URL:       srv.URL + "/files/whatever",
		Checksums: []tea.Checksum{{AlgType: tea.ChecksumTypeSHA256, AlgValue: hexSum}},
	}
	if err := client.DownloadAndVerifyTo(context.Background(), format, io.Discard); err != nil {
		t.Fatalf("DownloadAndVerifyTo: %v, want success for content over DownloadAndVerify's (unrelated) limit", err)
	}
}

// TestDownloadAndVerifyToStreamsWithoutBuffering is the regression test for
// the "artifact downloads are unbounded in memory" finding: dst receives
// the content and the checksum still verifies, proving DownloadAndVerifyTo
// actually writes through to the caller-supplied destination while hashing,
// rather than accumulating its own internal buffer the caller never sees.
func TestDownloadAndVerifyToStreamsWithoutBuffering(t *testing.T) {
	content := []byte("hello, streaming tea client")
	sum := sha256.Sum256(content)
	hexSum := hex.EncodeToString(sum[:])

	client, srv := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(content)
	})

	format := tea.ArtifactFormat{
		URL:       srv.URL + "/files/whatever",
		Checksums: []tea.Checksum{{AlgType: tea.ChecksumTypeSHA256, AlgValue: hexSum}},
	}
	var dst bytes.Buffer
	if err := client.DownloadAndVerifyTo(context.Background(), format, &dst); err != nil {
		t.Fatalf("DownloadAndVerifyTo: %v", err)
	}
	if dst.String() != string(content) {
		t.Fatalf("dst = %q, want %q", dst.String(), content)
	}
}

// TestDownloadAndVerifyToDiscardsWithoutDownloadingWhenUnsupported checks
// that an unsupported checksum algorithm is caught before any HTTP request
// is made -- verifying that io.Discard is a genuinely safe, zero-buffering
// way to call this for verification only (the whole point of adding it).
func TestDownloadAndVerifyToDiscardsWithoutDownloadingWhenUnsupported(t *testing.T) {
	requested := false
	client, srv := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		requested = true
		_, _ = w.Write([]byte("should never be fetched"))
	})

	format := tea.ArtifactFormat{
		URL:       srv.URL + "/files/whatever",
		Checksums: []tea.Checksum{{AlgType: "BLAKE3", AlgValue: "deadbeef"}},
	}
	err := client.DownloadAndVerifyTo(context.Background(), format, io.Discard)
	if err == nil {
		t.Fatal("expected an unsupported-algorithm error")
	}
	if requested {
		t.Fatal("expected the unsupported algorithm to be caught before any request was made")
	}
}

func TestDoRejectsOversizedResponse(t *testing.T) {
	client, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Otherwise-valid JSON, padded past maxResponseBody with a long
		// "name" string value -- must be rejected purely for being too
		// large. (Using invalid/non-JSON padding here would make this test
		// pass for the wrong reason: json.Unmarshal failing on malformed
		// input, not the size guard actually triggering.)
		padding := bytes.Repeat([]byte("a"), maxResponseBody+1)
		_, _ = w.Write([]byte(`{"uuid":"abc-123","name":"`))
		_, _ = w.Write(padding)
		_, _ = w.Write([]byte(`"}`))
	})

	_, err := client.GetProduct(context.Background(), "abc-123")
	if err == nil {
		t.Fatal("expected an error for a response exceeding maxResponseBody")
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		t.Fatalf("err = %v (*APIError), want a body-too-large error instead", err)
	}
}

func TestDiscover(t *testing.T) {
	client, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tei") != "urn:tei:example" {
			t.Errorf("tei = %q", r.URL.Query().Get("tei"))
		}
		_ = json.NewEncoder(w).Encode([]tea.DiscoveryInfo{{ProductReleaseUUID: "pr-1"}})
	})

	results, err := client.Discover(context.Background(), "urn:tei:example")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(results) != 1 || results[0].ProductReleaseUUID != "pr-1" {
		t.Fatalf("results = %+v", results)
	}
}

// TestDiscoverNoMatchIsNotFound confirms Discover surfaces upstream TEA
// 1.0's no-match response (404 + OBJECT_UNKNOWN, spec/openapi.yaml) as an
// error, not as a successful empty slice -- the distinction
// bootstrapDiscoverWithAuthority's failover logic (wellknown.go) relies on
// to tell "this endpoint doesn't have it, try the next one" apart from "a
// genuine, if empty, match."
func TestDiscoverNoMatchIsNotFound(t *testing.T) {
	client, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(tea.ErrorResponse{Error: tea.ErrorObjectUnknown})
	})

	results, err := client.Discover(context.Background(), "urn:tei:unknown")
	if !IsNotFound(err) {
		t.Fatalf("err = %v, want IsNotFound", err)
	}
	if results != nil {
		t.Fatalf("results = %+v, want nil", results)
	}
}

// TestDiscoverByPURL confirms the purl query parameter is sent correctly
// and a match decodes the same as Discover's -- added alongside tei in
// upstream TEA 1.0 (spec/openapi.yaml).
func TestDiscoverByPURL(t *testing.T) {
	client, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("purl"); got != "pkg:generic/acme-widget@1.0.0" {
			t.Errorf("purl = %q", got)
		}
		if r.URL.Query().Get("tei") != "" {
			t.Errorf("tei query parameter should not be set for a purl request")
		}
		_ = json.NewEncoder(w).Encode([]tea.DiscoveryInfo{{ProductReleaseUUID: "pr-1"}})
	})

	results, err := client.DiscoverByPURL(context.Background(), "pkg:generic/acme-widget@1.0.0")
	if err != nil {
		t.Fatalf("DiscoverByPURL: %v", err)
	}
	if len(results) != 1 || results[0].ProductReleaseUUID != "pr-1" {
		t.Fatalf("results = %+v", results)
	}
}

// TestDiscoverByPURLNoMatchIsNotFound mirrors
// TestDiscoverNoMatchIsNotFound for the purl path.
func TestDiscoverByPURLNoMatchIsNotFound(t *testing.T) {
	client, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(tea.ErrorResponse{Error: tea.ErrorObjectUnknown})
	})

	results, err := client.DiscoverByPURL(context.Background(), "pkg:generic/unknown")
	if !IsNotFound(err) {
		t.Fatalf("err = %v, want IsNotFound", err)
	}
	if results != nil {
		t.Fatalf("results = %+v, want nil", results)
	}
}

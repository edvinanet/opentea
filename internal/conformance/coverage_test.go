// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package conformance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oej/opentea/pkg/teaclient"
)

func TestCoverageTrackerRecordsExercisedOperations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"uuid":"x","name":"n","identifiers":[]}`))
	}))
	t.Cleanup(srv.Close)

	baseURL := srv.URL + "/tea/v1"
	tracker, err := NewCoverageTracker(baseURL)
	if err != nil {
		t.Fatalf("NewCoverageTracker: %v", err)
	}
	client := teaclient.NewClient(baseURL, teaclient.WithHTTPClient(&http.Client{
		Transport: tracker.Transport(nil),
	}))

	if _, err := client.GetProduct(context.Background(), "abc-123"); err != nil {
		t.Fatalf("GetProduct: %v", err)
	}

	report := tracker.Report()
	if report.Total != len(Operations) {
		t.Fatalf("report.Total = %d, want %d", report.Total, len(Operations))
	}
	if report.Exercised != 1 {
		t.Fatalf("report.Exercised = %d, want 1", report.Exercised)
	}
	if len(report.NeverCalled) != len(Operations)-1 {
		t.Fatalf("len(NeverCalled) = %d, want %d", len(report.NeverCalled), len(Operations)-1)
	}
	for _, key := range report.NeverCalled {
		if key == "GET /product/{uuid}" {
			t.Fatal("getProduct appears in NeverCalled despite being exercised")
		}
	}
}

func TestCoverageTrackerStripsBasePath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"hasNext":false,"results":[]}`))
	}))
	t.Cleanup(srv.Close)

	baseURL := srv.URL + "/some/custom/mount"
	tracker, err := NewCoverageTracker(baseURL)
	if err != nil {
		t.Fatalf("NewCoverageTracker: %v", err)
	}
	client := teaclient.NewClient(baseURL, teaclient.WithHTTPClient(&http.Client{
		Transport: tracker.Transport(nil),
	}))

	if _, err := client.QueryProducts(context.Background(), teaclient.ListParams{}); err != nil {
		t.Fatalf("QueryProducts: %v", err)
	}
	if gotPath != "/some/custom/mount/products" {
		t.Fatalf("server saw path %q", gotPath)
	}

	report := tracker.Report()
	if report.Exercised != 1 {
		t.Fatalf("report.Exercised = %d, want 1 (basePath stripping should still let this match)", report.Exercised)
	}
}

func TestCoverageTrackerIgnoresUnregisteredRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("external content"))
	}))
	t.Cleanup(srv.Close)

	tracker, err := NewCoverageTracker("http://example.invalid/tea/v1")
	if err != nil {
		t.Fatalf("NewCoverageTracker: %v", err)
	}
	hc := &http.Client{Transport: tracker.Transport(nil)}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/some/external/location", nil)
	resp, err := hc.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp.Body.Close()

	report := tracker.Report()
	if report.Exercised != 0 {
		t.Fatalf("report.Exercised = %d, want 0 for a request to an unregistered path", report.Exercised)
	}
}

func TestCoverageReportPercent(t *testing.T) {
	r := CoverageReport{Exercised: 7, Total: 28}
	got := r.Percent()
	want := 100 * 7.0 / 28.0
	if got != want {
		t.Fatalf("Percent() = %v, want %v", got, want)
	}
	if (CoverageReport{}).Percent() != 0 {
		t.Fatal("Percent() on a zero-Total report should be 0, not NaN/panic")
	}
}

// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teaclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"

	"github.com/oej/opentea/pkg/tea"
)

func TestDownloadLatestAndVerifyToSelfHostedUsesLatestEndpoint(t *testing.T) {
	content := []byte("hello, latest tea client")
	sum := sha256.Sum256(content)
	hexSum := hex.EncodeToString(sum[:])

	var gotPath, gotQuery string
	client, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_, _ = w.Write(content)
	})

	format := tea.ArtifactFormat{
		MediaType: "application/vnd.cyclonedx+json",
		Checksums: []tea.Checksum{{AlgType: tea.ChecksumTypeSHA256, AlgValue: hexSum}},
	}
	var dst bytes.Buffer
	if err := client.DownloadLatestAndVerifyTo(context.Background(), "artifact-uuid-1", format, &dst); err != nil {
		t.Fatalf("DownloadLatestAndVerifyTo: %v", err)
	}
	if dst.String() != string(content) {
		t.Fatalf("got %q, want %q", dst.String(), content)
	}
	if gotPath != "/artifact/artifact-uuid-1/latest/download" {
		t.Fatalf("path = %q, want the latest download endpoint", gotPath)
	}
	if gotQuery != "mediaType=application%2Fvnd.cyclonedx%2Bjson" {
		t.Fatalf("query = %q", gotQuery)
	}
}

func TestDownloadSignatureSelfHosted(t *testing.T) {
	sig := []byte("deterministic-test-signature-bytes")

	var gotPath, gotQuery, gotAuth string
	client, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write(sig)
	})
	client.bearerToken = "sig-token"

	format := tea.ArtifactFormat{MediaType: "application/vnd.cyclonedx+json"}
	got, err := client.DownloadSignature(context.Background(), "artifact-uuid-1", 3, format)
	if err != nil {
		t.Fatalf("DownloadSignature: %v", err)
	}
	if string(got) != string(sig) {
		t.Fatalf("got %q, want %q", got, sig)
	}
	if gotPath != "/artifact/artifact-uuid-1/3/signature/download" {
		t.Fatalf("path = %q, want the versioned signature download endpoint", gotPath)
	}
	if gotQuery != "mediaType=application%2Fvnd.cyclonedx%2Bjson" {
		t.Fatalf("query = %q", gotQuery)
	}
	if gotAuth != "Bearer sig-token" {
		t.Fatalf("Authorization = %q, want the client's bearer token sent to this server's own endpoint", gotAuth)
	}
}

func TestDownloadSignatureExternalURL(t *testing.T) {
	sig := []byte("external-signature-bytes")
	var gotAuth string
	_, srv := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write(sig)
	})

	client := NewClient("http://unused.invalid", WithBearerToken("should-not-be-sent"))
	format := tea.ArtifactFormat{MediaType: "application/pdf", SignatureURL: srv.URL + "/sigs/whatever"}
	got, err := client.DownloadSignature(context.Background(), "artifact-uuid-1", 1, format)
	if err != nil {
		t.Fatalf("DownloadSignature: %v", err)
	}
	if string(got) != string(sig) {
		t.Fatalf("got %q, want %q", got, sig)
	}
	if gotAuth != "" {
		t.Fatalf("Authorization = %q, want empty for an external signatureUrl", gotAuth)
	}
}

func TestDownloadLatestSignatureUsesLatestEndpoint(t *testing.T) {
	sig := []byte("latest-signature-bytes")
	var gotPath string
	client, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write(sig)
	})

	format := tea.ArtifactFormat{MediaType: "application/vnd.cyclonedx+json"}
	got, err := client.DownloadLatestSignature(context.Background(), "artifact-uuid-1", format)
	if err != nil {
		t.Fatalf("DownloadLatestSignature: %v", err)
	}
	if string(got) != string(sig) {
		t.Fatalf("got %q, want %q", got, sig)
	}
	if gotPath != "/artifact/artifact-uuid-1/latest/signature/download" {
		t.Fatalf("path = %q, want the latest signature download endpoint", gotPath)
	}
}

func TestDownloadSignatureNotFoundReportsTEAErrorCode(t *testing.T) {
	client, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"SIGNATURE_NOT_FOUND"}`))
	})

	format := tea.ArtifactFormat{MediaType: "application/vnd.cyclonedx+json"}
	_, err := client.DownloadSignature(context.Background(), "artifact-uuid-1", 1, format)
	if err == nil {
		t.Fatal("expected an error for a 404 response")
	}
	if !IsNotFound(err) {
		t.Fatalf("IsNotFound(%v) = false, want true", err)
	}
	code, ok := TEAErrorCode(err)
	if !ok || code != "SIGNATURE_NOT_FOUND" {
		t.Fatalf("TEAErrorCode = %q, %v, want %q, true", code, ok, "SIGNATURE_NOT_FOUND")
	}
}

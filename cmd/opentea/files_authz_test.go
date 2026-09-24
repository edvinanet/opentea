// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/pkg/tea"
)

// TestFilesEndpointEnforcesArtifactDownloadAuthz is the regression test for
// the finding that /files/{sha256} served any stored blob to anyone who
// knew its hash, with no authorization check at all -- bypassing the
// artifact.download capability the versioned download endpoints
// (/tea/v1/artifact/.../download) already enforce. Uploads through the
// real admin HTTP flow (proving the happy path still works end to end,
// which no other test exercises for /files/ specifically), then restricts
// this one artifact's artifact.download capability and confirms
// /files/{sha256} for its content blob honors that restriction exactly
// like the versioned endpoint does: 401 anonymous (no token presented,
// spec's own 401-unauthorized text), 404 for an authenticated-but-
// unauthorized caller (existence-hiding, spec Sec 18).
func TestFilesEndpointEnforcesArtifactDownloadAuthz(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	status, raw := jsonRequest(t, srv, http.MethodPost, "/admin/v1/artifacts", map[string]any{
		"type":        "BOM",
		"createdDate": "2026-07-01T00:00:00Z",
		"formats":     []map[string]any{{"mediaType": "application/vnd.cyclonedx+json"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("create artifact: status=%d body=%s", status, raw)
	}
	var artifact tea.Artifact
	decodeInto(t, raw, &artifact)

	content := []byte(`{"bomFormat":"CycloneDX"}`)
	status, raw = uploadFile(t, srv, "/admin/v1/artifacts/"+artifact.UUID+"/1/files", "sbom.json", content, "application/vnd.cyclonedx+json")
	if status != http.StatusOK {
		t.Fatalf("upload artifact file: status=%d body=%s", status, raw)
	}

	// Read the format's checksum back through the real consumer metadata
	// endpoint (still governed by the still-unrestricted bootstrap
	// entitlement at this point) -- the artifact response never carries a
	// /files/{sha256} URL for self-hosted content itself (that link is
	// deliberately not published as the spec's own "url" field, see
	// internal/admin/upload.go), so a real client would resolve the hash
	// this same way.
	sha256Hex := findArtifactFormatSHA256(t, srv, artifact.UUID, "application/vnd.cyclonedx+json")

	// Before restriction: the migration-seeded permissive bootstrap
	// entitlement allows anonymous access, same as the versioned download
	// endpoint would.
	status, _, raw = teaRequest(t, srv, http.MethodGet, "/files/"+sha256Hex, "")
	if status != http.StatusOK {
		t.Fatalf("GET /files/%s before restriction: status=%d body=%s, want 200", sha256Hex, status, raw)
	}
	if string(raw) != string(content) {
		t.Fatalf("body = %q, want %q", raw, content)
	}

	createTemplateAndEntitlement(t, srv,
		[]map[string]any{{"capability": "artifact.download", "decision": "deny"}},
		"everyone", "", "artifact", artifact.UUID,
	)

	status, headers, raw := teaRequest(t, srv, http.MethodGet, "/files/"+sha256Hex, "")
	if status != http.StatusUnauthorized {
		t.Fatalf("GET /files/%s (anonymous, after restriction): status=%d body=%s, want 401", sha256Hex, status, raw)
	}
	if got := headers.Get("WWW-Authenticate"); got != `Bearer realm="tea"` {
		t.Fatalf("GET /files/%s (anonymous): WWW-Authenticate = %q, want a bare Bearer challenge (no error param -- no token was presented to be invalid)", sha256Hex, got)
	}

	consumer, err := srv.repo.CreateUser(ctx, "consumer", "password123", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	consumerToken, _, err := srv.repo.CreateAccessToken(ctx, consumer.UUID, time.Hour)
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}
	status, _, raw = teaRequest(t, srv, http.MethodGet, "/files/"+sha256Hex, consumerToken)
	if status != http.StatusNotFound {
		t.Fatalf("GET /files/%s (authenticated, unauthorized): status=%d body=%s, want 404 (denied, but a valid token was presented)", sha256Hex, status, raw)
	}
}

// findArtifactFormatSHA256 fetches an artifact's metadata through the
// consumer API and returns the SHA-256 checksum of the format matching
// mediaType.
func findArtifactFormatSHA256(t *testing.T, srv *testServer, artifactUUID, mediaType string) string {
	t.Helper()
	status, _, raw := teaRequest(t, srv, http.MethodGet, "/tea/v1/artifact/"+artifactUUID+"/1", "")
	if status != http.StatusOK {
		t.Fatalf("GET /tea/v1/artifact/%s/1: status=%d body=%s", artifactUUID, status, raw)
	}
	var a tea.Artifact
	decodeInto(t, raw, &a)
	for _, f := range a.Formats {
		if f.MediaType != mediaType {
			continue
		}
		for _, c := range f.Checksums {
			if c.AlgType == "SHA-256" {
				return c.AlgValue
			}
		}
	}
	t.Fatalf("no SHA-256 checksum found for artifact %s format %s", artifactUUID, mediaType)
	return ""
}

// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package webadmin

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/oej/opentea/internal/authn"
	"github.com/oej/opentea/internal/config"
	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/repo"
)

// getAuthenticated GETs path as adminToken and returns the status and body.
func getAuthenticated(t *testing.T, srvURL, path, adminToken string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srvURL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: authn.SessionCookieName, Value: adminToken})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status=%d body=%s", path, resp.StatusCode, body)
	}
	return resp.StatusCode, string(body)
}

// TestNavBarShowsTrustArchitectureMode confirms the deployment-level
// "Default TEA"/"Trusted TEA" badge (config.Config.TrustArchitectureEnabled,
// rendered in layout.html's nav on every page) reflects the server's
// config, in both directions.
func TestNavBarShowsTrustArchitectureMode(t *testing.T) {
	ctx := context.Background()

	for _, tt := range []struct {
		name    string
		enabled bool
		want    string
		wantNot string
	}{
		{"disabled", false, "Default TEA", "Trusted TEA"},
		{"enabled", true, "Trusted TEA", "Default TEA"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv, r := newTestServerWithConfig(t, config.Config{RootURL: "http://example.test", APIBasePath: "/tea/v1", TrustArchitectureEnabled: tt.enabled})

			admin, err := r.CreateUser(ctx, "admin", "adminpass1", model.RoleAdmin)
			if err != nil {
				t.Fatalf("CreateUser: %v", err)
			}
			token, _, err := r.CreateSession(ctx, admin.UUID, authn.SessionTTL)
			if err != nil {
				t.Fatalf("CreateSession: %v", err)
			}

			_, body := getAuthenticated(t, srv.URL, "/admin/ui/", token)
			if !strings.Contains(body, tt.want) {
				t.Errorf("response body doesn't contain %q; body=%s", tt.want, body)
			}
			if strings.Contains(body, tt.wantNot) {
				t.Errorf("response body unexpectedly contains %q; body=%s", tt.wantNot, body)
			}
		})
	}
}

// TestComponentReleaseDetailPageShowsEvidenceBadge confirms an artifact
// with an attached (draft, per Phase 1) evidence bundle renders a "Draft"
// badge on its release detail page, while a sibling artifact in the same
// collection with no evidence renders no badge at all (a plain em dash).
func TestComponentReleaseDetailPageShowsEvidenceBadge(t *testing.T) {
	ctx := context.Background()
	srv, r := newTestServerWithConfig(t, config.Config{RootURL: "http://example.test", APIBasePath: "/tea/v1"})

	component, err := r.CreateComponent(ctx, "libfoo", nil)
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	componentRelease, err := r.CreateComponentRelease(ctx, component.UUID, repo.ComponentReleaseInput{
		Version: "1.0.0", CreatedDate: time.Now().UTC().Truncate(time.Second),
	})
	if err != nil {
		t.Fatalf("CreateComponentRelease: %v", err)
	}

	signedArtifact, err := r.CreateArtifact(ctx, repo.ArtifactInput{
		Type: "BOM", Formats: []repo.ArtifactFormatInput{{MediaType: "application/json"}},
	})
	if err != nil {
		t.Fatalf("CreateArtifact (signed): %v", err)
	}
	unsignedArtifact, err := r.CreateArtifact(ctx, repo.ArtifactInput{
		Type: "BOM", Formats: []repo.ArtifactFormatInput{{MediaType: "application/json"}},
	})
	if err != nil {
		t.Fatalf("CreateArtifact (unsigned): %v", err)
	}

	if _, err := r.CreateCollectionForComponentRelease(ctx, componentRelease.UUID, repo.CollectionInput{
		Artifacts: []repo.ArtifactRef{
			{UUID: signedArtifact.UUID, Version: signedArtifact.Version},
			{UUID: unsignedArtifact.UUID, Version: unsignedArtifact.Version},
		},
	}); err != nil {
		t.Fatalf("CreateCollectionForComponentRelease: %v", err)
	}

	if _, err := r.CreateEvidenceBundle(ctx, repo.EvidenceBundleInput{
		OwnerType: "ARTIFACT", OwnerUUID: signedArtifact.UUID, OwnerVersion: signedArtifact.Version,
		ObjectType: "artifact", ObjectDigestValue: "deadbeef",
		SignatureFormat: "jws-detached", SignatureValue: "c2lnbmF0dXJl",
		CertificateFormat: "x509-pem", CertificateValue: "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
		CertificateFingerprint: "fp-webadmin-test", CertificateTrustDomain: "trust.example.com",
	}); err != nil {
		t.Fatalf("CreateEvidenceBundle: %v", err)
	}

	admin, err := r.CreateUser(ctx, "admin", "adminpass1", model.RoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, _, err := r.CreateSession(ctx, admin.UUID, authn.SessionTTL)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	_, body := getAuthenticated(t, srv.URL, "/admin/ui/componentReleases/"+componentRelease.UUID, token)

	if !strings.Contains(body, `class="badge badge-evidence-draft"`) || !strings.Contains(body, "Draft") {
		t.Errorf("response body doesn't show a draft evidence badge; body=%s", body)
	}
	if strings.Count(body, `class="badge badge-evidence-draft"`) != 1 {
		t.Errorf("expected exactly one draft evidence badge (only the signed artifact), got %d; body=%s", strings.Count(body, `class="badge badge-evidence-draft"`), body)
	}
	if strings.Contains(body, `class="badge badge-evidence-complete"`) {
		t.Errorf("response body unexpectedly shows a complete evidence badge; body=%s", body)
	}
}

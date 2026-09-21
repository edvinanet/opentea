// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/oej/opentea/internal/admin"
	"github.com/oej/opentea/internal/api"
	"github.com/oej/opentea/internal/config"
	"github.com/oej/opentea/internal/db"
	"github.com/oej/opentea/internal/files"
	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/internal/storage"
	"github.com/oej/opentea/internal/webadmin"
	"github.com/oej/opentea/pkg/teaclient"
)

// newTestServer builds a real in-process opentea server, including the
// admin GUI (needed since Login authenticates via POST /admin/ui/login,
// exactly like a human operator would).
func newTestServer(t *testing.T) (*httptest.Server, *repo.Repo) {
	t.Helper()
	dir := t.TempDir()

	sqlDB, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	blobStore, err := storage.NewFSStorage(filepath.Join(dir, "blobs"))
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}

	r := repo.New(sqlDB)

	cfg := config.Config{Versions: []string{"0.4.0"}, APIBasePath: "/tea/v1"}
	srv := httptest.NewServer(nil)
	cfg.RootURL = srv.URL

	mux := http.NewServeMux()
	mux.Handle(cfg.APIBasePath+"/", api.NewRouter(r, blobStore, cfg))
	mux.Handle("/admin/v1/", admin.NewRouter(r, blobStore, cfg, time.Now()))
	mux.Handle("/admin/ui/", webadmin.NewRouter(r, cfg))
	mux.Handle("/files/", files.NewHandler(r, blobStore))
	srv.Config.Handler = mux
	t.Cleanup(srv.Close)

	if _, err := r.CreateUser(context.Background(), "fixtures-test-admin", "fixtures-test-password", model.RoleAdmin); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	return srv, r
}

func loadFixture(t *testing.T, srv *httptest.Server, path string) {
	t.Helper()
	runner, err := Login(srv.URL, "fixtures-test-admin", "fixtures-test-password")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	runner.fixtureDir = filepath.Dir(path)

	data, err := loadFixtureFile(path)
	if err != nil {
		t.Fatalf("load fixture %s: %v", path, err)
	}
	if err := runner.Run(data); err != nil {
		t.Fatalf("replay fixture %s: %v", path, err)
	}
}

// TestLoadReferenceFixtures loads both reference fixture files into a real
// server, then reads the result back through pkg/teaclient -- fixtures,
// server, and client all exercised together.
func TestLoadReferenceFixtures(t *testing.T) {
	srv, _ := newTestServer(t)

	loadFixture(t, srv, "../../testdata/fixtures/log4j-tomcat.json")
	loadFixture(t, srv, "../../testdata/fixtures/edge-cases.json")

	ctx := context.Background()
	client := teaclient.NewClient(srv.URL + "/tea/v1")

	products, err := client.QueryProducts(ctx, teaclient.ListParams{PageSize: 100})
	if err != nil {
		t.Fatalf("QueryProducts: %v", err)
	}
	names := map[string]bool{}
	for _, p := range products.Results {
		names[p.Name] = true
	}
	if !names["Apache Log4j 2"] || !names["Acme Gateway"] {
		t.Fatalf("products = %+v, want Apache Log4j 2 and Acme Gateway present", products.Results)
	}

	componentReleases, err := client.QueryComponentReleases(ctx, teaclient.ListParams{PageSize: 100})
	if err != nil {
		t.Fatalf("QueryComponentReleases: %v", err)
	}
	var tomcatRelease, gatewayRelease string
	for _, cr := range componentReleases.Results {
		switch cr.ComponentName {
		case "Apache Tomcat":
			tomcatRelease = cr.UUID
		case "acme-gateway-core":
			gatewayRelease = cr.UUID
		}
	}
	if tomcatRelease == "" || gatewayRelease == "" {
		t.Fatalf("componentReleases = %+v, want Apache Tomcat and acme-gateway-core present", componentReleases.Results)
	}

	// Tomcat: distributions + a checksum-verifiable SBOM.
	withCollection, err := client.GetComponentReleaseWithCollection(ctx, tomcatRelease)
	if err != nil {
		t.Fatalf("GetComponentReleaseWithCollection(tomcat): %v", err)
	}
	if len(withCollection.Release.Distributions) != 2 {
		t.Fatalf("tomcat distributions = %+v, want 2", withCollection.Release.Distributions)
	}
	if len(withCollection.LatestCollection.Artifacts) != 1 {
		t.Fatalf("tomcat collection artifacts = %+v, want 1", withCollection.LatestCollection.Artifacts)
	}
	tomcatArtifact := withCollection.LatestCollection.Artifacts[0]
	if _, err := client.DownloadAndVerify(ctx, tomcatArtifact.UUID, tomcatArtifact.Version, tomcatArtifact.Formats[0]); err != nil {
		t.Fatalf("DownloadAndVerify(tomcat sbom): %v", err)
	}

	// Acme Gateway edge case: preRelease, 3 collection versions, multi-format artifact.
	gatewayWithCollection, err := client.GetComponentReleaseWithCollection(ctx, gatewayRelease)
	if err != nil {
		t.Fatalf("GetComponentReleaseWithCollection(gateway): %v", err)
	}
	if gatewayWithCollection.Release.PreRelease == nil || !*gatewayWithCollection.Release.PreRelease {
		t.Fatalf("gateway release PreRelease = %v, want true", gatewayWithCollection.Release.PreRelease)
	}
	if gatewayWithCollection.LatestCollection.Version != 3 {
		t.Fatalf("gateway latest collection version = %d, want 3", gatewayWithCollection.LatestCollection.Version)
	}
	if len(gatewayWithCollection.LatestCollection.Artifacts[0].Formats) != 2 {
		t.Fatalf("gateway artifact formats = %+v, want 2", gatewayWithCollection.LatestCollection.Artifacts[0].Formats)
	}
	gatewayArtifact := gatewayWithCollection.LatestCollection.Artifacts[0]
	for i, format := range gatewayArtifact.Formats {
		if _, err := client.DownloadAndVerify(ctx, gatewayArtifact.UUID, gatewayArtifact.Version, format); err != nil {
			t.Fatalf("DownloadAndVerify(gateway sbom format %d): %v", i, err)
		}
	}

	collections, err := client.ListCollectionsForComponentRelease(ctx, gatewayRelease, teaclient.ListParams{PageSize: 100})
	if err != nil {
		t.Fatalf("ListCollectionsForComponentRelease: %v", err)
	}
	if len(collections.Results) != 3 {
		t.Fatalf("gateway collection versions = %+v, want 3", collections.Results)
	}

	// CLE lifecycle: released, endOfSupport, supersededBy, withdrawn -- 4
	// events, newest (withdrawn, id 4) first.
	productReleases, err := client.QueryProductReleases(ctx, teaclient.ListParams{PageSize: 100, IDType: "TEI", IDValue: "urn:tei:uuid:acme.example.com:gateway-3.0.0-beta.1"})
	if err != nil {
		t.Fatalf("QueryProductReleases: %v", err)
	}
	if len(productReleases.Results) != 1 {
		t.Fatalf("productReleases (by TEI) = %+v, want exactly 1", productReleases.Results)
	}
	gatewayProductRelease := productReleases.Results[0]

	cle, err := client.GetCLEByProductRelease(ctx, gatewayProductRelease.UUID)
	if err != nil {
		t.Fatalf("GetCLEByProductRelease: %v", err)
	}
	if len(cle.Events) != 4 {
		t.Fatalf("cle.Events = %+v, want 4", cle.Events)
	}
	if cle.Events[0].Type != "withdrawn" || cle.Events[0].EventID == nil || *cle.Events[0].EventID != 2 {
		t.Fatalf("cle.Events[0] (newest) = %+v, want withdrawn referencing eventId 2", cle.Events[0])
	}
	if cle.Definitions == nil || len(cle.Definitions.Support) != 1 || cle.Definitions.Support[0].ID != "beta" {
		t.Fatalf("cle.Definitions = %+v, want one 'beta' support definition", cle.Definitions)
	}

	// Discovery: the Log4j TEI resolves to its product release.
	discovery, err := client.Discover(ctx, "urn:tei:uuid:apache.org:log4j-2.24.3")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(discovery) != 1 {
		t.Fatalf("discovery = %+v, want 1 result", discovery)
	}
}

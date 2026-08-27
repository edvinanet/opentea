package main

import (
	"bytes"
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
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/internal/storage"
	"github.com/oej/opentea/pkg/tea"
	"github.com/oej/opentea/pkg/teaclient"
)

// newTestServer builds a real in-process opentea server (the same wiring as
// cmd/opentea's newMux) so pkg/teaclient can be exercised against a real
// server, not mocks.
func newTestServer(t *testing.T) (*httptest.Server, *repo.Repo, storage.Storage) {
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
	mux.Handle(cfg.APIBasePath+"/", api.NewRouter(r, cfg))
	mux.Handle("/admin/v1/", admin.NewRouter(r, blobStore, cfg, time.Now()))
	mux.Handle("/files/", files.NewHandler(r, blobStore))
	srv.Config.Handler = mux
	t.Cleanup(srv.Close)

	return srv, r, blobStore
}

// TestClientAgainstRealServer seeds data directly via the repo layer (the
// server side of this test, mirroring how an admin would populate it) and
// then reads it all back through pkg/teaclient (the client side) -- a real
// client against a real server, exercising the full HTTP round trip for
// every kind of read.
func TestClientAgainstRealServer(t *testing.T) {
	ctx := context.Background()
	srv, r, blobStore := newTestServer(t)
	client := teaclient.NewClient(srv.URL + "/tea/v1")

	product, err := r.CreateProduct(ctx, "Acme Widget", []tea.Identifier{{IDType: tea.IdentifierTypePURL, IDValue: "pkg:generic/acme-widget"}})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	productRelease, err := r.CreateProductRelease(ctx, product.UUID, repo.ProductReleaseInput{
		Version:     "1.0.0",
		CreatedDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		Identifiers: []tea.Identifier{{IDType: tea.IdentifierTypeTEI, IDValue: "urn:tei:uuid:acme.example.com:widget-1.0.0"}},
	})
	if err != nil {
		t.Fatalf("CreateProductRelease: %v", err)
	}
	component, err := r.CreateComponent(ctx, "acme-widget-core", nil)
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	componentRelease, err := r.CreateComponentRelease(ctx, component.UUID, repo.ComponentReleaseInput{
		Version:     "1.0.0",
		CreatedDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateComponentRelease: %v", err)
	}
	artifact, err := r.CreateArtifact(ctx, repo.ArtifactInput{
		Name: "sbom.json", Type: tea.ArtifactTypeBOM,
		Formats: []repo.ArtifactFormatInput{{MediaType: "application/vnd.cyclonedx+json"}},
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	sbomContent := []byte(`{"bomFormat":"CycloneDX"}`)
	sha256Hex, _, err := blobStore.Put(ctx, bytes.NewReader(sbomContent))
	if err != nil {
		t.Fatalf("blob Put: %v", err)
	}
	if err := r.UpsertBlob(ctx, sha256Hex, int64(len(sbomContent)), "application/vnd.cyclonedx+json"); err != nil {
		t.Fatalf("UpsertBlob: %v", err)
	}
	artifact, err = r.SetArtifactFormatFile(ctx, artifact.UUID, artifact.Version, 0, srv.URL+"/files/"+sha256Hex, sha256Hex)
	if err != nil {
		t.Fatalf("SetArtifactFormatFile: %v", err)
	}
	if _, err := r.CreateCollectionForComponentRelease(ctx, componentRelease.UUID, repo.CollectionInput{
		Artifacts: []repo.ArtifactRef{{UUID: artifact.UUID, Version: artifact.Version}},
	}); err != nil {
		t.Fatalf("CreateCollectionForComponentRelease: %v", err)
	}
	if _, err := r.LinkComponent(ctx, productRelease.UUID, tea.ComponentRef{UUID: component.UUID, Release: &componentRelease.UUID}); err != nil {
		t.Fatalf("LinkComponent: %v", err)
	}
	if _, err := r.CreateCLEEvent(ctx, repo.OwnerProductRelease, productRelease.UUID, repo.CLEEventInput{
		Type: tea.CLEEventTypeReleased, Effective: time.Now(), Published: time.Now(), Version: "1.0.0",
	}); err != nil {
		t.Fatalf("CreateCLEEvent: %v", err)
	}

	// --- Now read all of it back through the client. ---

	gotProduct, err := client.GetProduct(ctx, product.UUID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if gotProduct.Name != "Acme Widget" {
		t.Fatalf("gotProduct = %+v", gotProduct)
	}

	products, err := client.QueryProducts(ctx, teaclient.ListParams{})
	if err != nil {
		t.Fatalf("QueryProducts: %v", err)
	}
	if len(products.Results) != 1 {
		t.Fatalf("QueryProducts.Results = %+v", products.Results)
	}

	gotRelease, err := client.GetProductRelease(ctx, productRelease.UUID)
	if err != nil {
		t.Fatalf("GetProductRelease: %v", err)
	}
	if len(gotRelease.Components) != 1 || gotRelease.Components[0].UUID != component.UUID {
		t.Fatalf("gotRelease.Components = %+v", gotRelease.Components)
	}

	withCollection, err := client.GetComponentReleaseWithCollection(ctx, componentRelease.UUID)
	if err != nil {
		t.Fatalf("GetComponentReleaseWithCollection: %v", err)
	}
	if len(withCollection.LatestCollection.Artifacts) != 1 {
		t.Fatalf("LatestCollection.Artifacts = %+v", withCollection.LatestCollection.Artifacts)
	}

	format := withCollection.LatestCollection.Artifacts[0].Formats[0]
	downloaded, err := client.DownloadAndVerify(ctx, format)
	if err != nil {
		t.Fatalf("DownloadAndVerify: %v", err)
	}
	if string(downloaded) != string(sbomContent) {
		t.Fatalf("downloaded = %q, want %q", downloaded, sbomContent)
	}

	cle, err := client.GetCLEByProductRelease(ctx, productRelease.UUID)
	if err != nil {
		t.Fatalf("GetCLEByProductRelease: %v", err)
	}
	if len(cle.Events) != 1 || cle.Events[0].Type != tea.CLEEventTypeReleased {
		t.Fatalf("cle.Events = %+v", cle.Events)
	}

	discovery, err := client.Discover(ctx, "urn:tei:uuid:acme.example.com:widget-1.0.0")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(discovery) != 1 || discovery[0].ProductReleaseUUID != productRelease.UUID {
		t.Fatalf("discovery = %+v", discovery)
	}

	// A nonexistent product is a real 404, mapped to teaclient.IsNotFound.
	if _, err := client.GetProduct(ctx, "00000000-0000-4000-8000-000000000000"); !teaclient.IsNotFound(err) {
		t.Fatalf("err = %v, want IsNotFound", err)
	}
}

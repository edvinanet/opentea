package bundle

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oej/opentea/internal/db"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/internal/storage"
	"github.com/oej/opentea/pkg/tea"
)

func newTestRepo(t *testing.T) *repo.Repo {
	t.Helper()
	sqlDB, err := openTestDB(t)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	return repo.New(sqlDB)
}

func newTestStore(t *testing.T) storage.Storage {
	t.Helper()
	store, err := storage.NewFSStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	return store
}

// seedProduct builds a minimal but representative product: one release
// linking one pinned component release, one distribution with a real
// stored blob, and one collection carrying one artifact whose format also
// has a real stored blob.
func seedProduct(t *testing.T, r *repo.Repo, store storage.Storage) (productUUID string) {
	t.Helper()
	ctx := context.Background()

	product, err := r.CreateProduct(ctx, "Test Product", []tea.Identifier{{IDType: "TEI", IDValue: "urn:tei:test"}})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	release, err := r.CreateProductRelease(ctx, product.UUID, repo.ProductReleaseInput{
		Version: "1.0.0", CreatedDate: time.Now().UTC().Truncate(time.Second),
	})
	if err != nil {
		t.Fatalf("CreateProductRelease: %v", err)
	}

	component, err := r.CreateComponent(ctx, "libfoo", []tea.Identifier{{IDType: "PURL", IDValue: "pkg:generic/libfoo"}})
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	componentRelease, err := r.CreateComponentRelease(ctx, component.UUID, repo.ComponentReleaseInput{
		Version: "9.9.9", CreatedDate: time.Now().UTC().Truncate(time.Second),
	})
	if err != nil {
		t.Fatalf("CreateComponentRelease: %v", err)
	}

	distSHA, _, err := store.Put(ctx, strings.NewReader("distribution bytes"))
	if err != nil {
		t.Fatalf("store.Put distribution: %v", err)
	}
	dist, err := r.CreateDistribution(ctx, componentRelease.UUID, "binary tarball")
	if err != nil {
		t.Fatalf("CreateDistribution: %v", err)
	}
	if _, err := r.SetDistributionFile(ctx, dist.DistributionID, "http://source.example/files/"+distSHA, distSHA); err != nil {
		t.Fatalf("SetDistributionFile: %v", err)
	}

	releaseUUID := &componentRelease.UUID
	if _, err := r.LinkComponent(ctx, release.UUID, tea.ComponentRef{UUID: component.UUID, Release: releaseUUID}); err != nil {
		t.Fatalf("LinkComponent: %v", err)
	}

	formatSHA, _, err := store.Put(ctx, strings.NewReader("artifact bytes"))
	if err != nil {
		t.Fatalf("store.Put artifact format: %v", err)
	}
	artifact, err := r.CreateArtifact(ctx, repo.ArtifactInput{
		Type: "BOM", Formats: []repo.ArtifactFormatInput{{MediaType: "application/json"}},
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	if _, err := r.SetArtifactFormatFile(ctx, artifact.UUID, artifact.Version, 0, "http://source.example/files/"+formatSHA, formatSHA); err != nil {
		t.Fatalf("SetArtifactFormatFile: %v", err)
	}
	if _, err := r.CreateCollectionForComponentRelease(ctx, componentRelease.UUID, repo.CollectionInput{
		Artifacts: []repo.ArtifactRef{{UUID: artifact.UUID, Version: artifact.Version}},
	}); err != nil {
		t.Fatalf("CreateCollectionForComponentRelease: %v", err)
	}

	if _, err := r.CreateCLEEvent(ctx, repo.OwnerProduct, product.UUID, repo.CLEEventInput{
		Type: "released", Effective: time.Now().UTC().Truncate(time.Second), Published: time.Now().UTC().Truncate(time.Second),
	}); err != nil {
		t.Fatalf("CreateCLEEvent: %v", err)
	}

	return product.UUID
}

func TestExportProducesValidZip(t *testing.T) {
	r := newTestRepo(t)
	store := newTestStore(t)
	productUUID := seedProduct(t, r, store)

	var buf bytes.Buffer
	if err := Export(context.Background(), r, store, productUUID, &buf); err != nil {
		t.Fatalf("Export: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	var names []string
	var manifestFile *zip.File
	for _, f := range zr.File {
		names = append(names, f.Name)
		if f.Name == "manifest.json" {
			manifestFile = f
		}
	}
	if manifestFile == nil {
		t.Fatalf("manifest.json not found in zip, entries: %v", names)
	}

	// Expect exactly two files/ entries: the distribution blob and the
	// artifact format blob.
	fileEntries := 0
	for _, n := range names {
		if strings.HasPrefix(n, "files/") {
			fileEntries++
		}
	}
	if fileEntries != 2 {
		t.Fatalf("files/ entries = %d, want 2, entries: %v", fileEntries, names)
	}

	rc, err := manifestFile.Open()
	if err != nil {
		t.Fatalf("open manifest.json: %v", err)
	}
	defer func() { _ = rc.Close() }()
	var m Manifest
	if err := json.NewDecoder(rc).Decode(&m); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}

	if m.FormatVersion != FormatVersion {
		t.Fatalf("FormatVersion = %q", m.FormatVersion)
	}
	if m.Product.UUID != productUUID {
		t.Fatalf("Product.UUID = %q, want %q", m.Product.UUID, productUUID)
	}
	if m.Product.CLE == nil || len(m.Product.CLE.Events) != 1 {
		t.Fatalf("Product.CLE = %+v, want exactly one event", m.Product.CLE)
	}
	if len(m.ProductReleases) != 1 {
		t.Fatalf("ProductReleases = %+v, want 1", m.ProductReleases)
	}
	if len(m.Components) != 1 {
		t.Fatalf("Components = %+v, want 1", m.Components)
	}
	if len(m.ComponentReleases) != 1 {
		t.Fatalf("ComponentReleases = %+v, want 1", m.ComponentReleases)
	}
	if len(m.ComponentReleases[0].Distributions) != 1 {
		t.Fatalf("ComponentReleases[0].Distributions = %+v, want 1", m.ComponentReleases[0].Distributions)
	}
	if len(m.Collections) != 1 {
		t.Fatalf("Collections = %+v, want 1", m.Collections)
	}
	if len(m.Collections[0].Artifacts) != 1 {
		t.Fatalf("Collections[0].Artifacts = %+v, want 1", m.Collections[0].Artifacts)
	}

	// The manifest itself must satisfy the JSON Schema.
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("re-marshal manifest: %v", err)
	}
	if err := ValidateManifest(raw); err != nil {
		t.Fatalf("exported manifest failed schema validation: %v", err)
	}
}

func TestExportUnpinnedComponentRefOmitsReleaseData(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	store := newTestStore(t)

	product, err := r.CreateProduct(ctx, "Test Product", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	release, err := r.CreateProductRelease(ctx, product.UUID, repo.ProductReleaseInput{
		Version: "1.0.0", CreatedDate: time.Now().UTC().Truncate(time.Second),
	})
	if err != nil {
		t.Fatalf("CreateProductRelease: %v", err)
	}
	component, err := r.CreateComponent(ctx, "libfoo", nil)
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	if _, err := r.CreateComponentRelease(ctx, component.UUID, repo.ComponentReleaseInput{
		Version: "1.0.0", CreatedDate: time.Now().UTC().Truncate(time.Second),
	}); err != nil {
		t.Fatalf("CreateComponentRelease: %v", err)
	}
	// Unpinned ref: no Release set.
	if _, err := r.LinkComponent(ctx, release.UUID, tea.ComponentRef{UUID: component.UUID}); err != nil {
		t.Fatalf("LinkComponent: %v", err)
	}

	var buf bytes.Buffer
	if err := Export(ctx, r, store, product.UUID, &buf); err != nil {
		t.Fatalf("Export: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	var m Manifest
	for _, f := range zr.File {
		if f.Name == "manifest.json" {
			rc, _ := f.Open()
			defer func() { _ = rc.Close() }()
			if err := json.NewDecoder(rc).Decode(&m); err != nil {
				t.Fatalf("decode manifest: %v", err)
			}
		}
	}

	if len(m.Components) != 1 {
		t.Fatalf("Components = %+v, want 1 (bare component only)", m.Components)
	}
	if len(m.ComponentReleases) != 0 {
		t.Fatalf("ComponentReleases = %+v, want 0 (unpinned ref must not pull in release history)", m.ComponentReleases)
	}
}

func openTestDB(t *testing.T) (*sql.DB, error) {
	t.Helper()
	return db.Open(filepath.Join(t.TempDir(), "test.db"))
}

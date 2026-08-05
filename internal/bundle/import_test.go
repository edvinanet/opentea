package bundle

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/pkg/tea"
)

func fixedTime() time.Time {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
}

func componentRef(componentUUID string, releaseUUID *string) tea.ComponentRef {
	return tea.ComponentRef{UUID: componentUUID, Release: releaseUUID}
}

// corruptFirstFileEntry rewrites the first files/<sha256> entry's content in
// a bundle zip (read fully into memory, since these are tiny test fixtures)
// so its bytes no longer match its claimed hash, without touching the
// manifest.
func corruptFirstFileEntry(t *testing.T, zipBytes []byte) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	corrupted := false
	for _, f := range zr.File {
		w, err := zw.Create(f.Name)
		if err != nil {
			t.Fatalf("zw.Create(%s): %v", f.Name, err)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		if !corrupted && strings.HasPrefix(f.Name, "files/") {
			if _, err := w.Write([]byte("corrupted content that will not match the hash")); err != nil {
				t.Fatalf("write corrupted %s: %v", f.Name, err)
			}
			corrupted = true
		} else {
			if _, err := io.Copy(w, rc); err != nil {
				t.Fatalf("copy %s: %v", f.Name, err)
			}
		}
		_ = rc.Close()
	}
	if !corrupted {
		t.Fatal("test bug: no files/ entry found to corrupt")
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return out.Bytes()
}

func TestImportRoundTrip(t *testing.T) {
	ctx := context.Background()
	srcRepo := newTestRepo(t)
	srcStore := newTestStore(t)
	productUUID := seedProduct(t, srcRepo, srcStore)

	var buf bytes.Buffer
	if err := Export(ctx, srcRepo, srcStore, productUUID, &buf); err != nil {
		t.Fatalf("Export: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	dstRepo := newTestRepo(t)
	dstStore := newTestStore(t)

	result, err := Import(ctx, dstRepo, dstStore, "http://dest.example", zr)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if !result.ProductCreated {
		t.Fatal("ProductCreated = false, want true on first import")
	}
	wantCreated := map[string]int{
		"product": 1, "productRelease": 1, "component": 1, "componentRelease": 1,
		"distribution": 1, "artifact": 1, "collection": 1, "cleEvent": 1,
	}
	for kind, want := range wantCreated {
		if got := result.Created[kind]; got != want {
			t.Errorf("Created[%q] = %d, want %d (full: %+v)", kind, got, want, result.Created)
		}
	}

	// Verify the imported data is actually readable and correct.
	product, err := dstRepo.GetProduct(ctx, productUUID)
	if err != nil {
		t.Fatalf("GetProduct after import: %v", err)
	}
	if product.Name != "Test Product" {
		t.Fatalf("Name = %q", product.Name)
	}

	releases, err := dstRepo.ListProductReleasesByProduct(ctx, productUUID, "", "asc", nil, 10)
	if err != nil {
		t.Fatalf("ListProductReleasesByProduct: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("releases = %+v, want 1", releases)
	}
	if len(releases[0].Components) != 1 {
		t.Fatalf("Components = %+v, want 1", releases[0].Components)
	}
	componentReleaseUUID := *releases[0].Components[0].Release

	componentRelease, err := dstRepo.GetComponentRelease(ctx, componentReleaseUUID)
	if err != nil {
		t.Fatalf("GetComponentRelease: %v", err)
	}
	if len(componentRelease.Distributions) != 1 {
		t.Fatalf("Distributions = %+v, want 1", componentRelease.Distributions)
	}
	dist := componentRelease.Distributions[0]
	if dist.URL == "" || dist.URL[:len("http://dest.example")] != "http://dest.example" {
		t.Fatalf("Distribution URL = %q, want rewritten against the destination server's root URL", dist.URL)
	}

	collections, err := dstRepo.ListCollections(ctx, componentReleaseUUID, "asc", nil, 10)
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(collections) != 1 || len(collections[0].Artifacts) != 1 {
		t.Fatalf("collections = %+v", collections)
	}

	// Re-importing the identical bundle must be a complete no-op.
	zr2, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader (2nd): %v", err)
	}
	result2, err := Import(ctx, dstRepo, dstStore, "http://dest.example", zr2)
	if err != nil {
		t.Fatalf("second Import: %v", err)
	}
	if result2.ProductCreated {
		t.Fatal("ProductCreated = true on second import, want false (idempotent)")
	}
	if len(result2.Created) != 0 {
		t.Fatalf("Created = %+v on second import, want nothing newly created", result2.Created)
	}
}

func TestImportDedupsSharedComponent(t *testing.T) {
	ctx := context.Background()
	srcRepo := newTestRepo(t)
	srcStore := newTestStore(t)

	// Two products sharing one component release.
	productA, err := srcRepo.CreateProduct(ctx, "Product A", nil)
	if err != nil {
		t.Fatalf("CreateProduct A: %v", err)
	}
	productB, err := srcRepo.CreateProduct(ctx, "Product B", nil)
	if err != nil {
		t.Fatalf("CreateProduct B: %v", err)
	}
	component, err := srcRepo.CreateComponent(ctx, "shared-lib", nil)
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	componentRelease, err := srcRepo.CreateComponentRelease(ctx, component.UUID, repo.ComponentReleaseInput{Version: "1.0.0", CreatedDate: fixedTime()})
	if err != nil {
		t.Fatalf("CreateComponentRelease: %v", err)
	}

	releaseA, err := srcRepo.CreateProductRelease(ctx, productA.UUID, repo.ProductReleaseInput{Version: "1.0.0", CreatedDate: fixedTime()})
	if err != nil {
		t.Fatalf("CreateProductRelease A: %v", err)
	}
	releaseB, err := srcRepo.CreateProductRelease(ctx, productB.UUID, repo.ProductReleaseInput{Version: "1.0.0", CreatedDate: fixedTime()})
	if err != nil {
		t.Fatalf("CreateProductRelease B: %v", err)
	}
	crUUID := componentRelease.UUID
	if _, err := srcRepo.LinkComponent(ctx, releaseA.UUID, componentRef(component.UUID, &crUUID)); err != nil {
		t.Fatalf("LinkComponent A: %v", err)
	}
	if _, err := srcRepo.LinkComponent(ctx, releaseB.UUID, componentRef(component.UUID, &crUUID)); err != nil {
		t.Fatalf("LinkComponent B: %v", err)
	}

	dstRepo := newTestRepo(t)
	dstStore := newTestStore(t)

	for _, productUUID := range []string{productA.UUID, productB.UUID} {
		var buf bytes.Buffer
		if err := Export(ctx, srcRepo, srcStore, productUUID, &buf); err != nil {
			t.Fatalf("Export %s: %v", productUUID, err)
		}
		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		if err != nil {
			t.Fatalf("zip.NewReader: %v", err)
		}
		if _, err := Import(ctx, dstRepo, dstStore, "http://dest.example", zr); err != nil {
			t.Fatalf("Import %s: %v", productUUID, err)
		}
	}

	// The shared component release must exist exactly once, not duplicated.
	got, err := dstRepo.GetComponentRelease(ctx, componentRelease.UUID)
	if err != nil {
		t.Fatalf("GetComponentRelease after both imports: %v", err)
	}
	if got.UUID != componentRelease.UUID {
		t.Fatalf("UUID = %q, want %q", got.UUID, componentRelease.UUID)
	}

	releases, err := dstRepo.ListComponentReleasesByComponent(ctx, component.UUID, "", "asc", nil, 10)
	if err != nil {
		t.Fatalf("ListComponentReleasesByComponent: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("component releases = %+v, want exactly 1 (shared, not duplicated)", releases)
	}
}

func TestImportRejectsCorruptBundle(t *testing.T) {
	ctx := context.Background()
	srcRepo := newTestRepo(t)
	srcStore := newTestStore(t)
	productUUID := seedProduct(t, srcRepo, srcStore)

	var buf bytes.Buffer
	if err := Export(ctx, srcRepo, srcStore, productUUID, &buf); err != nil {
		t.Fatalf("Export: %v", err)
	}

	corrupted := corruptFirstFileEntry(t, buf.Bytes())
	zr, err := zip.NewReader(bytes.NewReader(corrupted), int64(len(corrupted)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	dstRepo := newTestRepo(t)
	dstStore := newTestStore(t)
	if _, err := Import(ctx, dstRepo, dstStore, "http://dest.example", zr); err == nil {
		t.Fatal("expected Import to reject a bundle whose file content doesn't match its claimed hash")
	}
}

// oversizeFirstFileEntry rewrites the first files/<sha256> entry's content
// in a bundle zip to be n bytes, without touching the manifest -- used to
// exercise the maxZipEntrySize bound without needing a real multi-gigabyte
// fixture.
func oversizeFirstFileEntry(t *testing.T, zipBytes []byte, n int) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	resized := false
	for _, f := range zr.File {
		w, err := zw.Create(f.Name)
		if err != nil {
			t.Fatalf("zw.Create(%s): %v", f.Name, err)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		if !resized && strings.HasPrefix(f.Name, "files/") {
			if _, err := io.CopyN(w, zeroReader{}, int64(n)); err != nil {
				t.Fatalf("write oversized %s: %v", f.Name, err)
			}
			resized = true
		} else {
			if _, err := io.Copy(w, rc); err != nil {
				t.Fatalf("copy %s: %v", f.Name, err)
			}
		}
		_ = rc.Close()
	}
	if !resized {
		t.Fatal("test bug: no files/ entry found to resize")
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return out.Bytes()
}

// zeroReader is an infinite source of zero bytes, for cheaply generating
// large test content without allocating it all in memory up front.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func TestOversizedZipEntryIsRejected(t *testing.T) {
	ctx := context.Background()
	srcRepo := newTestRepo(t)
	srcStore := newTestStore(t)
	productUUID := seedProduct(t, srcRepo, srcStore)

	var buf bytes.Buffer
	if err := Export(ctx, srcRepo, srcStore, productUUID, &buf); err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Temporarily lower the bound so this test doesn't need a real
	// multi-gigabyte fixture -- this package's tests never run in
	// parallel, so mutating the shared var is safe as long as it's
	// restored before this test returns.
	original := maxZipEntrySize
	maxZipEntrySize = 1024
	t.Cleanup(func() { maxZipEntrySize = original })

	oversized := oversizeFirstFileEntry(t, buf.Bytes(), 2048)

	t.Run("Check", func(t *testing.T) {
		zr, err := zip.NewReader(bytes.NewReader(oversized), int64(len(oversized)))
		if err != nil {
			t.Fatalf("zip.NewReader: %v", err)
		}
		if _, err := Check(zr); err == nil {
			t.Fatal("expected Check to reject a zip entry exceeding maxZipEntrySize")
		}
	})

	t.Run("Import", func(t *testing.T) {
		zr, err := zip.NewReader(bytes.NewReader(oversized), int64(len(oversized)))
		if err != nil {
			t.Fatalf("zip.NewReader: %v", err)
		}
		dstRepo := newTestRepo(t)
		dstStore := newTestStore(t)
		if _, err := Import(ctx, dstRepo, dstStore, "http://dest.example", zr); err == nil {
			t.Fatal("expected Import to reject a zip entry exceeding maxZipEntrySize")
		}
	})
}

func TestImportRejectsBadManifest(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mw, err := zw.Create("manifest.json")
	if err != nil {
		t.Fatalf("create manifest.json: %v", err)
	}
	if _, err := mw.Write([]byte(`{"formatVersion":"1.0"}`)); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	r := newTestRepo(t)
	store := newTestStore(t)
	if _, err := Import(context.Background(), r, store, "http://dest.example", zr); err == nil {
		t.Fatal("expected Import to reject a manifest missing required fields")
	}
}

package bundle

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
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

	collections, err := dstRepo.ListCollections(ctx, componentReleaseUUID, "asc", nil, 10, repo.BelongsToComponentRelease)
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

// TestImportBumpsWatermarksOnce is the regression test for list-endpoint
// ETag support via bundle import: importing a bundle bumps every affected
// resource-family watermark (each entity's own Import* call bumps its own
// family on a genuine insert -- there's no separate bundle-import-level
// watermark step, so this proves that composition actually works end to
// end), and a second, idempotent re-import of the identical bundle must not
// bump any of them further.
func TestImportBumpsWatermarksOnce(t *testing.T) {
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
	if _, err := Import(ctx, dstRepo, dstStore, "http://dest.example", zr); err != nil {
		t.Fatalf("Import: %v", err)
	}

	families := []string{
		repo.WatermarkProducts, repo.WatermarkProductReleases, repo.WatermarkComponents,
		repo.WatermarkComponentReleases, repo.WatermarkCollections,
	}
	afterFirst := map[string]int64{}
	for _, f := range families {
		w, err := dstRepo.GetWatermark(ctx, f)
		if err != nil {
			t.Fatalf("GetWatermark(%q): %v", f, err)
		}
		if w == 0 {
			t.Errorf("watermark(%q) = 0 after import, want > 0", f)
		}
		afterFirst[f] = w
	}

	// Re-importing the identical bundle is an idempotent no-op (already
	// proven by TestImportRoundTrip) -- confirm watermarks don't move either.
	zr2, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader (2nd): %v", err)
	}
	if _, err := Import(ctx, dstRepo, dstStore, "http://dest.example", zr2); err != nil {
		t.Fatalf("second Import: %v", err)
	}
	for _, f := range families {
		w, err := dstRepo.GetWatermark(ctx, f)
		if err != nil {
			t.Fatalf("GetWatermark(%q) after re-import: %v", f, err)
		}
		if w != afterFirst[f] {
			t.Errorf("watermark(%q) after idempotent re-import = %d, want unchanged at %d", f, w, afterFirst[f])
		}
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

// TestZipResourceLimitsRejectOversizedManifest is the regression test for
// maxManifestSize: manifest.json used to be read via an unbounded
// io.ReadAll. Temporarily lowers maxManifestSize below a real exported
// manifest's actual size, rather than crafting an oversized manifest,
// since the limit itself is what's under test here, not any particular
// manifest content.
func TestZipResourceLimitsRejectOversizedManifest(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	store := newTestStore(t)
	productUUID := seedProduct(t, r, store)

	var buf bytes.Buffer
	if err := Export(ctx, r, store, productUUID, &buf); err != nil {
		t.Fatalf("Export: %v", err)
	}

	original := maxManifestSize
	maxManifestSize = 10 // far smaller than any real manifest
	t.Cleanup(func() { maxManifestSize = original })

	t.Run("Check", func(t *testing.T) {
		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		if err != nil {
			t.Fatalf("zip.NewReader: %v", err)
		}
		if _, err := Check(zr); err == nil {
			t.Fatal("expected Check to reject a manifest exceeding maxManifestSize")
		}
	})

	t.Run("Import", func(t *testing.T) {
		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		if err != nil {
			t.Fatalf("zip.NewReader: %v", err)
		}
		dstRepo := newTestRepo(t)
		dstStore := newTestStore(t)
		if _, err := Import(ctx, dstRepo, dstStore, "http://dest.example", zr); err == nil {
			t.Fatal("expected Import to reject a manifest exceeding maxManifestSize")
		}
	})
}

// TestZipResourceLimitsRejectExcessiveEntryCount is the regression test for
// maxZipEntries -- a zip crafted with many entries (well past any real
// bundle's handful of files) must be rejected before any entry is opened.
func TestZipResourceLimitsRejectExcessiveEntryCount(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i := 0; i < maxZipEntries+1; i++ {
		if _, err := zw.Create("files/entry-" + strconv.Itoa(i)); err != nil {
			t.Fatalf("zw.Create: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	t.Run("Check", func(t *testing.T) {
		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		if err != nil {
			t.Fatalf("zip.NewReader: %v", err)
		}
		if _, err := Check(zr); err == nil {
			t.Fatal("expected Check to reject a zip exceeding maxZipEntries")
		}
	})

	t.Run("Import", func(t *testing.T) {
		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		if err != nil {
			t.Fatalf("zip.NewReader: %v", err)
		}
		r := newTestRepo(t)
		store := newTestStore(t)
		if _, err := Import(context.Background(), r, store, "http://dest.example", zr); err == nil {
			t.Fatal("expected Import to reject a zip exceeding maxZipEntries")
		}
	})
}

// TestZipResourceLimitsRejectExcessiveAggregateSize is the regression test
// for maxZipTotalUncompressed -- temporarily lowers it well below a small
// real entry's size, rather than needing a multi-gigabyte fixture to
// exceed the real 4 GiB default.
func TestZipResourceLimitsRejectExcessiveAggregateSize(t *testing.T) {
	original := maxZipTotalUncompressed
	maxZipTotalUncompressed = 100
	t.Cleanup(func() { maxZipTotalUncompressed = original })

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("files/dummy")
	if err != nil {
		t.Fatalf("zw.Create: %v", err)
	}
	if _, err := w.Write(make([]byte, 200)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	t.Run("Check", func(t *testing.T) {
		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		if err != nil {
			t.Fatalf("zip.NewReader: %v", err)
		}
		if _, err := Check(zr); err == nil {
			t.Fatal("expected Check to reject a zip exceeding maxZipTotalUncompressed")
		}
	})

	t.Run("Import", func(t *testing.T) {
		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		if err != nil {
			t.Fatalf("zip.NewReader: %v", err)
		}
		r := newTestRepo(t)
		store := newTestStore(t)
		if _, err := Import(context.Background(), r, store, "http://dest.example", zr); err == nil {
			t.Fatal("expected Import to reject a zip exceeding maxZipTotalUncompressed")
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

// corruptComponentLinkRelease rewrites the manifest's one pinned
// productRelease -> component link ("productReleases[0].components[0].release")
// to a well-formed but nonexistent UUID, via a targeted decode/mutate/re-encode
// of manifest.json rather than a raw string replace, since the same UUID also
// appears (correctly) as the referenced component release's own "uuid" field
// elsewhere in the document. The rest of the manifest -- including that
// component release's real identity -- is left untouched, so Import proceeds
// normally until it reaches LinkComponent, whose FK insert then fails.
func corruptComponentLinkRelease(t *testing.T, zipBytes []byte) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	replaced := false
	for _, f := range zr.File {
		w, err := zw.Create(f.Name)
		if err != nil {
			t.Fatalf("zw.Create(%s): %v", f.Name, err)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		if f.Name == "manifest.json" {
			raw, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("read manifest.json: %v", err)
			}
			var doc map[string]any
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatalf("unmarshal manifest.json: %v", err)
			}
			releases, ok := doc["productReleases"].([]any)
			if !ok || len(releases) == 0 {
				t.Fatal("test bug: manifest has no productReleases")
			}
			pr, ok := releases[0].(map[string]any)
			if !ok {
				t.Fatal("test bug: productReleases[0] not an object")
			}
			comps, ok := pr["components"].([]any)
			if !ok || len(comps) == 0 {
				t.Fatal("test bug: productReleases[0] has no components")
			}
			comp, ok := comps[0].(map[string]any)
			if !ok {
				t.Fatal("test bug: components[0] not an object")
			}
			if _, ok := comp["release"]; !ok {
				t.Fatal("test bug: components[0] has no pinned release")
			}
			comp["release"] = "00000000-0000-0000-0000-000000000000"

			mutated, err := json.Marshal(doc)
			if err != nil {
				t.Fatalf("marshal mutated manifest.json: %v", err)
			}
			if _, err := w.Write(mutated); err != nil {
				t.Fatalf("write mutated manifest.json: %v", err)
			}
			replaced = true
		} else {
			if _, err := io.Copy(w, rc); err != nil {
				t.Fatalf("copy %s: %v", f.Name, err)
			}
		}
		_ = rc.Close()
	}
	if !replaced {
		t.Fatal("test bug: no manifest.json entry found")
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return out.Bytes()
}

// mutateManifestJSON decodes zipBytes' manifest.json into a generic map,
// applies mutate, and re-encodes it into a copy of the zip. Used by every
// import-conflict test below to introduce a targeted, single-field content
// difference under an otherwise-identical (uuid[, version]) identity --
// simulating a second, unrelated source server that happened to reuse the
// same key for different content (see repo.ErrImportIdentityConflict's doc
// comment). Mirrors corruptComponentLinkRelease's decode/mutate/re-encode
// approach above, generalized to an arbitrary mutation.
func mutateManifestJSON(t *testing.T, zipBytes []byte, mutate func(doc map[string]any)) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	mutated := false
	for _, f := range zr.File {
		w, err := zw.Create(f.Name)
		if err != nil {
			t.Fatalf("zw.Create(%s): %v", f.Name, err)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		if f.Name == "manifest.json" {
			raw, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("read manifest.json: %v", err)
			}
			var doc map[string]any
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatalf("unmarshal manifest.json: %v", err)
			}
			mutate(doc)
			out2, err := json.Marshal(doc)
			if err != nil {
				t.Fatalf("marshal mutated manifest.json: %v", err)
			}
			if _, err := w.Write(out2); err != nil {
				t.Fatalf("write mutated manifest.json: %v", err)
			}
			mutated = true
		} else {
			if _, err := io.Copy(w, rc); err != nil {
				t.Fatalf("copy %s: %v", f.Name, err)
			}
		}
		_ = rc.Close()
	}
	if !mutated {
		t.Fatal("test bug: no manifest.json entry found")
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return out.Bytes()
}

// TestImportConflictDetection is a table-driven regression test for the
// bundle-identity/deduplication redesign: importing a bundle a second time
// with one field of one entity changed (but the same identity key) must be
// rejected with repo.ErrImportIdentityConflict, and must not silently
// overwrite the original content -- see repo.ErrImportIdentityConflict's
// doc comment for the underlying two-unrelated-source-servers scenario this
// guards against. ImportArtifact and ImportComponentLink get their own
// dedicated tests below (TestImportArtifactConflictFailsFast,
// TestImportComponentLinkConflictButAdminAPIStillAllowsRepin) since they
// need extra machinery (a short context timeout; a follow-up admin-API
// call) that doesn't fit this table.
func TestImportConflictDetection(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(doc map[string]any)
		verify func(t *testing.T, ctx context.Context, dstRepo *repo.Repo, productUUID string)
	}{
		{
			name: "product",
			mutate: func(doc map[string]any) {
				doc["product"].(map[string]any)["name"] = "Different Product"
			},
			verify: func(t *testing.T, ctx context.Context, dstRepo *repo.Repo, productUUID string) {
				got, err := dstRepo.GetProduct(ctx, productUUID)
				if err != nil {
					t.Fatalf("GetProduct: %v", err)
				}
				if got.Name != "Test Product" {
					t.Fatalf("Name = %q, want original unchanged", got.Name)
				}
			},
		},
		{
			name: "productRelease",
			mutate: func(doc map[string]any) {
				releases := doc["productReleases"].([]any)
				releases[0].(map[string]any)["version"] = "9.9.9-different"
			},
			verify: func(t *testing.T, ctx context.Context, dstRepo *repo.Repo, productUUID string) {
				releases, err := dstRepo.ListProductReleasesByProduct(ctx, productUUID, "", "asc", nil, 10)
				if err != nil {
					t.Fatalf("ListProductReleasesByProduct: %v", err)
				}
				if len(releases) != 1 || releases[0].Version != "1.0.0" {
					t.Fatalf("releases = %+v, want exactly the original unchanged 1.0.0 release", releases)
				}
			},
		},
		{
			name: "component",
			mutate: func(doc map[string]any) {
				comps := doc["components"].([]any)
				comps[0].(map[string]any)["name"] = "Different Component"
			},
			verify: func(t *testing.T, ctx context.Context, dstRepo *repo.Repo, productUUID string) {
				releases, err := dstRepo.ListProductReleasesByProduct(ctx, productUUID, "", "asc", nil, 10)
				if err != nil || len(releases) != 1 || len(releases[0].Components) != 1 {
					t.Fatalf("ListProductReleasesByProduct: releases=%+v err=%v", releases, err)
				}
				got, err := dstRepo.GetComponent(ctx, releases[0].Components[0].UUID)
				if err != nil {
					t.Fatalf("GetComponent: %v", err)
				}
				if got.Name != "libfoo" {
					t.Fatalf("Name = %q, want original unchanged", got.Name)
				}
			},
		},
		{
			name: "componentRelease",
			mutate: func(doc map[string]any) {
				releases := doc["componentReleases"].([]any)
				releases[0].(map[string]any)["version"] = "0.0.1-different"
			},
			verify: func(t *testing.T, ctx context.Context, dstRepo *repo.Repo, productUUID string) {
				releases, err := dstRepo.ListProductReleasesByProduct(ctx, productUUID, "", "asc", nil, 10)
				if err != nil || len(releases) != 1 || len(releases[0].Components) != 1 {
					t.Fatalf("ListProductReleasesByProduct: releases=%+v err=%v", releases, err)
				}
				componentReleaseUUID := *releases[0].Components[0].Release
				got, err := dstRepo.GetComponentRelease(ctx, componentReleaseUUID)
				if err != nil {
					t.Fatalf("GetComponentRelease: %v", err)
				}
				if got.Version != "9.9.9" {
					t.Fatalf("Version = %q, want original unchanged", got.Version)
				}
			},
		},
		{
			name: "distribution",
			mutate: func(doc map[string]any) {
				releases := doc["componentReleases"].([]any)
				dists := releases[0].(map[string]any)["distributions"].([]any)
				dists[0].(map[string]any)["description"] = "a completely different distribution"
			},
			verify: func(t *testing.T, ctx context.Context, dstRepo *repo.Repo, productUUID string) {
				releases, err := dstRepo.ListProductReleasesByProduct(ctx, productUUID, "", "asc", nil, 10)
				if err != nil || len(releases) != 1 || len(releases[0].Components) != 1 {
					t.Fatalf("ListProductReleasesByProduct: releases=%+v err=%v", releases, err)
				}
				componentReleaseUUID := *releases[0].Components[0].Release
				cr, err := dstRepo.GetComponentRelease(ctx, componentReleaseUUID)
				if err != nil || len(cr.Distributions) != 1 {
					t.Fatalf("GetComponentRelease: cr=%+v err=%v", cr, err)
				}
				if cr.Distributions[0].Description != "binary tarball" {
					t.Fatalf("Description = %q, want original unchanged", cr.Distributions[0].Description)
				}
			},
		},
		{
			name: "collection",
			mutate: func(doc map[string]any) {
				collections := doc["collections"].([]any)
				collections[0].(map[string]any)["date"] = "2020-01-01T00:00:00Z"
			},
			verify: func(t *testing.T, ctx context.Context, dstRepo *repo.Repo, productUUID string) {
				releases, err := dstRepo.ListProductReleasesByProduct(ctx, productUUID, "", "asc", nil, 10)
				if err != nil || len(releases) != 1 || len(releases[0].Components) != 1 {
					t.Fatalf("ListProductReleasesByProduct: releases=%+v err=%v", releases, err)
				}
				componentReleaseUUID := *releases[0].Components[0].Release
				collections, err := dstRepo.ListCollections(ctx, componentReleaseUUID, "asc", nil, 10, repo.BelongsToComponentRelease)
				if err != nil || len(collections) != 1 {
					t.Fatalf("ListCollections: collections=%+v err=%v", collections, err)
				}
				if collections[0].Date.Year() == 2020 {
					t.Fatalf("Date = %v, want original unchanged (not the mutated 2020 date)", collections[0].Date)
				}
			},
		},
		{
			name: "cleEvent",
			mutate: func(doc map[string]any) {
				cle := doc["product"].(map[string]any)["cle"].(map[string]any)
				events := cle["events"].([]any)
				events[0].(map[string]any)["reason"] = "a completely different reason"
			},
			verify: func(t *testing.T, ctx context.Context, dstRepo *repo.Repo, productUUID string) {
				cle, err := dstRepo.GetCLE(ctx, repo.OwnerProduct, productUUID)
				if err != nil {
					t.Fatalf("GetCLE: %v", err)
				}
				if len(cle.Events) != 1 {
					t.Fatalf("Events = %+v, want exactly 1", cle.Events)
				}
				if cle.Events[0].Reason != "" {
					t.Fatalf("Reason = %q, want original unchanged (empty)", cle.Events[0].Reason)
				}
			},
		},
		{
			name: "artifact",
			mutate: func(doc map[string]any) {
				collections := doc["collections"].([]any)
				artifacts := collections[0].(map[string]any)["artifacts"].([]any)
				artifacts[0].(map[string]any)["type"] = "VULNERABILITIES"
			},
			verify: func(t *testing.T, ctx context.Context, dstRepo *repo.Repo, productUUID string) {
				releases, err := dstRepo.ListProductReleasesByProduct(ctx, productUUID, "", "asc", nil, 10)
				if err != nil || len(releases) != 1 || len(releases[0].Components) != 1 {
					t.Fatalf("ListProductReleasesByProduct: releases=%+v err=%v", releases, err)
				}
				componentReleaseUUID := *releases[0].Components[0].Release
				collections, err := dstRepo.ListCollections(ctx, componentReleaseUUID, "asc", nil, 10, repo.BelongsToComponentRelease)
				if err != nil || len(collections) != 1 || len(collections[0].Artifacts) != 1 {
					t.Fatalf("ListCollections: collections=%+v err=%v", collections, err)
				}
				if collections[0].Artifacts[0].Type != "BOM" {
					t.Fatalf("Type = %q, want original unchanged", collections[0].Artifacts[0].Type)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			srcRepo := newTestRepo(t)
			srcStore := newTestStore(t)
			productUUID := seedProduct(t, srcRepo, srcStore)

			var buf bytes.Buffer
			if err := Export(ctx, srcRepo, srcStore, productUUID, &buf); err != nil {
				t.Fatalf("Export: %v", err)
			}

			dstRepo := newTestRepo(t)
			dstStore := newTestStore(t)
			zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
			if err != nil {
				t.Fatalf("zip.NewReader: %v", err)
			}
			if _, err := Import(ctx, dstRepo, dstStore, "http://dest.example", zr); err != nil {
				t.Fatalf("first Import: %v", err)
			}

			mutated := mutateManifestJSON(t, buf.Bytes(), tc.mutate)
			zr2, err := zip.NewReader(bytes.NewReader(mutated), int64(len(mutated)))
			if err != nil {
				t.Fatalf("zip.NewReader (mutated): %v", err)
			}
			if _, err := Import(ctx, dstRepo, dstStore, "http://dest.example", zr2); !errors.Is(err, repo.ErrImportIdentityConflict) {
				t.Fatalf("second Import: err = %v, want ErrImportIdentityConflict", err)
			}

			tc.verify(t, ctx, dstRepo, productUUID)
		})
	}
}

// TestImportComponentLinkConflictButAdminAPIStillAllowsRepin pins down the
// intentional asymmetry between ImportComponentLink (strict, used by bundle
// import) and LinkComponent (permissive UPSERT-on-conflict, used by the
// regular admin API) -- see ImportComponentLink's doc comment -- so a
// future unrelated refactor doesn't accidentally unify the two behaviors.
func TestImportComponentLinkConflictButAdminAPIStillAllowsRepin(t *testing.T) {
	ctx := context.Background()
	srcRepo := newTestRepo(t)
	srcStore := newTestStore(t)
	productUUID := seedProduct(t, srcRepo, srcStore)

	var buf bytes.Buffer
	if err := Export(ctx, srcRepo, srcStore, productUUID, &buf); err != nil {
		t.Fatalf("Export: %v", err)
	}

	dstRepo := newTestRepo(t)
	dstStore := newTestStore(t)
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	if _, err := Import(ctx, dstRepo, dstStore, "http://dest.example", zr); err != nil {
		t.Fatalf("first Import: %v", err)
	}

	releases, err := dstRepo.ListProductReleasesByProduct(ctx, productUUID, "", "asc", nil, 10)
	if err != nil || len(releases) != 1 || len(releases[0].Components) != 1 {
		t.Fatalf("ListProductReleasesByProduct: releases=%+v err=%v", releases, err)
	}
	componentUUID := releases[0].Components[0].UUID

	// Re-importing a bundle that pins the same (productRelease, component)
	// pair to a *different* release must be rejected.
	mutated := mutateManifestJSON(t, buf.Bytes(), func(doc map[string]any) {
		prReleases := doc["productReleases"].([]any)
		comps := prReleases[0].(map[string]any)["components"].([]any)
		comps[0].(map[string]any)["release"] = "00000000-0000-0000-0000-000000000000"
	})
	zr2, err := zip.NewReader(bytes.NewReader(mutated), int64(len(mutated)))
	if err != nil {
		t.Fatalf("zip.NewReader (mutated): %v", err)
	}
	if _, err := Import(ctx, dstRepo, dstStore, "http://dest.example", zr2); !errors.Is(err, repo.ErrImportIdentityConflict) {
		t.Fatalf("second Import (re-pinning via bundle import): err = %v, want ErrImportIdentityConflict", err)
	}

	// But the admin API's own LinkComponent, called directly (not via bundle
	// import), must still allow re-pinning this same pair to a genuinely
	// different (real, locally-created) release -- that's a deliberate,
	// correct action there, not a conflict.
	newRelease, err := dstRepo.CreateComponentRelease(ctx, componentUUID, repo.ComponentReleaseInput{Version: "2.0.0", CreatedDate: fixedTime()})
	if err != nil {
		t.Fatalf("CreateComponentRelease: %v", err)
	}
	if _, err := dstRepo.LinkComponent(ctx, releases[0].UUID, tea.ComponentRef{UUID: componentUUID, Release: &newRelease.UUID}); err != nil {
		t.Fatalf("LinkComponent (admin API re-pin): %v", err)
	}
	got, err := dstRepo.GetProductRelease(ctx, releases[0].UUID)
	if err != nil {
		t.Fatalf("GetProductRelease: %v", err)
	}
	if len(got.Components) != 1 || got.Components[0].Release == nil || *got.Components[0].Release != newRelease.UUID {
		t.Fatalf("Components = %+v, want re-pinned to %q", got.Components, newRelease.UUID)
	}
}

// TestImportIsAtomicOnFailure is the regression test for the non-atomicity
// bug reported in TODO.md: a bundle that fails partway through Import's DB
// writes must leave nothing behind, not a partially-imported product tree.
// The failure is injected late in the write sequence (LinkComponent, which
// runs after the product, its release, the component, and the component
// release have all already been written within the same transaction), so a
// correct rollback must undo all of those earlier writes too, not just the
// one that actually errored.
func TestImportIsAtomicOnFailure(t *testing.T) {
	ctx := context.Background()
	srcRepo := newTestRepo(t)
	srcStore := newTestStore(t)

	product, err := srcRepo.CreateProduct(ctx, "Atomic Test Product", nil)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	release, err := srcRepo.CreateProductRelease(ctx, product.UUID, repo.ProductReleaseInput{Version: "1.0.0", CreatedDate: fixedTime()})
	if err != nil {
		t.Fatalf("CreateProductRelease: %v", err)
	}
	component, err := srcRepo.CreateComponent(ctx, "libfoo", nil)
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	componentRelease, err := srcRepo.CreateComponentRelease(ctx, component.UUID, repo.ComponentReleaseInput{Version: "9.9.9", CreatedDate: fixedTime()})
	if err != nil {
		t.Fatalf("CreateComponentRelease: %v", err)
	}
	releaseUUID := componentRelease.UUID
	if _, err := srcRepo.LinkComponent(ctx, release.UUID, componentRef(component.UUID, &releaseUUID)); err != nil {
		t.Fatalf("LinkComponent: %v", err)
	}

	var buf bytes.Buffer
	if err := Export(ctx, srcRepo, srcStore, product.UUID, &buf); err != nil {
		t.Fatalf("Export: %v", err)
	}
	corrupted := corruptComponentLinkRelease(t, buf.Bytes())

	zr, err := zip.NewReader(bytes.NewReader(corrupted), int64(len(corrupted)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	dstRepo := newTestRepo(t)
	dstStore := newTestStore(t)
	if _, err := Import(ctx, dstRepo, dstStore, "http://dest.example", zr); err == nil {
		t.Fatal("expected Import to fail on the corrupted component link")
	}

	// Every entity that would have been written before the failure point
	// must also be gone -- proving the whole import rolled back as a unit,
	// not just the one write that errored.
	if _, err := dstRepo.GetProduct(ctx, product.UUID); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("GetProduct after failed import: err = %v, want ErrNotFound", err)
	}
	if _, err := dstRepo.GetComponent(ctx, component.UUID); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("GetComponent after failed import: err = %v, want ErrNotFound", err)
	}
	if _, err := dstRepo.GetProductRelease(ctx, release.UUID); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("GetProductRelease after failed import: err = %v, want ErrNotFound", err)
	}
	if _, err := dstRepo.GetComponentRelease(ctx, componentRelease.UUID); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("GetComponentRelease after failed import: err = %v, want ErrNotFound", err)
	}

	// Watermark bumps ride inside the same transaction as everything else
	// (each entity's own Import* call bumps its own family) -- confirm they
	// roll back too, not just the entity rows, so a failed import can't
	// falsely invalidate list-endpoint caches for content it never
	// actually committed.
	for _, f := range []string{repo.WatermarkProducts, repo.WatermarkProductReleases, repo.WatermarkComponents, repo.WatermarkComponentReleases} {
		w, err := dstRepo.GetWatermark(ctx, f)
		if err != nil {
			t.Fatalf("GetWatermark(%q) after failed import: %v", f, err)
		}
		if w != 0 {
			t.Errorf("watermark(%q) after failed import = %d, want 0 (rolled back)", f, w)
		}
	}
}

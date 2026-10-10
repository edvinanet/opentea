// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package bundle

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/oej/opentea/internal/repo"
)

// TestImportRejectsMissingFile is Import's counterpart to
// TestCheckMissingFile (check_test.go) and to TestImportRejectsCorruptBundle
// above: a checksum the manifest references with no backing files/ entry
// at all must be rejected, the same guarantee missingFileHashes documents
// Import as providing (see import.go's own comment on that check).
func TestImportRejectsMissingFile(t *testing.T) {
	ctx := context.Background()
	srcRepo := newTestRepo(t)
	srcStore := newTestStore(t)
	productUUID := seedProduct(t, srcRepo, srcStore)

	var buf bytes.Buffer
	if err := Export(ctx, srcRepo, srcStore, productUUID, &buf); err != nil {
		t.Fatalf("Export: %v", err)
	}
	stripped := removeFirstFileEntry(t, buf.Bytes())
	zr, err := zip.NewReader(bytes.NewReader(stripped), int64(len(stripped)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	dstRepo := newTestRepo(t)
	dstStore := newTestStore(t)
	if _, err := Import(ctx, dstRepo, dstStore, "http://dest.example", zr); err == nil {
		t.Fatal("expected Import to reject a bundle with a checksum and no backing files/ entry")
	}
}

// firstFileEntryHash returns the declared sha256 (the "files/" entry's own
// name, not its content) of the first files/ entry in zipBytes.
func firstFileEntryHash(t *testing.T, zipBytes []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "files/") {
			return strings.TrimPrefix(f.Name, "files/")
		}
	}
	t.Fatal("test bug: no files/ entry found")
	return ""
}

// TestImportUncheckedBypassesChecksumMismatch is ImportUnchecked's whole
// reason to exist, half A: the same corrupted bundle
// TestImportRejectsCorruptBundle proves plain Import rejects must
// successfully force-import via ImportUnchecked instead -- and the
// corrupted bytes must actually land in storage under the declared
// (wrong) checksum, not be silently corrected, so a client reading it back
// genuinely encounters the mismatch docs/consumer-api-conformance-test-rig.md's
// suite needs to detect.
func TestImportUncheckedBypassesChecksumMismatch(t *testing.T) {
	ctx := context.Background()
	srcRepo := newTestRepo(t)
	srcStore := newTestStore(t)
	productUUID := seedProduct(t, srcRepo, srcStore)

	var buf bytes.Buffer
	if err := Export(ctx, srcRepo, srcStore, productUUID, &buf); err != nil {
		t.Fatalf("Export: %v", err)
	}
	corrupted := corruptFirstFileEntry(t, buf.Bytes())
	declaredHash := firstFileEntryHash(t, corrupted)

	zr, err := zip.NewReader(bytes.NewReader(corrupted), int64(len(corrupted)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	dstRepo := newTestRepo(t)
	dstStore := newTestStore(t)
	if _, err := ImportUnchecked(ctx, dstRepo, dstStore, "http://dest.example", zr); err != nil {
		t.Fatalf("ImportUnchecked: %v, want success despite the corrupted content", err)
	}

	rc, err := dstStore.Open(ctx, declaredHash)
	if err != nil {
		t.Fatalf("Open(%s) after ImportUnchecked: %v, want the corrupted blob to actually be stored under its declared hash", declaredHash, err)
	}
	got, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatalf("read stored blob: %v", err)
	}
	if string(got) != "corrupted content that will not match the hash" {
		t.Fatalf("stored blob = %q, want the corrupted content corruptFirstFileEntry wrote", got)
	}
}

// TestImportUncheckedBypassesMissingFile is ImportUnchecked's half B: the
// same bundle TestImportRejectsMissingFile proves plain Import rejects
// (a declared checksum with no files/ entry backing it at all) must
// import successfully via ImportUnchecked -- leaving a checksum with
// nothing downloadable behind it, the other corruption shape
// docs/consumer-api-conformance-test-rig.md's suite needs to detect.
func TestImportUncheckedBypassesMissingFile(t *testing.T) {
	ctx := context.Background()
	srcRepo := newTestRepo(t)
	srcStore := newTestStore(t)
	productUUID := seedProduct(t, srcRepo, srcStore)

	var buf bytes.Buffer
	if err := Export(ctx, srcRepo, srcStore, productUUID, &buf); err != nil {
		t.Fatalf("Export: %v", err)
	}
	stripped := removeFirstFileEntry(t, buf.Bytes())
	zr, err := zip.NewReader(bytes.NewReader(stripped), int64(len(stripped)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	dstRepo := newTestRepo(t)
	dstStore := newTestStore(t)
	if _, err := ImportUnchecked(ctx, dstRepo, dstStore, "http://dest.example", zr); err != nil {
		t.Fatalf("ImportUnchecked: %v, want success despite the missing file", err)
	}
}

// TestImportUncheckedStillRejectsDanglingComponentLink confirms
// ImportUnchecked's documented boundary: a dangling cross-reference (a
// product release pinning a component-release UUID absent from the
// manifest's own top-level componentReleases[]) still fails, because it's
// a foreign-key violation at the database level -- unaffected by skipping
// the schema/file-hash checks, which is all ImportUnchecked actually
// bypasses.
func TestImportUncheckedStillRejectsDanglingComponentLink(t *testing.T) {
	ctx := context.Background()
	srcRepo := newTestRepo(t)
	srcStore := newTestStore(t)

	product, err := srcRepo.CreateProduct(ctx, "Dangling Link Test Product", nil)
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
	if _, err := ImportUnchecked(ctx, dstRepo, dstStore, "http://dest.example", zr); err == nil {
		t.Fatal("expected ImportUnchecked to still reject a dangling component-release reference")
	}
}

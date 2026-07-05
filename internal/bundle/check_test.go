package bundle

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestCheckValidBundle(t *testing.T) {
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

	report, err := Check(zr)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !report.Valid {
		t.Fatalf("report = %+v, want Valid=true", report)
	}
	if len(report.SchemaErrors) != 0 || len(report.HashMismatches) != 0 || len(report.MissingFiles) != 0 {
		t.Fatalf("report = %+v, want no errors", report)
	}
}

func TestCheckCorruptBundle(t *testing.T) {
	r := newTestRepo(t)
	store := newTestStore(t)
	productUUID := seedProduct(t, r, store)

	var buf bytes.Buffer
	if err := Export(context.Background(), r, store, productUUID, &buf); err != nil {
		t.Fatalf("Export: %v", err)
	}
	corrupted := corruptFirstFileEntry(t, buf.Bytes())
	zr, err := zip.NewReader(bytes.NewReader(corrupted), int64(len(corrupted)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	report, err := Check(zr)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if report.Valid {
		t.Fatal("report.Valid = true, want false for a corrupt bundle")
	}
	if len(report.HashMismatches) != 1 {
		t.Fatalf("HashMismatches = %+v, want exactly 1", report.HashMismatches)
	}
}

func TestCheckMissingFile(t *testing.T) {
	r := newTestRepo(t)
	store := newTestStore(t)
	productUUID := seedProduct(t, r, store)

	var buf bytes.Buffer
	if err := Export(context.Background(), r, store, productUUID, &buf); err != nil {
		t.Fatalf("Export: %v", err)
	}
	stripped := removeFirstFileEntry(t, buf.Bytes())
	zr, err := zip.NewReader(bytes.NewReader(stripped), int64(len(stripped)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	report, err := Check(zr)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if report.Valid {
		t.Fatal("report.Valid = true, want false when a referenced file is missing")
	}
	if len(report.MissingFiles) != 1 {
		t.Fatalf("MissingFiles = %+v, want exactly 1", report.MissingFiles)
	}
}

func TestCheckBadManifest(t *testing.T) {
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

	report, err := Check(zr)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if report.Valid {
		t.Fatal("report.Valid = true, want false for a schema-invalid manifest")
	}
	if len(report.SchemaErrors) == 0 {
		t.Fatal("SchemaErrors is empty, want at least one violation reported")
	}
}

// removeFirstFileEntry rewrites a bundle zip with its first files/<sha256>
// entry dropped entirely, leaving the manifest's reference to it dangling.
func removeFirstFileEntry(t *testing.T, zipBytes []byte) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	removed := false
	for _, f := range zr.File {
		if !removed && strings.HasPrefix(f.Name, "files/") {
			removed = true
			continue
		}
		w, err := zw.Create(f.Name)
		if err != nil {
			t.Fatalf("zw.Create(%s): %v", f.Name, err)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		if _, err := io.Copy(w, rc); err != nil {
			t.Fatalf("copy %s: %v", f.Name, err)
		}
		rc.Close()
	}
	if !removed {
		t.Fatal("test bug: no files/ entry found to remove")
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return out.Bytes()
}

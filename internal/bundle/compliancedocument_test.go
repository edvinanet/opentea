// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package bundle

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
)

// addBadComplianceDocumentIdentifierToProduct rewrites the manifest's
// single "product" object to carry an invalid COMPLIANCE_DOCUMENT
// identifier (TEA 1.0 forbids that identifier type on products entirely)
// -- same targeted decode/mutate/re-encode approach as
// corruptComponentLinkRelease, applied to a different field.
func addBadComplianceDocumentIdentifierToProduct(t *testing.T, zipBytes []byte) []byte {
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
			product, ok := doc["product"].(map[string]any)
			if !ok {
				t.Fatal("test bug: manifest has no product object")
			}
			identifiers, _ := product["identifiers"].([]any)
			product["identifiers"] = append(identifiers, map[string]any{
				"idType": "COMPLIANCE_DOCUMENT", "idValue": "GDPR",
			})
			mutatedRaw, err := json.Marshal(doc)
			if err != nil {
				t.Fatalf("marshal mutated manifest: %v", err)
			}
			if _, err := w.Write(mutatedRaw); err != nil {
				t.Fatalf("write mutated manifest.json: %v", err)
			}
			mutated = true
			_ = rc.Close()
			continue
		}
		if _, err := io.Copy(w, rc); err != nil {
			t.Fatalf("copy %s: %v", f.Name, err)
		}
		_ = rc.Close()
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if !mutated {
		t.Fatal("test bug: manifest.json entry never found")
	}
	return out.Bytes()
}

// TestImportRejectsInvalidComplianceDocumentIdentifier confirms a bundle
// carrying a COMPLIANCE_DOCUMENT identifier on a product (TEA 1.0
// forbids that identifier type there entirely, spec/openapi.yaml's
// identifier-type) fails the whole import cleanly -- internal/bundle
// itself needed no new validation code: Import runs the manifest in one
// transaction, and internal/repo's insertIdentifiers (identifier.go)
// already rejects it; this just confirms that error actually propagates
// out of Import rather than being swallowed or silently accepted.
func TestImportRejectsInvalidComplianceDocumentIdentifier(t *testing.T) {
	ctx := context.Background()
	srcRepo := newTestRepo(t)
	srcStore := newTestStore(t)
	productUUID := seedProduct(t, srcRepo, srcStore)

	var buf bytes.Buffer
	if err := Export(ctx, srcRepo, srcStore, productUUID, &buf); err != nil {
		t.Fatalf("Export: %v", err)
	}

	mutated := addBadComplianceDocumentIdentifierToProduct(t, buf.Bytes())
	zr, err := zip.NewReader(bytes.NewReader(mutated), int64(len(mutated)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	dstRepo := newTestRepo(t)
	dstStore := newTestStore(t)
	if _, err := Import(ctx, dstRepo, dstStore, "http://dest.example", zr); err == nil {
		t.Fatal("expected Import to reject a product carrying a COMPLIANCE_DOCUMENT identifier")
	}
}

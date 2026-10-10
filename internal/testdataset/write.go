// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package testdataset

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/oej/opentea/internal/bundle"
)

// WriteBundle marshals m as manifest.json and zips it together with
// files[sha256]=content as files/<sha256> entries -- the exact layout
// internal/bundle/export.go produces and internal/bundle/check.go (and
// real import) expect, so a hand-assembled Manifest here is
// indistinguishable from one a real server exported.
func WriteBundle(path string, m bundle.Manifest, files map[string][]byte) error {
	f, err := os.Create(path) //nolint:gosec // path is this generator's own -out argument, operator-controlled, not remote input
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)

	mw, err := zw.Create("manifest.json")
	if err != nil {
		return fmt.Errorf("create manifest.json entry: %w", err)
	}
	if err := writeManifestJSON(mw, m); err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}

	for sha256Hex, content := range files {
		fw, err := zw.Create("files/" + sha256Hex)
		if err != nil {
			return fmt.Errorf("create files/%s entry: %w", sha256Hex, err)
		}
		if _, err := fw.Write(content); err != nil {
			return fmt.Errorf("write files/%s: %w", sha256Hex, err)
		}
	}

	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}

// writeManifestJSON encodes m as indented JSON into w -- shared by
// WriteBundle and the package's own tests, which build the same zip
// layout in-memory to validate a Manifest against the real schema
// without writing to disk.
func writeManifestJSON(w io.Writer, m bundle.Manifest) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(m)
}

// WriteBundleRaw is WriteBundle for a caller that needs to write
// manifest.json's exact bytes directly (e.g. deliberately-invalid JSON a
// bundle.Manifest value could never produce) rather than marshal one --
// used only by the bad-bundle builders.
func WriteBundleRaw(path string, manifestJSON []byte, files map[string][]byte) error {
	f, err := os.Create(path) //nolint:gosec // path is this generator's own -out argument, operator-controlled, not remote input
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)
	mw, err := zw.Create("manifest.json")
	if err != nil {
		return fmt.Errorf("create manifest.json entry: %w", err)
	}
	if _, err := io.Copy(mw, bytes.NewReader(manifestJSON)); err != nil {
		return fmt.Errorf("write manifest.json: %w", err)
	}
	for sha256Hex, content := range files {
		fw, err := zw.Create("files/" + sha256Hex)
		if err != nil {
			return fmt.Errorf("create files/%s entry: %w", sha256Hex, err)
		}
		if _, err := fw.Write(content); err != nil {
			return fmt.Errorf("write files/%s: %w", sha256Hex, err)
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}

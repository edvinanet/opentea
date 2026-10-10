// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Command testbundlegen writes the reference bundle zip files
// docs/bundle-import-export-test-rig.md and
// docs/consumer-api-conformance-test-rig.md describe -- the good datasets
// (Simple, Complex, Lifecycle & compliance) and the bad-bundle variants --
// to a directory, by default testdata/bundles/. internal/testdataset's
// build functions are the actual source of truth; this is a thin wrapper
// that calls them and writes the result, so the committed zips can be
// regenerated deterministically without hand-editing JSON.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/oej/opentea/internal/bundle"
	"github.com/oej/opentea/internal/testdataset"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("testbundlegen", flag.ExitOnError)
	out := fs.String("out", "testdata/bundles", "output directory for the generated bundle zip files")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if err := os.MkdirAll(*out, 0o755); err != nil { //nolint:gosec // generator's own output directory, operator-controlled
		return fmt.Errorf("create %s: %w", *out, err)
	}

	simple := testdataset.Simple()
	complexManifest, complexFiles := testdataset.Complex()
	lifecycle, lifecycleFiles := testdataset.Lifecycle()

	badChecksumMismatch, badChecksumMismatchFiles := testdataset.BadChecksumMismatch()
	badMissingFile, badMissingFileFiles := testdataset.BadMissingFile()
	badFormatVersion, badFormatVersionFiles := testdataset.BadFormatVersion()
	badDanglingComponentRelease, badDanglingComponentReleaseFiles := testdataset.BadDanglingComponentRelease()
	badDanglingComponent, badDanglingComponentFiles := testdataset.BadDanglingComponent()

	writes := []struct {
		name  string
		m     bundle.Manifest
		files map[string][]byte
	}{
		{"simple.zip", simple, nil},
		{"complex.zip", complexManifest, complexFiles},
		{"lifecycle-compliance.zip", lifecycle, lifecycleFiles},
		{"bad-checksum-mismatch.zip", badChecksumMismatch, badChecksumMismatchFiles},
		{"bad-missing-file.zip", badMissingFile, badMissingFileFiles},
		{"bad-format-version.zip", badFormatVersion, badFormatVersionFiles},
		{"bad-dangling-component-release.zip", badDanglingComponentRelease, badDanglingComponentReleaseFiles},
		{"bad-dangling-component.zip", badDanglingComponent, badDanglingComponentFiles},
	}

	for _, w := range writes {
		path := filepath.Join(*out, w.name)
		if err := testdataset.WriteBundle(path, w.m, w.files); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		fmt.Println(path)
	}
	return nil
}

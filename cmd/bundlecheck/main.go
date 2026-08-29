// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Command bundlecheck validates one or more product export/import bundle
// zips (see docs/bundle-format.md) without needing a running server, a
// database, or admin authentication -- it only reads the given zip files.
// Useful for checking a bundle before sending it to another organization, or
// verifying one received from elsewhere.
package main

import (
	"archive/zip"
	"fmt"
	"os"

	"github.com/oej/opentea/internal/bundle"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: bundlecheck <bundle.zip> [<bundle.zip> ...]")
		return 1
	}

	allValid := true
	for _, path := range args {
		valid, err := checkOne(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: error: %v\n", path, err)
			allValid = false
			continue
		}
		if !valid {
			allValid = false
		}
	}
	if allValid {
		return 0
	}
	return 1
}

func checkOne(path string) (valid bool, err error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return false, fmt.Errorf("open: %w", err)
	}
	defer func() { _ = zr.Close() }()

	report, err := bundle.Check(&zr.Reader)
	if err != nil {
		return false, err
	}

	printReport(path, report)
	return report.Valid, nil
}

func printReport(path string, report *bundle.CheckReport) {
	if report.Valid {
		fmt.Printf("%s: OK\n", path)
	} else {
		fmt.Printf("%s: INVALID\n", path)
	}
	for _, e := range report.SchemaErrors {
		fmt.Printf("  schema error: %s\n", e)
	}
	for _, f := range report.HashMismatches {
		fmt.Printf("  hash mismatch: %s (content doesn't match its claimed checksum)\n", f)
	}
	for _, h := range report.MissingFiles {
		fmt.Printf("  missing file: files/%s (referenced by the manifest, not present in the bundle)\n", h)
	}
	for _, h := range report.OrphanFiles {
		fmt.Printf("  note: files/%s is present but not referenced by anything in the manifest\n", h)
	}
}

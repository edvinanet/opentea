// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package bundle

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// CheckReport is the result of validating a bundle zip without importing it
// -- nothing is persisted anywhere; this only reads the zip itself. Distinct
// from bundle.Import's own validation (which serves the same guarantees but
// as a side effect of actually applying the bundle).
type CheckReport struct {
	Valid bool

	// SchemaErrors comes from ValidateManifest, if manifest.json doesn't
	// conform to schema.json.
	SchemaErrors []string

	// HashMismatches lists files/<sha256> entries whose actual content hash
	// doesn't match their claimed name -- a corrupted or tampered bundle.
	HashMismatches []string

	// MissingFiles lists SHA-256 checksums referenced by a distribution or
	// artifact format in the manifest that have no corresponding files/
	// entry in the zip.
	MissingFiles []string

	// OrphanFiles lists files/ entries present in the zip but not
	// referenced by anything in the manifest -- harmless, but worth
	// flagging; doesn't affect Valid.
	OrphanFiles []string
}

// Check validates a bundle zip's manifest against the JSON Schema and
// verifies every files/<sha256> entry's actual content hash against its
// claimed name, without persisting anything. Unlike Import, it keeps
// checking everything it can even after finding a problem, so a single run
// reports every issue rather than just the first one.
func Check(zr *zip.Reader) (*CheckReport, error) {
	if err := checkZipResourceLimits(zr); err != nil {
		return nil, err
	}

	report := &CheckReport{}

	manifestRaw, err := readZipFile(zr, "manifest.json", maxManifestSize)
	if err != nil {
		return nil, err
	}

	if err := ValidateManifest(manifestRaw); err != nil {
		report.SchemaErrors = append(report.SchemaErrors, err.Error())
	}

	var m Manifest
	if err := json.Unmarshal(manifestRaw, &m); err != nil {
		report.SchemaErrors = append(report.SchemaErrors, fmt.Sprintf("decode manifest.json: %v", err))
		return report, nil // nothing further can be checked without a decoded manifest
	}

	if err := checkFileHashes(zr, m, report); err != nil {
		return nil, err
	}

	report.Valid = len(report.SchemaErrors) == 0 && len(report.HashMismatches) == 0 && len(report.MissingFiles) == 0
	return report, nil
}

// checkFileHashes compares the manifest's declared file references against
// the zip's actual files/ entries, populating report's HashMismatches,
// MissingFiles, and OrphanFiles. Split out of Check itself to keep that
// function's own branching within the project's complexity budget.
func checkFileHashes(zr *zip.Reader, m Manifest, report *CheckReport) error {
	expected := map[string]struct{}{}
	collectFileHashes(expected, m.Collections)
	for _, cr := range m.ComponentReleases {
		collectDistributionFileHashes(expected, cr.Distributions)
	}

	present := map[string]struct{}{}
	for _, f := range zr.File {
		const prefix = "files/"
		if !strings.HasPrefix(f.Name, prefix) {
			continue
		}
		claimedSHA256 := f.Name[len(prefix):]
		present[claimedSHA256] = struct{}{}

		actualSHA256, err := sha256OfZipEntry(f)
		if err != nil {
			return fmt.Errorf("read %s: %w", f.Name, err)
		}
		if actualSHA256 != claimedSHA256 {
			report.HashMismatches = append(report.HashMismatches, f.Name)
		}
	}

	for hash := range expected {
		if _, ok := present[hash]; !ok {
			report.MissingFiles = append(report.MissingFiles, hash)
		}
	}
	for hash := range present {
		if _, ok := expected[hash]; !ok {
			report.OrphanFiles = append(report.OrphanFiles, hash)
		}
	}
	sort.Strings(report.HashMismatches)
	sort.Strings(report.MissingFiles)
	sort.Strings(report.OrphanFiles)
	return nil
}

func sha256OfZipEntry(f *zip.File) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()

	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(rc, maxZipEntrySize+1))
	if err != nil {
		return "", err
	}
	if n > maxZipEntrySize {
		return "", fmt.Errorf("zip entry %s exceeds %d byte limit", f.Name, maxZipEntrySize)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Package testdataset builds the reference bundle datasets described in
// docs/bundle-import-export-test-rig.md and docs/consumer-api-conformance-test-rig.md.
// It is the single source of truth both documents and both consumers
// (cmd/testbundlegen, which writes the bundles to disk, and the conformance
// suite, which reads them back through /tea/v1) share -- the "expected"
// structure a conformance run checks a live server against is this
// package's own Manifest values, never a second, separately-authored
// description of the same data.
package testdataset

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
)

// deterministicUUID derives a schema-valid, lowercase-hex UUID-shaped
// string from parts -- deterministic so regenerating a dataset produces
// byte-identical output (diffs cleanly in review), without hand-typing
// hundreds of UUIDs. internal/bundle/schema.json's uuid pattern
// (^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$) only
// constrains hex-digit grouping, not RFC 4122 version/variant bits, so a
// plain SHA-256-derived value satisfies it without needing a real UUID
// library.
func deterministicUUID(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	b := h[:16]
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// withMarker appends a recognizable, parseable marker to an otherwise
// realistic name -- the entity's own bundle-authored UUID -- so the
// conformance suite can recognize "which authored entity is this" purely
// from a non-normative free-text field, independent of (and a cross-check
// against) identifier-based resolution. See both test-rig docs' "identity
// resolution" sections for why this exists: no bundle importer is
// required to preserve the bundle's authored UUIDs on import.
func withMarker(name, uuid string) string {
	return fmt.Sprintf("%s [test:%s]", name, uuid)
}

var markerRe = regexp.MustCompile(`\[test:([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\]`)

// ExtractMarker returns the authored UUID embedded in s by withMarker, if
// any. Exported for the conformance suite (a different binary) to call
// against whatever free-text field a live server's response actually
// carries it in.
func ExtractMarker(s string) (uuid string, ok bool) {
	m := markerRe.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	return m[1], true
}

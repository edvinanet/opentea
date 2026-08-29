// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teaclient

import (
	"strconv"
	"strings"
)

// SupportedVersions is the TEA API version(s) this client implements --
// used by BootstrapDiscover to pick the highest version a candidate
// endpoint and this client both support (TEA discovery spec's "Connecting
// to the API" section: a client MUST pick an endpoint sharing at least one
// version, and MUST prefer the highest mutually-supported one by SemVer
// 2.0.0 precedence). Matches opentea's own server-side default
// (internal/config.Config's TEA_VERSIONS default, "0.4.0") since this is
// the reference client for this project's own server, but any conformant
// TEA server's well-known endpoint can be matched against it too.
var SupportedVersions = []string{"0.4.0"}

// semVer is a parsed SemVer 2.0.0 version: numeric major/minor/patch, plus
// an unparsed pre-release identifier (only its dot-separated fields are
// split out, each classified as numeric or not -- build metadata is
// deliberately not parsed since it has no bearing on precedence at all,
// per §10).
type semVer struct {
	major, minor, patch int
	preRelease          []string // nil for a normal (non-pre-release) version
	valid               bool
}

// parseSemVer parses s per SemVer 2.0.0 §2's grammar, permissively enough
// to accept the two-component form ("1.0") this project's own version
// examples use alongside strict three-component versions -- treating a
// missing patch as 0. Build metadata (a "+..." suffix) is discarded, not
// validated. Returns semVer{valid: false} for anything that doesn't parse
// as at least MAJOR.MINOR, rather than erroring -- callers treat an
// invalid version as never matching/ordering meaningfully, which is the
// right default for data coming from a remote server's well-known
// document rather than this codebase's own input.
func parseSemVer(s string) semVer {
	s, _, _ = strings.Cut(s, "+") // discard build metadata
	core, pre, hasPre := strings.Cut(s, "-")

	parts := strings.Split(core, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return semVer{}
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semVer{}
		}
		nums[i] = n
	}

	v := semVer{major: nums[0], minor: nums[1], patch: nums[2], valid: true}
	if hasPre {
		v.preRelease = strings.Split(pre, ".")
	}
	return v
}

// compareSemVer returns -1, 0, or 1 as a has lower, equal, or higher
// precedence than b, per SemVer 2.0.0 §11. An unparseable version compares
// as lower precedence than any parseable one (and equal to another
// unparseable one) -- an arbitrary but total ordering, since this is only
// used to pick a *highest* mutual version among values already known to be
// well-formed version strings from a schema-validated well-known document
// in the normal case.
func compareSemVer(a, b string) int {
	va, vb := parseSemVer(a), parseSemVer(b)
	if va.valid != vb.valid {
		if va.valid {
			return 1
		}
		return -1
	}
	if !va.valid {
		return 0
	}

	if c := compareInt(va.major, vb.major); c != 0 {
		return c
	}
	if c := compareInt(va.minor, vb.minor); c != 0 {
		return c
	}
	if c := compareInt(va.patch, vb.patch); c != 0 {
		return c
	}
	return comparePreRelease(va.preRelease, vb.preRelease)
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// comparePreRelease implements SemVer 2.0.0 §11's pre-release precedence
// rule: no pre-release fields (a normal release) has higher precedence
// than having any; otherwise compare fields left to right (numeric
// identifiers compared numerically, alphanumeric compared as ASCII
// strings, numeric always lower than alphanumeric), and if all shared
// fields are equal, the version with more fields has higher precedence.
func comparePreRelease(a, b []string) int {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	if len(a) == 0 {
		return 1 // a is a normal release, b is a pre-release: a is higher
	}
	if len(b) == 0 {
		return -1
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		if c := comparePreReleaseField(a[i], b[i]); c != 0 {
			return c
		}
	}
	return compareInt(len(a), len(b))
}

func comparePreReleaseField(a, b string) int {
	na, aErr := strconv.Atoi(a)
	nb, bErr := strconv.Atoi(b)
	aNum, bNum := aErr == nil, bErr == nil
	switch {
	case aNum && bNum:
		return compareInt(na, nb)
	case aNum && !bNum:
		return -1 // numeric identifiers always have lower precedence than alphanumeric
	case !aNum && bNum:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

// highestMutualVersion returns the highest-precedence version present in
// both client and endpoint, and whether any match exists at all. Matching
// is by parsed semantic equality (major.minor.patch + pre-release fields),
// not exact string equality -- so a client offering "1.0.0" matches an
// endpoint advertising the equally-valid two-component form "1.0", per the
// schema's own pattern allowing both. The returned string is always taken
// from endpoint's list (the form the server itself advertised), since
// that's what gets used to build the "/v<version>" path.
func highestMutualVersion(client, endpoint []string) (string, bool) {
	var best string
	found := false
	for _, c := range client {
		cv := parseSemVer(c)
		if !cv.valid {
			continue
		}
		for _, e := range endpoint {
			ev := parseSemVer(e)
			if !ev.valid || !semVerEqual(cv, ev) {
				continue
			}
			if !found || compareSemVer(e, best) > 0 {
				best = e
				found = true
			}
		}
	}
	return best, found
}

func semVerEqual(a, b semVer) bool {
	if a.major != b.major || a.minor != b.minor || a.patch != b.patch {
		return false
	}
	if len(a.preRelease) != len(b.preRelease) {
		return false
	}
	for i := range a.preRelease {
		if a.preRelease[i] != b.preRelease[i] {
			return false
		}
	}
	return true
}

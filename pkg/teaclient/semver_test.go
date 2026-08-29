// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package teaclient

import "testing"

// TestCompareSemVerCanonicalOrdering exercises SemVer 2.0.0 §11's own
// worked example ordering directly from the spec:
// 1.0.0-alpha < 1.0.0-alpha.1 < 1.0.0-alpha.beta < 1.0.0-beta <
// 1.0.0-beta.2 < 1.0.0-beta.11 < 1.0.0-rc.1 < 1.0.0.
func TestCompareSemVerCanonicalOrdering(t *testing.T) {
	ordered := []string{
		"1.0.0-alpha",
		"1.0.0-alpha.1",
		"1.0.0-alpha.beta",
		"1.0.0-beta",
		"1.0.0-beta.2",
		"1.0.0-beta.11",
		"1.0.0-rc.1",
		"1.0.0",
	}
	for i := 0; i < len(ordered)-1; i++ {
		lower, higher := ordered[i], ordered[i+1]
		if c := compareSemVer(lower, higher); c >= 0 {
			t.Errorf("compareSemVer(%q, %q) = %d, want < 0", lower, higher, c)
		}
		if c := compareSemVer(higher, lower); c <= 0 {
			t.Errorf("compareSemVer(%q, %q) = %d, want > 0", higher, lower, c)
		}
		if c := compareSemVer(lower, lower); c != 0 {
			t.Errorf("compareSemVer(%q, %q) = %d, want 0", lower, lower, c)
		}
	}
}

func TestCompareSemVerNumericVsAlphanumericIdentifiers(t *testing.T) {
	// Numeric identifiers always have lower precedence than alphanumeric
	// ones, and numeric identifiers compare numerically (10 > 9), not
	// lexically ("10" < "9" as strings).
	if compareSemVer("1.0.0-9", "1.0.0-10") >= 0 {
		t.Error("numeric pre-release identifiers must compare numerically: 9 < 10")
	}
	if compareSemVer("1.0.0-10", "1.0.0-abc") >= 0 {
		t.Error("a numeric identifier must have lower precedence than an alphanumeric one")
	}
}

func TestCompareSemVerReleaseVsPatch(t *testing.T) {
	if compareSemVer("1.2.3", "1.2.4") >= 0 {
		t.Error("1.2.3 should have lower precedence than 1.2.4")
	}
	if compareSemVer("2.0.0", "1.9.9") <= 0 {
		t.Error("2.0.0 should have higher precedence than 1.9.9")
	}
}

func TestCompareSemVerTwoComponentForm(t *testing.T) {
	// The schema's version pattern allows a two-component form
	// (MAJOR.MINOR, no patch) -- must be treated as patch 0.
	if c := compareSemVer("1.0", "1.0.0"); c != 0 {
		t.Errorf("compareSemVer(1.0, 1.0.0) = %d, want 0", c)
	}
}

func TestCompareSemVerInvalidVersions(t *testing.T) {
	if compareSemVer("not-a-version", "1.0.0") >= 0 {
		t.Error("an unparseable version must not outrank a valid one")
	}
	if c := compareSemVer("also-not-a-version", "not-a-version"); c != 0 {
		t.Errorf("two unparseable versions should compare equal, got %d", c)
	}
}

func TestHighestMutualVersion(t *testing.T) {
	tests := []struct {
		name           string
		client         []string
		endpoint       []string
		wantVersion    string
		wantFoundMatch bool
	}{
		{"single exact match", []string{"0.4.0"}, []string{"0.2.0-beta.2", "0.4.0"}, "0.4.0", true},
		{"picks highest of several matches", []string{"0.4.0", "1.0.0"}, []string{"0.4.0", "1.0.0", "2.0.0"}, "1.0.0", true},
		{"two-component form matches three-component", []string{"0.4.0"}, []string{"0.4"}, "0.4", true},
		{"no overlap", []string{"0.4.0"}, []string{"1.0.0", "2.0.0"}, "", false},
		{"empty endpoint list", []string{"0.4.0"}, nil, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := highestMutualVersion(tt.client, tt.endpoint)
			if ok != tt.wantFoundMatch {
				t.Fatalf("highestMutualVersion(%v, %v) found=%v, want %v", tt.client, tt.endpoint, ok, tt.wantFoundMatch)
			}
			if ok && got != tt.wantVersion {
				t.Fatalf("highestMutualVersion(%v, %v) = %q, want %q", tt.client, tt.endpoint, got, tt.wantVersion)
			}
		})
	}
}

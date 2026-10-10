// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package testdataset

import "testing"

func TestExtractMarkerBareUUID(t *testing.T) {
	uuid := deterministicUUID("marker-test", "bare")
	s := withMarker("Acme Widget", uuid)
	marker, ok := ExtractMarker(s)
	if !ok || marker != uuid {
		t.Fatalf("ExtractMarker(%q) = %q, %v; want %q, true", s, marker, ok, uuid)
	}
}

func TestExtractMarkerUUIDStripsSuffix(t *testing.T) {
	uuid := deterministicUUID("marker-test", "suffix")
	s := withMarker("Initial collection", uuid+":1")
	got, ok := ExtractMarkerUUID(s)
	if !ok || got != uuid {
		t.Fatalf("ExtractMarkerUUID(%q) = %q, %v; want %q, true", s, got, ok, uuid)
	}
}

func TestExtractMarkerUUIDOnBareMarker(t *testing.T) {
	uuid := deterministicUUID("marker-test", "bare2")
	s := withMarker("Acme Widget", uuid)
	got, ok := ExtractMarkerUUID(s)
	if !ok || got != uuid {
		t.Fatalf("ExtractMarkerUUID(%q) = %q, %v; want %q, true", s, got, ok, uuid)
	}
}

func TestExtractCollectionMarker(t *testing.T) {
	uuid := deterministicUUID("marker-test", "collection")
	s := withMarker("Initial collection for component X", uuid+":3")
	gotUUID, gotVersion, ok := ExtractCollectionMarker(s)
	if !ok || gotUUID != uuid || gotVersion != 3 {
		t.Fatalf("ExtractCollectionMarker(%q) = %q, %d, %v; want %q, 3, true", s, gotUUID, gotVersion, ok, uuid)
	}
}

func TestExtractMarkerNoMarkerPresent(t *testing.T) {
	if _, ok := ExtractMarker("just an ordinary name"); ok {
		t.Fatal("ExtractMarker on a plain string returned ok=true, want false")
	}
	if _, ok := ExtractMarkerUUID("just an ordinary name"); ok {
		t.Fatal("ExtractMarkerUUID on a plain string returned ok=true, want false")
	}
	if _, _, ok := ExtractCollectionMarker("just an ordinary name"); ok {
		t.Fatal("ExtractCollectionMarker on a plain string returned ok=true, want false")
	}
}

// TestRealDatasetCollectionMarkersParse is the regression test for the bug
// this file's other tests were written to catch: every real collection
// UpdateReason.Comment produced by Simple/Complex/Lifecycle must actually
// parse via ExtractCollectionMarker -- markerRe's old UUID-only pattern
// silently failed to match any of them (the marker payload is always
// "<ownerUUID>:<version>", never a bare UUID, for a collection comment).
func TestRealDatasetCollectionMarkersParse(t *testing.T) {
	simple := Simple()
	for _, c := range simple.Collections {
		if c.UpdateReason == nil {
			t.Fatalf("Simple collection %s v%d has no UpdateReason", c.UUID, c.Version)
		}
		uuid, version, ok := ExtractCollectionMarker(c.UpdateReason.Comment)
		if !ok {
			t.Fatalf("Simple collection %s v%d: ExtractCollectionMarker(%q) failed", c.UUID, c.Version, c.UpdateReason.Comment)
		}
		if uuid != c.UUID || version != c.Version {
			t.Fatalf("Simple collection %s v%d: marker = %s v%d, want it to match its own owner/version", c.UUID, c.Version, uuid, version)
		}
	}

	complexManifest, _ := Complex()
	for _, c := range complexManifest.Collections {
		if c.UpdateReason == nil {
			t.Fatalf("Complex collection %s v%d has no UpdateReason", c.UUID, c.Version)
		}
		uuid, version, ok := ExtractCollectionMarker(c.UpdateReason.Comment)
		if !ok {
			t.Fatalf("Complex collection %s v%d: ExtractCollectionMarker(%q) failed", c.UUID, c.Version, c.UpdateReason.Comment)
		}
		if uuid != c.UUID || version != c.Version {
			t.Fatalf("Complex collection %s v%d: marker = %s v%d, want it to match its own owner/version", c.UUID, c.Version, uuid, version)
		}
	}

	lifecycle, _ := Lifecycle()
	for _, c := range lifecycle.Collections {
		if c.UpdateReason == nil {
			t.Fatalf("Lifecycle collection %s v%d has no UpdateReason", c.UUID, c.Version)
		}
		uuid, version, ok := ExtractCollectionMarker(c.UpdateReason.Comment)
		if !ok {
			t.Fatalf("Lifecycle collection %s v%d: ExtractCollectionMarker(%q) failed", c.UUID, c.Version, c.UpdateReason.Comment)
		}
		if uuid != c.UUID || version != c.Version {
			t.Fatalf("Lifecycle collection %s v%d: marker = %s v%d, want it to match its own owner/version", c.UUID, c.Version, uuid, version)
		}
	}
}

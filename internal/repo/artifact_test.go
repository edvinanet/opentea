// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"errors"
	"testing"
)

func TestArtifactCreateGetAndUploadFile(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	a, err := r.CreateArtifact(ctx, ArtifactInput{
		Name: "cyclonedx-sbom.json",
		Type: "BOM",
		Formats: []ArtifactFormatInput{
			{MediaType: "application/vnd.cyclonedx+json"},
		},
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	if a.Version != 1 {
		t.Fatalf("Version = %d, want 1", a.Version)
	}
	if len(a.Formats) != 1 {
		t.Fatalf("Formats = %+v", a.Formats)
	}

	updated, err := r.SetArtifactFormatFile(ctx, a.UUID, a.Version, 0, "deadbeef")
	if err != nil {
		t.Fatalf("SetArtifactFormatFile: %v", err)
	}
	// TEA 1.0 (spec/openapi.yaml): url is reserved for genuinely external
	// locations -- self-hosted content (this call) must leave it empty, not
	// populate it with this server's own link. Content-Location is what
	// the new download endpoints publish instead; url itself is presence-
	// tested here, not asserted as some particular self-referential value.
	if updated.Formats[0].URL != "" {
		t.Fatalf("URL = %q, want empty for self-hosted content", updated.Formats[0].URL)
	}
	if len(updated.Formats[0].Checksums) != 1 || updated.Formats[0].Checksums[0].AlgValue != "deadbeef" {
		t.Fatalf("Checksums = %+v", updated.Formats[0].Checksums)
	}

	latest, err := r.GetArtifactLatest(ctx, a.UUID)
	if err != nil {
		t.Fatalf("GetArtifactLatest: %v", err)
	}
	if latest.Formats[0].URL != "" {
		t.Fatalf("latest.Formats[0].URL = %q, want empty for self-hosted content", latest.Formats[0].URL)
	}

	byVersion, err := r.GetArtifactByVersion(ctx, a.UUID, 1)
	if err != nil {
		t.Fatalf("GetArtifactByVersion: %v", err)
	}
	if byVersion.UUID != a.UUID {
		t.Fatalf("UUID mismatch")
	}
}

// TestSetArtifactFormatFileReplacesNotAccumulates is the regression test
// for docs/security-review-publisher-design-260828.md finding 14's
// "whether upload creates or replaces content": a second
// SetArtifactFormatFile call for the same format previously left two
// checksum rows behind -- the stale one still pointing at now-orphaned
// blob content -- instead of cleanly replacing it. Confirms exactly one
// checksum row survives, with the second upload's value, after two calls.
func TestSetArtifactFormatFileReplacesNotAccumulates(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	a, err := r.CreateArtifact(ctx, ArtifactInput{
		Type:    "BOM",
		Formats: []ArtifactFormatInput{{MediaType: "application/vnd.cyclonedx+json"}},
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	if _, err := r.SetArtifactFormatFile(ctx, a.UUID, a.Version, 0, "first-checksum"); err != nil {
		t.Fatalf("first SetArtifactFormatFile: %v", err)
	}
	updated, err := r.SetArtifactFormatFile(ctx, a.UUID, a.Version, 0, "second-checksum")
	if err != nil {
		t.Fatalf("second SetArtifactFormatFile: %v", err)
	}

	if len(updated.Formats[0].Checksums) != 1 {
		t.Fatalf("Checksums = %+v, want exactly 1 (replaced, not accumulated)", updated.Formats[0].Checksums)
	}
	if updated.Formats[0].Checksums[0].AlgValue != "second-checksum" {
		t.Fatalf("Checksums[0].AlgValue = %q, want the second upload's value", updated.Formats[0].Checksums[0].AlgValue)
	}
}

func TestArtifactSetFileInvalidFormatIndex(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	a, err := r.CreateArtifact(ctx, ArtifactInput{Type: "BOM", Formats: []ArtifactFormatInput{{MediaType: "application/json"}}})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	if _, err := r.SetArtifactFormatFile(ctx, a.UUID, a.Version, 5, "hash"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestCreateArtifactVersion is the regression test for
// docs/security-review-publisher-design-260828.md finding 4: there was no
// way to add a new revision to an existing artifact uuid at all, only mint
// a brand new one. Confirms the new version is server-numbered
// (current-highest + 1), that nothing is carried forward from the prior
// version's fields (a completely different name/type/format set), and that
// both the new and the original version remain independently fetchable.
func TestCreateArtifactVersion(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	v1, err := r.CreateArtifact(ctx, ArtifactInput{
		Name:    "sbom-draft.json",
		Type:    "BOM",
		Formats: []ArtifactFormatInput{{MediaType: "application/vnd.cyclonedx+json"}},
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	v2, err := r.CreateArtifactVersion(ctx, v1.UUID, nil, ArtifactInput{
		Name:    "sbom-corrected.json",
		Type:    "BOM",
		Formats: []ArtifactFormatInput{{MediaType: "application/vnd.cyclonedx+xml"}},
	})
	if err != nil {
		t.Fatalf("CreateArtifactVersion: %v", err)
	}
	if v2.UUID != v1.UUID {
		t.Fatalf("UUID = %q, want the same identity as v1 (%q)", v2.UUID, v1.UUID)
	}
	if v2.Version != 2 {
		t.Fatalf("Version = %d, want 2", v2.Version)
	}
	if v2.Name != "sbom-corrected.json" || len(v2.Formats) != 1 || v2.Formats[0].MediaType != "application/vnd.cyclonedx+xml" {
		t.Fatalf("v2 = %+v, want fresh fields, nothing carried forward from v1", v2)
	}

	// v1 is untouched.
	stillV1, err := r.GetArtifactByVersion(ctx, v1.UUID, 1)
	if err != nil {
		t.Fatalf("GetArtifactByVersion(1): %v", err)
	}
	if stillV1.Name != "sbom-draft.json" {
		t.Fatalf("v1.Name = %q, want it unchanged by creating v2", stillV1.Name)
	}

	// A third version continues the sequence.
	v3, err := r.CreateArtifactVersion(ctx, v1.UUID, nil, ArtifactInput{
		Type:    "BOM",
		Formats: []ArtifactFormatInput{{MediaType: "application/vnd.cyclonedx+json"}},
	})
	if err != nil {
		t.Fatalf("CreateArtifactVersion (v3): %v", err)
	}
	if v3.Version != 3 {
		t.Fatalf("Version = %d, want 3", v3.Version)
	}
}

func TestCreateArtifactVersionUnknownUUID(t *testing.T) {
	r := newTestRepo(t)
	_, err := r.CreateArtifactVersion(context.Background(), "00000000-0000-4000-8000-000000000000", nil, ArtifactInput{
		Type:    "BOM",
		Formats: []ArtifactFormatInput{{MediaType: "application/json"}},
	})
	if err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestCreateArtifactVersionPreviousVersionMismatch is the regression test
// for the optimistic-concurrency half of finding 4: a caller supplying a
// stale previousVersion must be rejected, not silently land on an
// unexpected version number.
func TestCreateArtifactVersionPreviousVersionMismatch(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	a, err := r.CreateArtifact(ctx, ArtifactInput{Type: "BOM", Formats: []ArtifactFormatInput{{MediaType: "application/json"}}})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	stale := 5
	_, err = r.CreateArtifactVersion(ctx, a.UUID, &stale, ArtifactInput{Type: "BOM", Formats: []ArtifactFormatInput{{MediaType: "application/json"}}})
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("err = %v, want ErrVersionConflict", err)
	}

	// The correct previousVersion (1) succeeds.
	correct := 1
	v2, err := r.CreateArtifactVersion(ctx, a.UUID, &correct, ArtifactInput{Type: "BOM", Formats: []ArtifactFormatInput{{MediaType: "application/json"}}})
	if err != nil {
		t.Fatalf("CreateArtifactVersion with correct previousVersion: %v", err)
	}
	if v2.Version != 2 {
		t.Fatalf("Version = %d, want 2", v2.Version)
	}
}

func TestGetArtifactLatestNotFound(t *testing.T) {
	r := newTestRepo(t)
	_, err := r.GetArtifactLatest(context.Background(), "00000000-0000-4000-8000-000000000000")
	if err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestArtifactRevisionStartsAtOneAndBumpsOnFileUpload is the regression
// test for artifact ETag support: a fresh artifact starts at revision 1
// (matching the schema's DEFAULT), and SetArtifactFormatFile -- the one
// mutation path that changes an artifact's representation after creation
// (the create-then-upload flow) -- bumps it by exactly 1, not left
// unchanged (which would make a client's cached ETag wrongly still match
// after the URL/checksum changed).
func TestArtifactRevisionStartsAtOneAndBumpsOnFileUpload(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	a, err := r.CreateArtifact(ctx, ArtifactInput{Type: "BOM", Formats: []ArtifactFormatInput{{MediaType: "application/json"}}})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	rev, err := r.GetArtifactRevision(ctx, a.UUID, a.Version)
	if err != nil {
		t.Fatalf("GetArtifactRevision: %v", err)
	}
	if rev != 1 {
		t.Fatalf("revision = %d, want 1 for a freshly created artifact", rev)
	}

	if _, err := r.SetArtifactFormatFile(ctx, a.UUID, a.Version, 0, "deadbeef"); err != nil {
		t.Fatalf("SetArtifactFormatFile: %v", err)
	}
	rev, err = r.GetArtifactRevision(ctx, a.UUID, a.Version)
	if err != nil {
		t.Fatalf("GetArtifactRevision after upload: %v", err)
	}
	if rev != 2 {
		t.Fatalf("revision after SetArtifactFormatFile = %d, want 2", rev)
	}

	// A second upload (a different format, say) bumps it again -- not
	// reset, not left alone.
	if _, err := r.SetArtifactFormatFile(ctx, a.UUID, a.Version, 0, "othersum"); err != nil {
		t.Fatalf("SetArtifactFormatFile (2nd): %v", err)
	}
	rev, err = r.GetArtifactRevision(ctx, a.UUID, a.Version)
	if err != nil {
		t.Fatalf("GetArtifactRevision after 2nd upload: %v", err)
	}
	if rev != 3 {
		t.Fatalf("revision after 2nd SetArtifactFormatFile = %d, want 3", rev)
	}
}

func TestGetArtifactRevisionNotFound(t *testing.T) {
	r := newTestRepo(t)
	if _, err := r.GetArtifactRevision(context.Background(), "00000000-0000-4000-8000-000000000000", 1); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestGetArtifactFormatForDownload covers GetArtifactFormatForDownload's
// full outcome matrix: self-hosted (sha256Hex set), external (externalURL
// set, case-insensitive mediaType match), no matching format at all
// (ErrNoMatchingFormat), and a matched-but-not-yet-uploaded format
// (ErrNotFound -- the documented asymmetry vs. signatures).
func TestGetArtifactFormatForDownload(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	a, err := r.CreateArtifact(ctx, ArtifactInput{Type: "BOM", Formats: []ArtifactFormatInput{
		{MediaType: "application/vnd.cyclonedx+json"},
	}})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	if _, err := r.SetArtifactFormatFile(ctx, a.UUID, a.Version, 0, "deadbeef"); err != nil {
		t.Fatalf("SetArtifactFormatFile: %v", err)
	}

	// External-url formats aren't reachable through the normal
	// create-then-upload flow at all (ArtifactFormatInput has no URL field
	// -- only ImportArtifact, used by bundle import, can set one), so build
	// a second artifact revision via ImportArtifact to cover that branch.
	if _, err := r.ImportArtifact(ctx, ImportArtifactInput{
		UUID: a.UUID, Version: 2, Type: "BOM",
		Formats: []ImportArtifactFormatInput{
			{MediaType: "application/spdx+json", URL: "https://example.com/external.json"},
		},
	}); err != nil {
		t.Fatalf("ImportArtifact: %v", err)
	}

	sha256Hex, externalURL, err := r.GetArtifactFormatForDownload(ctx, a.UUID, a.Version, "APPLICATION/VND.CYCLONEDX+JSON")
	if err != nil {
		t.Fatalf("self-hosted lookup (case-insensitive): %v", err)
	}
	if sha256Hex != "deadbeef" || externalURL != "" {
		t.Fatalf("self-hosted lookup = (%q, %q), want (deadbeef, \"\")", sha256Hex, externalURL)
	}

	sha256Hex, externalURL, err = r.GetArtifactFormatForDownload(ctx, a.UUID, 2, "application/spdx+json")
	if err != nil {
		t.Fatalf("external lookup: %v", err)
	}
	if externalURL != "https://example.com/external.json" || sha256Hex != "" {
		t.Fatalf("external lookup = (%q, %q), want (\"\", https://example.com/external.json)", sha256Hex, externalURL)
	}

	if _, _, err := r.GetArtifactFormatForDownload(ctx, a.UUID, a.Version, "application/does-not-exist"); err != ErrNoMatchingFormat {
		t.Fatalf("no matching format: err = %v, want ErrNoMatchingFormat", err)
	}

	a2, err := r.CreateArtifact(ctx, ArtifactInput{Type: "BOM", Formats: []ArtifactFormatInput{{MediaType: "application/json"}}})
	if err != nil {
		t.Fatalf("CreateArtifact (2nd): %v", err)
	}
	if _, _, err := r.GetArtifactFormatForDownload(ctx, a2.UUID, a2.Version, "application/json"); err != ErrNotFound {
		t.Fatalf("matched but not yet uploaded: err = %v, want ErrNotFound", err)
	}

	if _, _, err := r.GetArtifactFormatForDownload(ctx, "00000000-0000-4000-8000-000000000000", 1, "application/json"); err != ErrNotFound {
		t.Fatalf("nonexistent artifact: err = %v, want ErrNotFound", err)
	}
}

// TestGetArtifactFormatForSignatureDownload covers the SIGNATURE_NOT_FOUND/
// ErrNoMatchingFormat distinction: a real format with no signature at all
// is ErrSignatureNotFound (spec-defined, not existence-hiding), while a
// mediaType matching no format is ErrNoMatchingFormat, same as content.
func TestGetArtifactFormatForSignatureDownload(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	a, err := r.CreateArtifact(ctx, ArtifactInput{Type: "BOM", Formats: []ArtifactFormatInput{
		{MediaType: "application/vnd.cyclonedx+json"},
	}})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	if _, _, err := r.GetArtifactFormatForSignatureDownload(ctx, a.UUID, a.Version, "application/vnd.cyclonedx+json"); err != ErrSignatureNotFound {
		t.Fatalf("no signature published: err = %v, want ErrSignatureNotFound", err)
	}
	if _, _, err := r.GetArtifactFormatForSignatureDownload(ctx, a.UUID, a.Version, "application/does-not-exist"); err != ErrNoMatchingFormat {
		t.Fatalf("no matching format: err = %v, want ErrNoMatchingFormat", err)
	}

	if _, err := r.SetArtifactFormatSignatureFile(ctx, a.UUID, a.Version, 0, "sigsha256"); err != nil {
		t.Fatalf("SetArtifactFormatSignatureFile: %v", err)
	}
	sha256Hex, externalURL, err := r.GetArtifactFormatForSignatureDownload(ctx, a.UUID, a.Version, "application/vnd.cyclonedx+json")
	if err != nil {
		t.Fatalf("self-hosted signature lookup: %v", err)
	}
	if sha256Hex != "sigsha256" || externalURL != "" {
		t.Fatalf("self-hosted signature lookup = (%q, %q), want (sigsha256, \"\")", sha256Hex, externalURL)
	}
}

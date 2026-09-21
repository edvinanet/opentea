// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
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

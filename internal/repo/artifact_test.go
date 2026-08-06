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

	updated, err := r.SetArtifactFormatFile(ctx, a.UUID, a.Version, 0, "http://localhost/files/deadbeef", "deadbeef")
	if err != nil {
		t.Fatalf("SetArtifactFormatFile: %v", err)
	}
	if updated.Formats[0].URL != "http://localhost/files/deadbeef" {
		t.Fatalf("URL = %q", updated.Formats[0].URL)
	}
	if len(updated.Formats[0].Checksums) != 1 || updated.Formats[0].Checksums[0].AlgValue != "deadbeef" {
		t.Fatalf("Checksums = %+v", updated.Formats[0].Checksums)
	}

	latest, err := r.GetArtifactLatest(ctx, a.UUID)
	if err != nil {
		t.Fatalf("GetArtifactLatest: %v", err)
	}
	if latest.Formats[0].URL != "http://localhost/files/deadbeef" {
		t.Fatalf("latest.Formats[0].URL = %q", latest.Formats[0].URL)
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
	if _, err := r.SetArtifactFormatFile(ctx, a.UUID, a.Version, 5, "http://x", "hash"); err != ErrNotFound {
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

	if _, err := r.SetArtifactFormatFile(ctx, a.UUID, a.Version, 0, "http://localhost/files/deadbeef", "deadbeef"); err != nil {
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
	if _, err := r.SetArtifactFormatFile(ctx, a.UUID, a.Version, 0, "http://localhost/files/othersum", "othersum"); err != nil {
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

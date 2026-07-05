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

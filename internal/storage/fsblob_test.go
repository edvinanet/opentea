package storage

import (
	"bytes"
	"context"
	"io"
	"testing"
)

func TestFSStoragePutOpenDelete(t *testing.T) {
	s, err := NewFSStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	ctx := context.Background()

	content := []byte("hello, tea")
	sum, size, err := s.Put(ctx, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if size != int64(len(content)) {
		t.Fatalf("size = %d, want %d", size, len(content))
	}
	const wantSum = "d2dcfdfce3296e56b200d5a342c5dfa30dcc9e1ef8eacfd3cd1af5bca77bb586"
	if sum != wantSum {
		t.Fatalf("sha256 = %s, want %s", sum, wantSum)
	}

	rc, err := s.Open(ctx, sum)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("got %q, want %q", got, content)
	}

	if err := s.Delete(ctx, sum); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Open(ctx, sum); err == nil {
		t.Fatal("expected error opening deleted blob")
	}

	// Deleting a non-existent blob is not an error.
	if err := s.Delete(ctx, sum); err != nil {
		t.Fatalf("Delete (already gone): %v", err)
	}
}

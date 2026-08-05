package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

// FSStorage is a Storage implementation backed by a local filesystem
// directory, laid out as {dir}/{sha256[:2]}/{sha256} to avoid huge flat
// directories.
type FSStorage struct {
	dir string
}

// NewFSStorage creates the storage directory (if needed) and returns a
// Storage backed by it.
func NewFSStorage(dir string) (*FSStorage, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create blob dir: %w", err)
	}
	return &FSStorage{dir: dir}, nil
}

// sha256Pattern matches the only well-formed shape a blob's sha256 can be.
var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// pathFor validates sha256Hex before building the on-disk path for it.
// Every caller-supplied sha256 (Open, Delete) must go through this, not
// straight to filepath.Join -- callers don't necessarily control where
// their sha256 came from originally. In particular, internal/bundle's
// Export reads SHA-256 checksum values straight out of the DB to decide
// what to open, and those values can originate from a bundle *import*'s
// manifest -- attacker-controlled if importing from an untrusted TEA
// server, and never validated to actually correspond to an uploaded blob
// (a checksum with no matching blob is a legitimate "reference-only"
// import, so import can't reject it). Without this check, a crafted
// checksum value like "../../../../etc/passwd" stored via import would
// later be read and included in a re-export's zip -- an information leak,
// not just an availability failure -- rather than failing cleanly here.
func (s *FSStorage) pathFor(sha256Hex string) (string, error) {
	if !sha256Pattern.MatchString(sha256Hex) {
		return "", fmt.Errorf("storage: invalid sha256 %q", sha256Hex)
	}
	return filepath.Join(s.dir, sha256Hex[:2], sha256Hex), nil
}

// Put implements Storage by writing to a temp file first, then renaming it
// into place under its computed hash once fully written -- so a reader
// never observes a partially-written blob.
func (s *FSStorage) Put(ctx context.Context, r io.Reader) (string, int64, error) {
	tmp, err := os.CreateTemp(s.dir, "upload-*")
	if err != nil {
		return "", 0, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // no-op once successfully renamed

	h := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, h), r)
	closeErr := tmp.Close()
	if err != nil {
		return "", 0, fmt.Errorf("write blob: %w", err)
	}
	if closeErr != nil {
		return "", 0, fmt.Errorf("close temp file: %w", closeErr)
	}

	sum := hex.EncodeToString(h.Sum(nil))
	finalPath, err := s.pathFor(sum) // sum is our own sha256.New() output, so this can't actually fail
	if err != nil {
		return "", 0, err
	}
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o750); err != nil {
		return "", 0, fmt.Errorf("create blob subdir: %w", err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return "", 0, fmt.Errorf("finalize blob: %w", err)
	}
	return sum, size, nil
}

// Open implements Storage.
func (s *FSStorage) Open(ctx context.Context, sha256Hex string) (io.ReadCloser, error) {
	path, err := s.pathFor(sha256Hex)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path) //nolint:gosec // path is built from pathFor, which rejects anything but a well-formed sha256 before this point
	if err != nil {
		return nil, err
	}
	return f, nil
}

// Delete implements Storage.
func (s *FSStorage) Delete(ctx context.Context, sha256Hex string) error {
	path, err := s.pathFor(sha256Hex)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

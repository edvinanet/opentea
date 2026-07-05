package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// FSStorage is a Storage implementation backed by a local filesystem
// directory, laid out as {dir}/{sha256[:2]}/{sha256} to avoid huge flat
// directories.
type FSStorage struct {
	dir string
}

func NewFSStorage(dir string) (*FSStorage, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create blob dir: %w", err)
	}
	return &FSStorage{dir: dir}, nil
}

func (s *FSStorage) pathFor(sha256Hex string) string {
	return filepath.Join(s.dir, sha256Hex[:2], sha256Hex)
}

func (s *FSStorage) Put(ctx context.Context, r io.Reader) (string, int64, error) {
	tmp, err := os.CreateTemp(s.dir, "upload-*")
	if err != nil {
		return "", 0, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once successfully renamed

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
	finalPath := s.pathFor(sum)
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return "", 0, fmt.Errorf("create blob subdir: %w", err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return "", 0, fmt.Errorf("finalize blob: %w", err)
	}
	return sum, size, nil
}

func (s *FSStorage) Open(ctx context.Context, sha256Hex string) (io.ReadCloser, error) {
	f, err := os.Open(s.pathFor(sha256Hex))
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (s *FSStorage) Delete(ctx context.Context, sha256Hex string) error {
	err := os.Remove(s.pathFor(sha256Hex))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

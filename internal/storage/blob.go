// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Package storage provides content-addressed blob storage for artifact and
// distribution files, behind a small interface so the backing store can be
// swapped (e.g. for S3) without touching callers.
package storage

import (
	"context"
	"io"
)

// Storage stores and retrieves binary blobs addressed by their SHA-256 hash.
type Storage interface {
	// Put reads r fully, stores it, and returns its SHA-256 hash (hex) and size.
	Put(ctx context.Context, r io.Reader) (sha256 string, size int64, err error)
	// Open returns a reader for the blob with the given SHA-256 hash.
	Open(ctx context.Context, sha256 string) (io.ReadCloser, error)
	// Delete removes the blob with the given SHA-256 hash.
	Delete(ctx context.Context, sha256 string) error
}

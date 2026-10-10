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

// UnsafePutter is an optional capability a Storage backend may implement
// (checked via a type assertion, not part of the Storage interface itself
// -- every normal caller keeps using Put) for internal/bundle.ImportUnchecked's
// force-import path only (docs/bundle-format.md's "Force-import" section).
// Unlike Put, which always computes and returns the real hash of what it
// stored, PutUnchecked stores r's bytes under the caller's own claimed
// sha256Hex without verifying it against the content at all -- so a
// caller can deliberately create a blob whose declared checksum doesn't
// match its own bytes, the exact shape of corruption
// docs/consumer-api-conformance-test-rig.md's client-side suite needs to
// prove it detects. Never call this outside that gated, test-tooling-only
// path: it is the one place in this package that doesn't enforce its own
// content-addressing guarantee.
type UnsafePutter interface {
	PutUnchecked(ctx context.Context, sha256Hex string, r io.Reader) (size int64, err error)
}

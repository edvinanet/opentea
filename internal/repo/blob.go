// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"database/sql"
	"errors"
)

// UpsertBlob records bookkeeping for an uploaded file (used by the /files/
// handler to serve the right Content-Type). Blobs are content-addressed, so
// re-uploading identical bytes is a no-op on conflict.
func (r *Repo) UpsertBlob(ctx context.Context, sha256Hex string, sizeBytes int64, mediaType string) error {
	_, err := r.conn().ExecContext(ctx,
		`INSERT INTO blob (sha256, size_bytes, media_type) VALUES (?, ?, ?) ON CONFLICT (sha256) DO NOTHING`,
		sha256Hex, sizeBytes, nullIfEmpty(mediaType),
	)
	return err
}

// GetBlobMediaType returns the recorded Content-Type for a stored blob.
// Returns ErrNotFound if sha256Hex has no bookkeeping row.
func (r *Repo) GetBlobMediaType(ctx context.Context, sha256Hex string) (string, error) {
	var mediaType sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT media_type FROM blob WHERE sha256 = ?`, sha256Hex).Scan(&mediaType)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return mediaType.String, nil
}

// BlobArtifactOwner identifies one artifact whose content or detached
// signature references a stored blob -- see FindBlobArtifactOwners.
type BlobArtifactOwner struct {
	ArtifactUUID string
	ArtifactType string
}

// FindBlobArtifactOwners returns every artifact (deduplicated by UUID and
// type, across every version) whose content or detached signature
// references sha256Hex. Used by internal/files to authorize a
// /files/{sha256} request through the same artifact.download capability
// the versioned download endpoints enforce (internal/api/artifactdownload.go)
// -- a blob has no identity of its own to check authz against, so it's
// reached through its owning resources instead. The same bytes can be
// referenced by more than one artifact or revision (content reuse across
// formats), so this can return more than one owner; the caller is expected
// to permit the request if authorized for any one of them, mirroring the
// "shared artifact reachability via any one authorized relationship"
// principle spec Sec 17.2 already applies to collections.
func (r *Repo) FindBlobArtifactOwners(ctx context.Context, sha256Hex string) ([]BlobArtifactOwner, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT af.artifact_uuid, a.type
		FROM checksum c
		JOIN artifact_format af ON af.id = c.owner_id
		JOIN artifact a ON a.uuid = af.artifact_uuid AND a.version = af.artifact_version
		WHERE c.owner_type = 'ARTIFACT_FORMAT' AND c.alg_type = 'SHA-256' AND c.alg_value = ?
		UNION
		SELECT DISTINCT af.artifact_uuid, a.type
		FROM artifact_format af
		JOIN artifact a ON a.uuid = af.artifact_uuid AND a.version = af.artifact_version
		WHERE af.signature_sha256 = ?
	`, sha256Hex, sha256Hex)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var owners []BlobArtifactOwner
	for rows.Next() {
		var o BlobArtifactOwner
		if err := rows.Scan(&o.ArtifactUUID, &o.ArtifactType); err != nil {
			return nil, err
		}
		owners = append(owners, o)
	}
	return owners, rows.Err()
}

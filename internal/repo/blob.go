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
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO blob (sha256, size_bytes, media_type) VALUES (?, ?, ?) ON CONFLICT (sha256) DO NOTHING`,
		sha256Hex, sizeBytes, nullIfEmpty(mediaType),
	)
	return err
}

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

package repo

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/oej/opentea/internal/model"
)

// SetAPIToken generates a new API token for userUUID, replacing any existing
// one, and returns the raw token -- this is the only time the raw value is
// ever available; only its SHA-256 hash is stored (a token has enough
// entropy that a fast hash is fine here, unlike passwords).
func (r *Repo) SetAPIToken(ctx context.Context, userUUID string) (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	hash := hashToken(token)

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO api_token (user_uuid, token_hash, created_at) VALUES (?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		 ON CONFLICT (user_uuid) DO UPDATE SET token_hash = excluded.token_hash, created_at = excluded.created_at`,
		userUUID, hash,
	)
	if err != nil {
		return "", err
	}
	return token, nil
}

// GetAPITokenCreatedAt reports whether userUUID has an API token and, if so,
// when it was created -- for GUI display, without ever revealing the token
// itself. Returns (nil, nil) if no token exists.
func (r *Repo) GetAPITokenCreatedAt(ctx context.Context, userUUID string) (*time.Time, error) {
	var createdAt string
	err := r.db.QueryRowContext(ctx, `SELECT created_at FROM api_token WHERE user_uuid = ?`, userUUID).Scan(&createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t, err := parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetUserByAPIToken resolves a raw bearer token to its owning user.
func (r *Repo) GetUserByAPIToken(ctx context.Context, token string) (model.User, error) {
	hash := hashToken(token)

	var u model.User
	var createdAt string
	err := r.db.QueryRowContext(ctx,
		`SELECT u.uuid, u.username, u.role, u.created_at
		 FROM api_token t JOIN user u ON u.uuid = t.user_uuid
		 WHERE t.token_hash = ?`, hash,
	).Scan(&u.UUID, &u.Username, &u.Role, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.User{}, ErrNotFound
	}
	if err != nil {
		return model.User{}, err
	}
	created, err := parseTime(createdAt)
	if err != nil {
		return model.User{}, err
	}
	u.CreatedAt = created
	return u, nil
}

// hashToken hashes a raw, high-entropy random token (an API token or a
// session token -- see session.go) for storage/lookup: a fast, unsalted
// SHA-256 is fine here, unlike for passwords, since a 256-bit random token
// isn't brute-forceable regardless of hash speed. Used instead of storing
// the raw value directly so a leaked/read DB file doesn't hand out
// directly-usable tokens.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

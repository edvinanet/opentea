// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

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

	"github.com/oej/opentea/internal/idgen"
	"github.com/oej/opentea/internal/model"
)

// SetAPIKey generates a new API key (an identifier + secret pair, per TEA
// 1.0's auth/readme.md) for userUUID, replacing any existing one, and
// returns the raw keyID/secret -- this is the only time the raw secret is
// ever available; only its SHA-256 hash is stored. keyID is not
// confidential (safe to display indefinitely, e.g. for GUI identification)
// -- only secret is.
func (r *Repo) SetAPIKey(ctx context.Context, userUUID string) (keyID, secret string, err error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", err
	}
	keyID = idgen.New()
	secret = base64.RawURLEncoding.EncodeToString(raw[:])
	hash := hashToken(secret)

	_, err = r.db.ExecContext(ctx,
		`INSERT INTO api_key (user_uuid, key_id, secret_hash, created_at) VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		 ON CONFLICT (user_uuid) DO UPDATE SET key_id = excluded.key_id, secret_hash = excluded.secret_hash, created_at = excluded.created_at`,
		userUUID, keyID, hash,
	)
	if err != nil {
		return "", "", err
	}
	return keyID, secret, nil
}

// GetAPIKeyCreatedAt reports whether userUUID has an API key and, if so,
// when it was created -- for GUI display, without ever revealing the
// secret. Returns (nil, nil) if no key exists.
func (r *Repo) GetAPIKeyCreatedAt(ctx context.Context, userUUID string) (*time.Time, error) {
	var createdAt string
	err := r.db.QueryRowContext(ctx, `SELECT created_at FROM api_key WHERE user_uuid = ?`, userUUID).Scan(&createdAt)
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

// VerifyAPIKey resolves a (keyID, secret) pair presented to POST /token
// (internal/api/token.go) to its owning user. Returns ErrNotFound for
// both an unknown keyID and a keyID whose secret doesn't match --
// indistinguishable to the caller, same principle
// GetPublisherCredentialByToken applies to revoked-vs-unknown.
func (r *Repo) VerifyAPIKey(ctx context.Context, keyID, secret string) (model.User, error) {
	hash := hashToken(secret)

	var u model.User
	var createdAt string
	err := r.db.QueryRowContext(ctx,
		`SELECT u.uuid, u.username, u.role, u.created_at
		 FROM api_key k JOIN user u ON u.uuid = k.user_uuid
		 WHERE k.key_id = ? AND k.secret_hash = ?`, keyID, hash,
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

// hashToken hashes a raw, high-entropy random token (an API key secret,
// access token, or session token -- see session.go/token.go) for
// storage/lookup: a fast, unsalted SHA-256 is fine here, unlike for
// passwords, since a 256-bit random token isn't brute-forceable regardless
// of hash speed. Used instead of storing the raw value directly so a
// leaked/read DB file doesn't hand out directly-usable credentials.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

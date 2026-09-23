// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"

	"github.com/oej/opentea/internal/model"
)

// CreateAccessToken creates a new TEA access token for userUUID, valid for
// ttl from now, and returns the opaque bearer value -- the only credential
// /tea/v1 accepts as Authorization: Bearer (TEA 1.0's /token exchange,
// internal/api/token.go). This is the only time the raw value is ever
// available; only its hash is stored (see hashToken), same principle as
// CreateSession/SetAPIKey.
func (r *Repo) CreateAccessToken(ctx context.Context, userUUID string, ttl time.Duration) (token string, expiresAt time.Time, err error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", time.Time{}, err
	}
	token = base64.RawURLEncoding.EncodeToString(raw[:])
	expiresAt = time.Now().Add(ttl)

	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO access_token (token_hash, user_uuid, expires_at) VALUES (?, ?, ?)`,
		hashToken(token), userUUID, formatTime(expiresAt),
	); err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

// GetAccessTokenUser resolves an unexpired access token to its user --
// internal/authn.BearerUser calls this for every /tea/v1 request carrying
// an Authorization: Bearer header. Returns ErrNotFound for an unknown or
// expired token.
func (r *Repo) GetAccessTokenUser(ctx context.Context, token string) (model.User, error) {
	var u model.User
	var expiresAt, createdAt string
	err := r.db.QueryRowContext(ctx,
		`SELECT u.uuid, u.username, u.role, u.created_at, a.expires_at
		 FROM access_token a JOIN user u ON u.uuid = a.user_uuid
		 WHERE a.token_hash = ?`, hashToken(token),
	).Scan(&u.UUID, &u.Username, &u.Role, &createdAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.User{}, ErrNotFound
	}
	if err != nil {
		return model.User{}, err
	}

	exp, err := parseTime(expiresAt)
	if err != nil {
		return model.User{}, err
	}
	if time.Now().After(exp) {
		return model.User{}, ErrNotFound
	}

	created, err := parseTime(createdAt)
	if err != nil {
		return model.User{}, err
	}
	u.CreatedAt = created
	return u, nil
}

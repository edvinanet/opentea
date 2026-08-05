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

// CreateSession creates a new session for userUUID, valid for ttl from now,
// and returns the opaque token to be stored in the client's cookie.
func (r *Repo) CreateSession(ctx context.Context, userUUID string, ttl time.Duration) (token string, expiresAt time.Time, err error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", time.Time{}, err
	}
	token = base64.RawURLEncoding.EncodeToString(raw[:])
	expiresAt = time.Now().Add(ttl)

	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO session (token, user_uuid, expires_at) VALUES (?, ?, ?)`,
		token, userUUID, formatTime(expiresAt),
	); err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

// GetSessionUser resolves an unexpired session token to its user.
func (r *Repo) GetSessionUser(ctx context.Context, token string) (model.User, error) {
	var u model.User
	var expiresAt, createdAt string
	err := r.db.QueryRowContext(ctx,
		`SELECT u.uuid, u.username, u.role, u.created_at, s.expires_at
		 FROM session s JOIN user u ON u.uuid = s.user_uuid
		 WHERE s.token = ?`, token,
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

// DeleteSession invalidates the session identified by token (used on logout).
func (r *Repo) DeleteSession(ctx context.Context, token string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM session WHERE token = ?`, token)
	return err
}

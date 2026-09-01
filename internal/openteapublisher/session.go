// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"
)

// SessionTTL is how long a session stays valid after login -- matches
// internal/authn.SessionTTL's own value (fixed, not renewed on activity).
const SessionTTL = 24 * time.Hour

// CreateSession creates a new session for staffUUID, valid for SessionTTL
// from now, and returns the opaque token to be stored in the client's
// cookie -- this is the only time the raw value is ever available; only
// its hash is stored (see hashToken), matching internal/repo.CreateSession's
// own contract, so a leaked/read DB file doesn't hand out directly-usable
// sessions.
func (r *Repo) CreateSession(ctx context.Context, staffUUID string) (token string, expiresAt time.Time, err error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", time.Time{}, err
	}
	token = base64.RawURLEncoding.EncodeToString(raw[:])
	expiresAt = time.Now().Add(SessionTTL)

	if _, err := r.conn().ExecContext(ctx,
		`INSERT INTO session (token_hash, staff_uuid, expires_at) VALUES (?, ?, ?)`,
		hashToken(token), staffUUID, formatTime(expiresAt),
	); err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

// GetSessionStaff resolves an unexpired session token to its staff account.
func (r *Repo) GetSessionStaff(ctx context.Context, token string) (Staff, error) {
	var s Staff
	var expiresAt, createdAt string
	var workflowRole sql.NullString
	err := r.conn().QueryRowContext(ctx,
		`SELECT st.uuid, st.username, st.role, st.workflow_role, st.created_at, se.expires_at
		 FROM session se JOIN staff st ON st.uuid = se.staff_uuid
		 WHERE se.token_hash = ?`, hashToken(token),
	).Scan(&s.UUID, &s.Username, &s.Role, &workflowRole, &createdAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Staff{}, ErrNotFound
	}
	if err != nil {
		return Staff{}, err
	}
	s.WorkflowRole = workflowRole.String

	exp, err := parseTime(expiresAt)
	if err != nil {
		return Staff{}, err
	}
	if time.Now().After(exp) {
		return Staff{}, ErrNotFound
	}

	created, err := parseTime(createdAt)
	if err != nil {
		return Staff{}, err
	}
	s.CreatedAt = created
	return s, nil
}

// DeleteSession invalidates the session identified by token (used on logout).
func (r *Repo) DeleteSession(ctx context.Context, token string) error {
	_, err := r.conn().ExecContext(ctx, `DELETE FROM session WHERE token_hash = ?`, hashToken(token))
	return err
}

// hashToken hashes a raw, high-entropy random token for storage/lookup --
// matches internal/repo's own hashToken exactly (a fast, unsalted SHA-256
// is fine for a 256-bit random token, unlike for passwords).
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/oej/opentea/internal/idgen"
)

// minPasswordLength/bcryptCost/dummyBcryptHash mirror internal/repo/user.go's
// own choices exactly -- see there for the NIST SP 800-63B and timing-safe
// rationale, unchanged here.
const minPasswordLength = 8
const bcryptCost = 12

var dummyBcryptHash = mustGenerateDummyHash()

func mustGenerateDummyHash() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte("openteapublisher-dummy-password-never-used-for-login"), bcryptCost)
	if err != nil {
		panic("openteapublisher: failed to generate dummy bcrypt hash: " + err.Error())
	}
	return hash
}

var (
	// ErrInvalidCredentials is returned by VerifyLogin on any mismatch --
	// deliberately the same error whether the username or the password was
	// wrong, so callers can't use it to enumerate valid usernames.
	ErrInvalidCredentials = errors.New("openteapublisher: invalid credentials")
	// ErrUsernameTaken is returned by CreateStaff when the username is already in use.
	ErrUsernameTaken = errors.New("openteapublisher: username already taken")
	// ErrPasswordTooShort is returned by CreateStaff when the password is
	// shorter than minPasswordLength.
	ErrPasswordTooShort = fmt.Errorf("openteapublisher: password must be at least %d characters", minPasswordLength)
)

// Staff is one manufacturer staff account (Layer A,
// design/publisher-service.md §10.1). No role -- every logged-in staff
// member has equal access in this phase (see repo.go's package doc comment).
type Staff struct {
	UUID      string
	Username  string
	CreatedAt time.Time
}

// CreateStaff creates a new staff account with a bcrypt-hashed password.
// Returns ErrPasswordTooShort if plaintextPassword is shorter than
// minPasswordLength, or ErrUsernameTaken if username is already in use.
func (r *Repo) CreateStaff(ctx context.Context, username, plaintextPassword string) (Staff, error) {
	if len(plaintextPassword) < minPasswordLength {
		return Staff{}, ErrPasswordTooShort
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(plaintextPassword), bcryptCost)
	if err != nil {
		return Staff{}, err
	}

	uuid := idgen.New()
	_, err = r.conn().ExecContext(ctx,
		`INSERT INTO staff (uuid, username, password_hash) VALUES (?, ?, ?)`,
		uuid, username, string(hash),
	)
	if isUniqueConstraintError(err) {
		return Staff{}, ErrUsernameTaken
	}
	if err != nil {
		return Staff{}, err
	}
	return r.GetStaffByUUID(ctx, uuid)
}

// GetStaffByUUID fetches a staff account by UUID. Returns ErrNotFound if
// uuid doesn't exist.
func (r *Repo) GetStaffByUUID(ctx context.Context, uuid string) (Staff, error) {
	var s Staff
	var createdAt string
	err := r.conn().QueryRowContext(ctx,
		`SELECT uuid, username, created_at FROM staff WHERE uuid = ?`, uuid,
	).Scan(&s.UUID, &s.Username, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Staff{}, ErrNotFound
	}
	if err != nil {
		return Staff{}, err
	}
	created, err := parseTime(createdAt)
	if err != nil {
		return Staff{}, err
	}
	s.CreatedAt = created
	return s, nil
}

// VerifyLogin checks username/password and returns the staff account on
// success. Returns ErrInvalidCredentials (never ErrNotFound) on any
// failure, so callers can't distinguish "no such account" from "wrong
// password".
func (r *Repo) VerifyLogin(ctx context.Context, username, plaintextPassword string) (Staff, error) {
	var s Staff
	var passwordHash, createdAt string
	err := r.conn().QueryRowContext(ctx,
		`SELECT uuid, username, password_hash, created_at FROM staff WHERE username = ?`, username,
	).Scan(&s.UUID, &s.Username, &passwordHash, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword(dummyBcryptHash, []byte(plaintextPassword))
		return Staff{}, ErrInvalidCredentials
	}
	if err != nil {
		return Staff{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(plaintextPassword)) != nil {
		return Staff{}, ErrInvalidCredentials
	}
	created, err := parseTime(createdAt)
	if err != nil {
		return Staff{}, err
	}
	s.CreatedAt = created
	return s, nil
}

// isUniqueConstraintError reports whether err came from a SQLite UNIQUE
// constraint violation -- matches internal/repo's own string-matching
// approach (modernc.org/sqlite exposes no typed constraint-kind API worth
// depending on).
func isUniqueConstraintError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

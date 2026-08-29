// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/oej/opentea/internal/idgen"
	"github.com/oej/opentea/internal/model"
)

// minPasswordLength matches NIST SP 800-63B's minimum for user-chosen
// passwords -- deliberately just a length floor, not a composition rule
// (required uppercase/digit/symbol mixes are explicitly discouraged by the
// same guidance; they push users toward predictable substitutions without
// meaningfully increasing entropy).
const minPasswordLength = 8

// bcryptCost is pinned above bcrypt.DefaultCost (10) -- 12 is current
// common guidance for admin-facing accounts, and pinning it explicitly
// means this doesn't silently change if the library's own default ever
// does. Existing hashes keep working regardless: bcrypt encodes its cost
// in the hash string itself, so bcrypt.CompareHashAndPassword reads each
// hash's own cost rather than this constant -- raising it only affects
// newly created hashes, no migration needed.
const bcryptCost = 12

// dummyBcryptHash is a valid bcrypt hash (at bcryptCost, matching every
// real user's) of a fixed, unused, non-secret plaintext -- VerifyLogin
// compares against this on an unknown-username attempt so that path costs
// the same as a known-username/wrong-password attempt (which always runs a
// real bcrypt.CompareHashAndPassword). Without this, an unknown username
// returns immediately after a single failed SELECT while a known one pays
// bcrypt's ~100ms, a remotely observable timing oracle for username
// enumeration despite both paths returning the identical
// ErrInvalidCredentials response body.
var dummyBcryptHash = mustGenerateDummyHash()

func mustGenerateDummyHash() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte("opentea-dummy-password-never-used-for-login"), bcryptCost)
	if err != nil {
		panic("repo: failed to generate dummy bcrypt hash: " + err.Error())
	}
	return hash
}

var (
	// ErrInvalidCredentials is returned by VerifyLogin on any mismatch --
	// deliberately the same error whether the username or the password was
	// wrong, so callers can't use it to enumerate valid usernames.
	ErrInvalidCredentials = errors.New("repo: invalid credentials")
	// ErrUsernameTaken is returned by CreateUser when the username is already in use.
	ErrUsernameTaken = errors.New("repo: username already taken")
	// ErrPasswordTooShort is returned by CreateUser when the password is
	// shorter than minPasswordLength.
	ErrPasswordTooShort = fmt.Errorf("repo: password must be at least %d characters", minPasswordLength)
	// ErrLastAdmin is returned by DeleteUser when deleting would leave no admin users.
	ErrLastAdmin = errors.New("repo: cannot delete the last admin user")
)

// CreateUser creates a new user with a bcrypt-hashed password. Returns
// ErrPasswordTooShort if plaintextPassword is shorter than
// minPasswordLength, or ErrUsernameTaken if username is already in use.
func (r *Repo) CreateUser(ctx context.Context, username, plaintextPassword, role string) (model.User, error) {
	if len(plaintextPassword) < minPasswordLength {
		return model.User{}, ErrPasswordTooShort
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(plaintextPassword), bcryptCost)
	if err != nil {
		return model.User{}, err
	}

	uuid := idgen.New()
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO user (uuid, username, password_hash, role) VALUES (?, ?, ?, ?)`,
		uuid, username, string(hash), role,
	)
	if isUniqueConstraintError(err) {
		return model.User{}, ErrUsernameTaken
	}
	if err != nil {
		return model.User{}, err
	}
	return r.GetUserByUUID(ctx, uuid)
}

// GetUserByUUID fetches a user by UUID. Returns ErrNotFound if uuid doesn't exist.
func (r *Repo) GetUserByUUID(ctx context.Context, uuid string) (model.User, error) {
	var u model.User
	var createdAt string
	err := r.db.QueryRowContext(ctx,
		`SELECT uuid, username, role, created_at FROM user WHERE uuid = ?`, uuid,
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

// ListUsers returns every user, ordered by username. Unpaginated: matches
// the current admin GUI's user-management page, which lists everyone at once.
func (r *Repo) ListUsers(ctx context.Context) ([]model.User, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT uuid, username, role, created_at FROM user ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []model.User{}
	for rows.Next() {
		var u model.User
		var createdAt string
		if err := rows.Scan(&u.UUID, &u.Username, &u.Role, &createdAt); err != nil {
			return nil, err
		}
		created, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		u.CreatedAt = created
		out = append(out, u)
	}
	return out, rows.Err()
}

// VerifyLogin checks username/password and returns the user on success.
// Returns ErrInvalidCredentials (never ErrNotFound) on any failure, so
// callers can't distinguish "no such user" from "wrong password".
func (r *Repo) VerifyLogin(ctx context.Context, username, plaintextPassword string) (model.User, error) {
	var u model.User
	var passwordHash, createdAt string
	err := r.db.QueryRowContext(ctx,
		`SELECT uuid, username, role, password_hash, created_at FROM user WHERE username = ?`, username,
	).Scan(&u.UUID, &u.Username, &u.Role, &passwordHash, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword(dummyBcryptHash, []byte(plaintextPassword))
		return model.User{}, ErrInvalidCredentials
	}
	if err != nil {
		return model.User{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(plaintextPassword)) != nil {
		return model.User{}, ErrInvalidCredentials
	}
	created, err := parseTime(createdAt)
	if err != nil {
		return model.User{}, err
	}
	u.CreatedAt = created
	return u, nil
}

// DeleteUser deletes the user identified by uuid. Returns ErrNotFound if
// uuid doesn't exist, or ErrLastAdmin if uuid is the only remaining admin user.
func (r *Repo) DeleteUser(ctx context.Context, uuid string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var role string
	if err := tx.QueryRowContext(ctx, `SELECT role FROM user WHERE uuid = ?`, uuid).Scan(&role); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}

	if role == model.RoleAdmin {
		var adminCount int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM user WHERE role = ?`, model.RoleAdmin).Scan(&adminCount); err != nil {
			return err
		}
		if adminCount <= 1 {
			return ErrLastAdmin
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM user WHERE uuid = ?`, uuid); err != nil {
		return err
	}
	return tx.Commit()
}

// isUniqueConstraintError reports whether err came from a SQLite UNIQUE
// constraint violation. modernc.org/sqlite doesn't expose a typed
// constraint-kind API worth depending on here, so this matches on the
// driver's standard error text.
func isUniqueConstraintError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

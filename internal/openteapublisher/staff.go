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
	// ErrInvalidRole is returned by CreateStaff when role isn't
	// StaffRoleAdmin or StaffRoleMember.
	ErrInvalidRole = errors.New("openteapublisher: role must be \"admin\" or \"member\"")
	// ErrLastAdmin is returned by DeleteStaff when deleting would leave no
	// admin account -- mirrors internal/repo.ErrLastAdmin exactly (same
	// reasoning: a deployment must never be able to lock itself out of
	// admin-gated actions).
	ErrLastAdmin = errors.New("openteapublisher: cannot delete the last admin account")
)

// Staff roles (staff.role CHECK values,
// db/migrations/0003_staff_roles.sql). Two roles only, matching opentea's
// own admin/consumer split in shape: StaffRoleAdmin manages targets
// (§18.11 -- a target's bearer_token is a real credential, gating who may
// add/remove one) and other staff accounts; StaffRoleMember can do
// everything else (currently: view the dashboard). Not a capability
// system -- gates what actually needs gating today, no wider. Does not
// resolve §18.9's own gap (who may approve a collection draft) -- that
// workflow doesn't exist in the GUI yet; this is the schema groundwork it
// will eventually build on, not a solution to it.
const (
	StaffRoleAdmin  = "admin"
	StaffRoleMember = "member"
)

// RoleSatisfies reports whether staffRole grants access requiring minRole
// -- mirrors internal/authn.RoleSatisfies' own shape exactly (admin
// satisfies everything; anything else only satisfies itself).
func RoleSatisfies(staffRole, minRole string) bool {
	if staffRole == StaffRoleAdmin {
		return true
	}
	return staffRole == minRole
}

// Staff is one manufacturer staff account (Layer A,
// design/publisher-service.md §10.1).
type Staff struct {
	UUID      string
	Username  string
	Role      string
	CreatedAt time.Time
}

// CreateStaff creates a new staff account with a bcrypt-hashed password.
// Returns ErrPasswordTooShort if plaintextPassword is shorter than
// minPasswordLength, ErrInvalidRole if role isn't StaffRoleAdmin/Member,
// or ErrUsernameTaken if username is already in use.
func (r *Repo) CreateStaff(ctx context.Context, username, plaintextPassword, role string) (Staff, error) {
	if len(plaintextPassword) < minPasswordLength {
		return Staff{}, ErrPasswordTooShort
	}
	if role != StaffRoleAdmin && role != StaffRoleMember {
		return Staff{}, ErrInvalidRole
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(plaintextPassword), bcryptCost)
	if err != nil {
		return Staff{}, err
	}

	uuid := idgen.New()
	_, err = r.conn().ExecContext(ctx,
		`INSERT INTO staff (uuid, username, password_hash, role) VALUES (?, ?, ?, ?)`,
		uuid, username, string(hash), role,
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
		`SELECT uuid, username, role, created_at FROM staff WHERE uuid = ?`, uuid,
	).Scan(&s.UUID, &s.Username, &s.Role, &createdAt)
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

// ListStaff returns every staff account, ordered by username. Unpaginated
// -- matches ListTargets' own "list everyone at once" convention for this
// app's current size.
func (r *Repo) ListStaff(ctx context.Context) ([]Staff, error) {
	rows, err := r.conn().QueryContext(ctx, `SELECT uuid, username, role, created_at FROM staff ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []Staff{}
	for rows.Next() {
		var s Staff
		var createdAt string
		if err := rows.Scan(&s.UUID, &s.Username, &s.Role, &createdAt); err != nil {
			return nil, err
		}
		created, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		s.CreatedAt = created
		out = append(out, s)
	}
	return out, rows.Err()
}

// DeleteStaff deletes the staff account identified by uuid. Returns
// ErrNotFound if uuid doesn't exist, or ErrLastAdmin if uuid is the only
// remaining admin account -- mirrors internal/repo.DeleteUser's own
// last-admin protection exactly.
func (r *Repo) DeleteStaff(ctx context.Context, uuid string) error {
	var role string
	if err := r.conn().QueryRowContext(ctx, `SELECT role FROM staff WHERE uuid = ?`, uuid).Scan(&role); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}

	if role == StaffRoleAdmin {
		var adminCount int
		if err := r.conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM staff WHERE role = ?`, StaffRoleAdmin).Scan(&adminCount); err != nil {
			return err
		}
		if adminCount <= 1 {
			return ErrLastAdmin
		}
	}

	_, err := r.conn().ExecContext(ctx, `DELETE FROM staff WHERE uuid = ?`, uuid)
	return err
}

// VerifyLogin checks username/password and returns the staff account on
// success. Returns ErrInvalidCredentials (never ErrNotFound) on any
// failure, so callers can't distinguish "no such account" from "wrong
// password".
func (r *Repo) VerifyLogin(ctx context.Context, username, plaintextPassword string) (Staff, error) {
	var s Staff
	var passwordHash, createdAt string
	err := r.conn().QueryRowContext(ctx,
		`SELECT uuid, username, role, password_hash, created_at FROM staff WHERE username = ?`, username,
	).Scan(&s.UUID, &s.Username, &s.Role, &passwordHash, &createdAt)
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

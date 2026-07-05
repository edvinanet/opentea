package repo

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/oej/opentea/internal/idgen"
	"github.com/oej/opentea/internal/model"
)

var (
	// ErrInvalidCredentials is returned by VerifyLogin on any mismatch --
	// deliberately the same error whether the username or the password was
	// wrong, so callers can't use it to enumerate valid usernames.
	ErrInvalidCredentials = errors.New("repo: invalid credentials")
	ErrUsernameTaken      = errors.New("repo: username already taken")
	ErrLastAdmin          = errors.New("repo: cannot delete the last admin user")
)

func (r *Repo) CreateUser(ctx context.Context, username, plaintextPassword, role string) (model.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintextPassword), bcrypt.DefaultCost)
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

func (r *Repo) ListUsers(ctx context.Context) ([]model.User, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT uuid, username, role, created_at FROM user ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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

func (r *Repo) DeleteUser(ctx context.Context, uuid string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

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

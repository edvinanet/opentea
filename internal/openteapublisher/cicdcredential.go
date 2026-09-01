// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"

	"github.com/oej/opentea/internal/idgen"
)

// CICDCredential is one bearer credential opentea-publisher itself issues
// for a CI/CD pipeline to call its own /cicdapi/v1 surface against exactly
// one target (§18.11) -- a narrower, opentea-publisher-issued analogue of
// internal/repo's own publisher_credential, but always cicd-scoped: there
// is no "full" variant of this credential type.
type CICDCredential struct {
	UUID       string
	TargetUUID string
	Label      string
	CreatedAt  time.Time
	RevokedAt  *time.Time
}

// CreateCICDCredential mints a new cicd_credential for targetUUID and
// returns the raw token -- this is the only time the raw value is ever
// available; only its hash is stored (hashToken, session.go's). Returns
// ErrNotFound if targetUUID doesn't exist, so a bad selection fails
// cleanly rather than as a raw FK-constraint error.
func (r *Repo) CreateCICDCredential(ctx context.Context, targetUUID, label string) (CICDCredential, string, error) {
	if _, err := r.GetTarget(ctx, targetUUID); err != nil {
		return CICDCredential{}, "", err
	}

	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return CICDCredential{}, "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	uuid := idgen.New()

	if _, err := r.conn().ExecContext(ctx,
		`INSERT INTO cicd_credential (uuid, target_uuid, label, token_hash) VALUES (?, ?, ?, ?)`,
		uuid, targetUUID, label, hashToken(token),
	); err != nil {
		return CICDCredential{}, "", err
	}
	cred, err := r.getCICDCredential(ctx, `uuid = ?`, uuid)
	return cred, token, err
}

// GetCICDCredentialByToken resolves a raw bearer token to its credential
// -- requireCICDCredential (cicdapi_middleware.go) calls this on every
// /cicdapi/v1 request. Returns ErrNotFound if the token is unknown or has
// been revoked (a revoked credential is deliberately indistinguishable
// from an unknown one to the caller, matching
// internal/repo.GetPublisherCredentialByToken's own reasoning).
func (r *Repo) GetCICDCredentialByToken(ctx context.Context, token string) (CICDCredential, error) {
	cred, err := r.getCICDCredential(ctx, `token_hash = ?`, hashToken(token))
	if err != nil {
		return CICDCredential{}, err
	}
	if cred.RevokedAt != nil {
		return CICDCredential{}, ErrNotFound
	}
	return cred, nil
}

func (r *Repo) getCICDCredential(ctx context.Context, where string, arg any) (CICDCredential, error) {
	var (
		cred      CICDCredential
		createdAt string
		revokedAt sql.NullString
	)
	err := r.conn().QueryRowContext(ctx,
		`SELECT uuid, target_uuid, label, created_at, revoked_at FROM cicd_credential WHERE `+where, arg,
	).Scan(&cred.UUID, &cred.TargetUUID, &cred.Label, &createdAt, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CICDCredential{}, ErrNotFound
	}
	if err != nil {
		return CICDCredential{}, err
	}
	created, err := parseTime(createdAt)
	if err != nil {
		return CICDCredential{}, err
	}
	cred.CreatedAt = created
	revoked, err := parseNullTime(revokedAt)
	if err != nil {
		return CICDCredential{}, err
	}
	cred.RevokedAt = revoked
	return cred, nil
}

// ListCICDCredentials returns every credential for targetUUID (including
// revoked ones -- callers that need to exclude those check RevokedAt
// themselves), newest first. An empty targetUUID lists every credential
// across all targets.
func (r *Repo) ListCICDCredentials(ctx context.Context, targetUUID string) ([]CICDCredential, error) {
	query := `SELECT uuid, target_uuid, label, created_at, revoked_at FROM cicd_credential`
	args := []any{}
	if targetUUID != "" {
		query += ` WHERE target_uuid = ?`
		args = append(args, targetUUID)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := r.conn().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []CICDCredential{}
	for rows.Next() {
		var (
			cred      CICDCredential
			createdAt string
			revokedAt sql.NullString
		)
		if err := rows.Scan(&cred.UUID, &cred.TargetUUID, &cred.Label, &createdAt, &revokedAt); err != nil {
			return nil, err
		}
		created, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		cred.CreatedAt = created
		revoked, err := parseNullTime(revokedAt)
		if err != nil {
			return nil, err
		}
		cred.RevokedAt = revoked
		out = append(out, cred)
	}
	return out, rows.Err()
}

// RevokeCICDCredential marks uuid revoked (idempotent -- revoking an
// already-revoked credential again is not an error). Returns ErrNotFound
// if uuid doesn't exist at all -- mirrors
// internal/repo.RevokePublisherCredential exactly.
func (r *Repo) RevokeCICDCredential(ctx context.Context, uuid string) error {
	res, err := r.conn().ExecContext(ctx,
		`UPDATE cicd_credential SET revoked_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE uuid = ? AND revoked_at IS NULL`,
		uuid,
	)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n > 0 {
		return nil
	}
	if _, err := r.getCICDCredential(ctx, `uuid = ?`, uuid); err != nil {
		return err
	}
	return nil
}

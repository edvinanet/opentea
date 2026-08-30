// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"

	"github.com/oej/opentea/internal/idgen"
	"github.com/oej/opentea/internal/model"
)

// CreatePublisherCredential mints a new /publisher/v1 bearer credential
// with the given label/scope and returns the raw token -- this is the only
// time the raw value is ever available; only its hash is stored (see
// hashToken, shared with api_token/session).
func (r *Repo) CreatePublisherCredential(ctx context.Context, label, scope string) (model.PublisherCredential, string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return model.PublisherCredential{}, "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	uuid := idgen.New()

	if _, err := r.conn().ExecContext(ctx,
		`INSERT INTO publisher_credential (uuid, label, token_hash, scope) VALUES (?, ?, ?, ?)`,
		uuid, label, hashToken(token), scope,
	); err != nil {
		return model.PublisherCredential{}, "", err
	}
	cred, err := r.GetPublisherCredential(ctx, uuid)
	return cred, token, err
}

// GetPublisherCredential fetches a publisher credential by uuid (including
// a revoked one -- callers that need to exclude those check RevokedAt
// themselves, matching GetPublisherCredentialByToken's own revocation
// check). Returns ErrNotFound if uuid doesn't exist.
func (r *Repo) GetPublisherCredential(ctx context.Context, uuid string) (model.PublisherCredential, error) {
	return getPublisherCredentialTx(ctx, r.conn(), `uuid = ?`, uuid)
}

// GetPublisherCredentialByToken resolves a raw bearer token to its
// credential -- internal/publisher's auth middleware calls this on every
// request. Returns ErrNotFound if the token is unknown or has been
// revoked (a revoked credential is deliberately indistinguishable from an
// unknown one to the caller).
func (r *Repo) GetPublisherCredentialByToken(ctx context.Context, token string) (model.PublisherCredential, error) {
	cred, err := getPublisherCredentialTx(ctx, r.conn(), `token_hash = ?`, hashToken(token))
	if err != nil {
		return model.PublisherCredential{}, err
	}
	if cred.RevokedAt != nil {
		return model.PublisherCredential{}, ErrNotFound
	}
	return cred, nil
}

func getPublisherCredentialTx(ctx context.Context, q dbtx, where string, arg any) (model.PublisherCredential, error) {
	var (
		cred      model.PublisherCredential
		createdAt string
		revokedAt sql.NullString
	)
	err := q.QueryRowContext(ctx,
		`SELECT uuid, label, scope, created_at, revoked_at FROM publisher_credential WHERE `+where, arg,
	).Scan(&cred.UUID, &cred.Label, &cred.Scope, &createdAt, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.PublisherCredential{}, ErrNotFound
	}
	if err != nil {
		return model.PublisherCredential{}, err
	}
	created, err := parseTime(createdAt)
	if err != nil {
		return model.PublisherCredential{}, err
	}
	cred.CreatedAt = created
	revoked, err := parseNullTime(revokedAt)
	if err != nil {
		return model.PublisherCredential{}, err
	}
	cred.RevokedAt = revoked
	return cred, nil
}

// ListPublisherCredentials returns every issued credential (revoked ones
// included -- the admin GUI/JSON API needs to show revocation history, not
// just active credentials), newest first.
func (r *Repo) ListPublisherCredentials(ctx context.Context) ([]model.PublisherCredential, error) {
	rows, err := r.conn().QueryContext(ctx,
		`SELECT uuid, label, scope, created_at, revoked_at FROM publisher_credential ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []model.PublisherCredential{}
	for rows.Next() {
		var (
			cred      model.PublisherCredential
			createdAt string
			revokedAt sql.NullString
		)
		if err := rows.Scan(&cred.UUID, &cred.Label, &cred.Scope, &createdAt, &revokedAt); err != nil {
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

// RevokePublisherCredential marks uuid revoked (idempotent -- revoking an
// already-revoked credential again is not an error). Returns ErrNotFound
// if uuid doesn't exist at all.
func (r *Repo) RevokePublisherCredential(ctx context.Context, uuid string) error {
	res, err := r.conn().ExecContext(ctx,
		`UPDATE publisher_credential SET revoked_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE uuid = ? AND revoked_at IS NULL`,
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
	return r.ExistsPublisherCredential(ctx, uuid)
}

// ExistsPublisherCredential reports whether uuid exists at all (revoked or
// not) -- used by RevokePublisherCredential to distinguish "already
// revoked" (nil) from "no such credential" (ErrNotFound) after a
// zero-rows-affected UPDATE.
func (r *Repo) ExistsPublisherCredential(ctx context.Context, uuid string) error {
	var exists int
	err := r.conn().QueryRowContext(ctx, `SELECT 1 FROM publisher_credential WHERE uuid = ?`, uuid).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

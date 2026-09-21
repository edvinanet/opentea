// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"database/sql"
	"errors"
)

// FindProductReleaseUUIDByTEI resolves a TEI identifier to the UUID of the
// TEA Product Release that carries it, if any.
func (r *Repo) FindProductReleaseUUIDByTEI(ctx context.Context, tei string) (string, error) {
	var uuid string
	err := r.db.QueryRowContext(ctx,
		`SELECT owner_uuid FROM identifier WHERE owner_type = ? AND id_type = 'TEI' AND id_value = ?`,
		OwnerProductRelease, tei,
	).Scan(&uuid)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return uuid, nil
}

// FindProductReleaseUUIDByPURL resolves a PURL identifier to the UUID of
// the TEA Product Release that carries it, if any -- upstream TEA 1.0's
// GET /discovery (spec/openapi.yaml) resolves purl "within this TEA
// server's inventory" to a product release, the same target type tei
// resolves to; mirrors FindProductReleaseUUIDByTEI exactly, differing
// only in id_type.
func (r *Repo) FindProductReleaseUUIDByPURL(ctx context.Context, purl string) (string, error) {
	var uuid string
	err := r.db.QueryRowContext(ctx,
		`SELECT owner_uuid FROM identifier WHERE owner_type = ? AND id_type = 'PURL' AND id_value = ?`,
		OwnerProductRelease, purl,
	).Scan(&uuid)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return uuid, nil
}

// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/oej/opentea/internal/authz"
	"github.com/oej/opentea/internal/idgen"
	"github.com/oej/opentea/internal/model"
)

// EntitlementInput is CreateEntitlement's input. Resource/subject
// references (SubjectID, ResourceID) are not existence-checked against
// product/collection/artifact/user tables beyond what the schema's own
// foreign keys enforce (user, for a principal subject; template+revision,
// always) -- an entitlement scoped to a nonexistent product simply never
// matches anything at evaluation time, which is harmless, so Phase 1
// doesn't add extra application-level existence checks for it.
type EntitlementInput struct {
	SubjectType       authz.SubjectType
	SubjectID         string
	TemplateUUID      string
	TemplateRevision  int
	ResourceType      string
	ResourceID        string
	ValidFrom         *time.Time
	ValidUntil        *time.Time
	GrantingAuthority string
}

// CreateEntitlement creates a new, active entitlement.
func (r *Repo) CreateEntitlement(ctx context.Context, in EntitlementInput, createdBy string) (model.Entitlement, error) {
	uuid := idgen.New()

	_, err := runInTx(ctx, r, func(tx dbtx) (struct{}, error) {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO entitlement (uuid, subject_type, subject_id, template_uuid, template_revision, resource_type, resource_id, valid_from, valid_until, granting_authority, created_by)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid, string(in.SubjectType), nullIfEmpty(in.SubjectID), in.TemplateUUID, in.TemplateRevision,
			in.ResourceType, nullIfEmpty(in.ResourceID), formatTimePtr(in.ValidFrom), formatTimePtr(in.ValidUntil),
			in.GrantingAuthority, nullIfEmpty(createdBy),
		)
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, bumpWatermarkTx(ctx, tx, WatermarkEntitlements)
	})
	if err != nil {
		return model.Entitlement{}, err
	}
	return r.GetEntitlement(ctx, uuid)
}

// GetEntitlement fetches an entitlement by UUID. Returns ErrNotFound if
// uuid doesn't exist.
func (r *Repo) GetEntitlement(ctx context.Context, uuid string) (model.Entitlement, error) {
	return scanEntitlement(r.conn().QueryRowContext(ctx, entitlementSelectColumns+` FROM entitlement WHERE uuid = ?`, uuid))
}

const entitlementSelectColumns = `SELECT uuid, subject_type, subject_id, template_uuid, template_revision, resource_type, resource_id, status, valid_from, valid_until, granting_authority, created_at, created_by, revision`

func scanEntitlement(row *sql.Row) (model.Entitlement, error) {
	var e model.Entitlement
	var subjectType, resourceType string
	var subjectID, resourceID, createdBy sql.NullString
	var validFrom, validUntil sql.NullString
	var createdAt string
	err := row.Scan(&e.UUID, &subjectType, &subjectID, &e.TemplateUUID, &e.TemplateRevision,
		&resourceType, &resourceID, &e.Status, &validFrom, &validUntil,
		&e.GrantingAuthority, &createdAt, &createdBy, &e.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Entitlement{}, ErrNotFound
	}
	if err != nil {
		return model.Entitlement{}, err
	}
	e.SubjectType = authz.SubjectType(subjectType)
	e.SubjectID = subjectID.String
	e.ResourceType = resourceType
	e.ResourceID = resourceID.String
	e.CreatedBy = createdBy.String

	created, err := parseTime(createdAt)
	if err != nil {
		return model.Entitlement{}, err
	}
	e.CreatedAt = created
	if e.ValidFrom, err = parseNullTime(validFrom); err != nil {
		return model.Entitlement{}, err
	}
	if e.ValidUntil, err = parseNullTime(validUntil); err != nil {
		return model.Entitlement{}, err
	}
	return e, nil
}

// EntitlementFilter narrows ListEntitlements. An empty field means "no
// filter on that dimension".
type EntitlementFilter struct {
	SubjectType  string
	SubjectID    string
	ResourceType string
	ResourceID   string
}

// entitlementListLimit caps ListEntitlements -- this is an admin-tool
// listing, not the spec's paginated consumer surface, so a generous fixed
// cap (matching internal/admin's adminListLimit) is enough for Phase 1.
const entitlementListLimit = 1000

// ListEntitlements returns every entitlement matching filter, ordered by
// creation time, up to entitlementListLimit rows.
func (r *Repo) ListEntitlements(ctx context.Context, filter EntitlementFilter) ([]model.Entitlement, error) {
	query := entitlementSelectColumns + ` FROM entitlement WHERE 1=1`
	var args []any
	if filter.SubjectType != "" {
		query += ` AND subject_type = ?`
		args = append(args, filter.SubjectType)
	}
	if filter.SubjectID != "" {
		query += ` AND subject_id = ?`
		args = append(args, filter.SubjectID)
	}
	if filter.ResourceType != "" {
		query += ` AND resource_type = ?`
		args = append(args, filter.ResourceType)
	}
	if filter.ResourceID != "" {
		query += ` AND resource_id = ?`
		args = append(args, filter.ResourceID)
	}
	query += ` ORDER BY created_at LIMIT ?`
	args = append(args, entitlementListLimit)

	rows, err := r.conn().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []model.Entitlement{}
	for rows.Next() {
		var e model.Entitlement
		var subjectType, resourceType string
		var subjectID, resourceID, createdBy sql.NullString
		var validFrom, validUntil sql.NullString
		var createdAt string
		if err := rows.Scan(&e.UUID, &subjectType, &subjectID, &e.TemplateUUID, &e.TemplateRevision,
			&resourceType, &resourceID, &e.Status, &validFrom, &validUntil,
			&e.GrantingAuthority, &createdAt, &createdBy, &e.Revision); err != nil {
			return nil, err
		}
		e.SubjectType = authz.SubjectType(subjectType)
		e.SubjectID = subjectID.String
		e.ResourceType = resourceType
		e.ResourceID = resourceID.String
		e.CreatedBy = createdBy.String
		created, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		e.CreatedAt = created
		if e.ValidFrom, err = parseNullTime(validFrom); err != nil {
			return nil, err
		}
		if e.ValidUntil, err = parseNullTime(validUntil); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// UpdateEntitlementStatus transitions uuid to status (active/suspended/
// revoked), bumping its own revision and the global entitlements
// watermark. Returns ErrNotFound if uuid doesn't exist.
func (r *Repo) UpdateEntitlementStatus(ctx context.Context, uuid, status string) (model.Entitlement, error) {
	return runInTx(ctx, r, func(tx dbtx) (model.Entitlement, error) {
		res, err := tx.ExecContext(ctx, `UPDATE entitlement SET status = ?, revision = revision + 1 WHERE uuid = ?`, status, uuid)
		if err != nil {
			return model.Entitlement{}, err
		}
		if n, err := res.RowsAffected(); err != nil {
			return model.Entitlement{}, err
		} else if n == 0 {
			return model.Entitlement{}, ErrNotFound
		}
		if err := bumpWatermarkTx(ctx, tx, WatermarkEntitlements); err != nil {
			return model.Entitlement{}, err
		}
		return scanEntitlement(tx.QueryRowContext(ctx, entitlementSelectColumns+` FROM entitlement WHERE uuid = ?`, uuid))
	})
}

// UpdateEntitlementValidity replaces uuid's valid_from/valid_until window,
// bumping its own revision and the global entitlements watermark. Returns
// ErrNotFound if uuid doesn't exist.
func (r *Repo) UpdateEntitlementValidity(ctx context.Context, uuid string, validFrom, validUntil *time.Time) (model.Entitlement, error) {
	return runInTx(ctx, r, func(tx dbtx) (model.Entitlement, error) {
		res, err := tx.ExecContext(ctx,
			`UPDATE entitlement SET valid_from = ?, valid_until = ?, revision = revision + 1 WHERE uuid = ?`,
			formatTimePtr(validFrom), formatTimePtr(validUntil), uuid,
		)
		if err != nil {
			return model.Entitlement{}, err
		}
		if n, err := res.RowsAffected(); err != nil {
			return model.Entitlement{}, err
		} else if n == 0 {
			return model.Entitlement{}, ErrNotFound
		}
		if err := bumpWatermarkTx(ctx, tx, WatermarkEntitlements); err != nil {
			return model.Entitlement{}, err
		}
		return scanEntitlement(tx.QueryRowContext(ctx, entitlementSelectColumns+` FROM entitlement WHERE uuid = ?`, uuid))
	})
}

// DeleteEntitlement deletes uuid, bumping the global entitlements
// watermark. Returns ErrNotFound if uuid doesn't exist.
func (r *Repo) DeleteEntitlement(ctx context.Context, uuid string) error {
	_, err := runInTx(ctx, r, func(tx dbtx) (struct{}, error) {
		res, err := tx.ExecContext(ctx, `DELETE FROM entitlement WHERE uuid = ?`, uuid)
		if err != nil {
			return struct{}{}, err
		}
		if n, err := res.RowsAffected(); err != nil {
			return struct{}{}, err
		} else if n == 0 {
			return struct{}{}, ErrNotFound
		}
		return struct{}{}, bumpWatermarkTx(ctx, tx, WatermarkEntitlements)
	})
	return err
}

// GetPrincipalEntitlementWatermark returns the highest entitlement.revision
// among every active entitlement applicable to userUUID (its own
// principal-scoped entitlements, plus every everyone/authenticated one,
// since those apply to any authenticated caller too), or 0 if none apply.
// Combined with GetWatermark(ctx, WatermarkEntitlements) by the caller (see
// internal/api/cachepolicy.go's principalCacheKey), this partitions
// authenticated /tea/v1 responses' ETags per caller: it changes the moment
// any entitlement applicable to userUUID is created, modified, or removed.
func (r *Repo) GetPrincipalEntitlementWatermark(ctx context.Context, userUUID string) (int64, error) {
	var watermark int64
	err := r.conn().QueryRowContext(ctx,
		`SELECT COALESCE(MAX(revision), 0) FROM entitlement
		 WHERE status = 'active' AND (subject_type IN ('everyone','authenticated') OR (subject_type = 'principal' AND subject_id = ?))`,
		userUUID,
	).Scan(&watermark)
	return watermark, err
}

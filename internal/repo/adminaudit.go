package repo

import (
	"context"
	"database/sql"

	"github.com/oej/opentea/internal/model"
)

// AdminAuditInput is RecordAdminAudit's input -- see model.AdminAuditEntry
// for field meanings. PreviousState/Reason are optional (empty for
// creates/routine changes); ResultingState is always required.
type AdminAuditInput struct {
	ActorUUID      string
	RequestID      string
	Operation      string
	TargetType     string
	TargetID       string
	PreviousState  string
	ResultingState string
	Reason         string
}

// RecordAdminAudit inserts one admin_audit_log row, scoped to r's active
// transaction when r came from a WithTx closure -- so callers can fold this
// into the same transaction as the mutation it records (spec Sec 22.5:
// "Authorization-state mutations MUST atomically create an immutable audit
// record ... If either record cannot be committed, the mutation MUST
// fail"). See internal/admin/audit.go for the call-site pattern.
func (r *Repo) RecordAdminAudit(ctx context.Context, in AdminAuditInput) error {
	_, err := r.conn().ExecContext(ctx,
		`INSERT INTO admin_audit_log (actor_uuid, request_id, operation, target_type, target_id, previous_state, resulting_state, reason)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		nullIfEmpty(in.ActorUUID), nullIfEmpty(in.RequestID), in.Operation, in.TargetType, in.TargetID,
		nullIfEmpty(in.PreviousState), in.ResultingState, nullIfEmpty(in.Reason),
	)
	return err
}

// ListAdminAudit returns admin_audit_log rows for (targetType, targetID),
// newest first, up to limit rows. For a future /admin/ui audit view (spec
// Sec 22.5); nothing calls this in Phase 1's JSON-only admin surface yet.
func (r *Repo) ListAdminAudit(ctx context.Context, targetType, targetID string, limit int) ([]model.AdminAuditEntry, error) {
	rows, err := r.conn().QueryContext(ctx,
		`SELECT id, at, actor_uuid, request_id, operation, target_type, target_id, previous_state, resulting_state, reason
		 FROM admin_audit_log WHERE target_type = ? AND target_id = ? ORDER BY at DESC LIMIT ?`,
		targetType, targetID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []model.AdminAuditEntry{}
	for rows.Next() {
		var e model.AdminAuditEntry
		var actorUUID, requestID, previousState, reason sql.NullString
		var at string
		if err := rows.Scan(&e.ID, &at, &actorUUID, &requestID, &e.Operation, &e.TargetType, &e.TargetID, &previousState, &e.ResultingState, &reason); err != nil {
			return nil, err
		}
		e.ActorUUID = actorUUID.String
		e.RequestID = requestID.String
		e.PreviousState = previousState.String
		e.Reason = reason.String
		when, err := parseTime(at)
		if err != nil {
			return nil, err
		}
		e.At = when
		out = append(out, e)
	}
	return out, rows.Err()
}

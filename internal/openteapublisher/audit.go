// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// AuditEntry is one audit record -- every mutation opentea-publisher makes
// to its own database gets one, written atomically with the mutation
// itself (see RecordAudit, targets.go's call sites). PreviousState is
// empty for creates; both states are JSON snapshots of the affected
// object, not free-form text -- mirrors internal/model.AdminAuditEntry's
// own shape and reasoning.
type AuditEntry struct {
	ID             int64
	At             time.Time
	StaffUUID      string
	RequestID      string
	Operation      string
	TargetType     string
	TargetID       string
	PreviousState  string
	ResultingState string
}

// AuditInput carries what RecordAudit needs to write one entry.
// Previous/Resulting are marshaled to JSON; Previous may be nil for a
// create.
type AuditInput struct {
	StaffUUID  string
	RequestID  string
	Operation  string
	TargetType string
	TargetID   string
	Previous   any
	Resulting  any
}

// RecordAudit writes one audit entry. Call this from inside a WithTx
// callback alongside the mutation it describes, so the two commit or roll
// back together -- an audit record written after the fact (outside the
// same transaction) could be lost or left describing a mutation that
// never actually committed.
func (r *Repo) RecordAudit(ctx context.Context, in AuditInput) error {
	var previousJSON string
	if in.Previous != nil {
		b, err := json.Marshal(in.Previous)
		if err != nil {
			return err
		}
		previousJSON = string(b)
	}
	resultingJSON, err := json.Marshal(in.Resulting)
	if err != nil {
		return err
	}

	_, err = r.conn().ExecContext(ctx,
		`INSERT INTO audit_log (staff_uuid, request_id, operation, target_type, target_id, previous_state, resulting_state)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		nullIfEmpty(in.StaffUUID), nullIfEmpty(in.RequestID), in.Operation, in.TargetType, in.TargetID, nullIfEmpty(previousJSON), string(resultingJSON),
	)
	return err
}

// ListAuditEntries returns up to limit audit entries, newest first --
// unpaginated beyond the limit, matching ListTargets' own "list everyone
// at once" convention for this app's current size.
func (r *Repo) ListAuditEntries(ctx context.Context, limit int) ([]AuditEntry, error) {
	rows, err := r.conn().QueryContext(ctx,
		`SELECT id, at, staff_uuid, request_id, operation, target_type, target_id, previous_state, resulting_state
		 FROM audit_log ORDER BY id DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []AuditEntry{}
	for rows.Next() {
		var (
			e                          AuditEntry
			at                         string
			staffUUID, requestID, prev sql.NullString
		)
		if err := rows.Scan(&e.ID, &at, &staffUUID, &requestID, &e.Operation, &e.TargetType, &e.TargetID, &prev, &e.ResultingState); err != nil {
			return nil, err
		}
		parsedAt, err := parseTime(at)
		if err != nil {
			return nil, err
		}
		e.At = parsedAt
		e.StaffUUID = staffUUID.String
		e.RequestID = requestID.String
		e.PreviousState = prev.String
		out = append(out, e)
	}
	return out, rows.Err()
}

// nullIfEmpty returns nil for an empty string (stored as SQL NULL) or the
// string itself otherwise -- matches internal/repo's own convention.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

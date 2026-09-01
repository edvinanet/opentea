// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"context"
	"strings"
	"testing"
)

func TestRecordAndListAuditEntries(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	staff, err := r.CreateStaff(ctx, "alice", "hunter222", StaffRoleAdmin, "")
	if err != nil {
		t.Fatalf("CreateStaff: %v", err)
	}

	if err := r.RecordAudit(ctx, AuditInput{
		StaffUUID: staff.UUID, RequestID: "req-1",
		Operation: "target.create", TargetType: "target", TargetID: "target-1",
		Resulting: map[string]string{"label": "Acme"},
	}); err != nil {
		t.Fatalf("RecordAudit: %v", err)
	}
	if err := r.RecordAudit(ctx, AuditInput{
		StaffUUID: staff.UUID,
		Operation: "target.delete", TargetType: "target", TargetID: "target-1",
		Previous: map[string]string{"label": "Acme"}, Resulting: struct{}{},
	}); err != nil {
		t.Fatalf("RecordAudit (2nd): %v", err)
	}

	entries, err := r.ListAuditEntries(ctx, 10)
	if err != nil {
		t.Fatalf("ListAuditEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %+v, want 2", entries)
	}
	// Newest first.
	if entries[0].Operation != "target.delete" || entries[1].Operation != "target.create" {
		t.Fatalf("entries not newest-first: %+v", entries)
	}
	if entries[0].StaffUUID != staff.UUID {
		t.Fatalf("entries[0].StaffUUID = %q, want %q", entries[0].StaffUUID, staff.UUID)
	}
	if !strings.Contains(entries[0].PreviousState, "Acme") {
		t.Fatalf("entries[0].PreviousState = %q, want it to contain the previous label", entries[0].PreviousState)
	}
	if entries[1].RequestID != "req-1" {
		t.Fatalf("entries[1].RequestID = %q, want req-1", entries[1].RequestID)
	}
}

// TestCreateTargetWritesAuditEntry proves createTargetForm's audit write
// (targets.go) actually happens, atomically with the create -- not just
// that RecordAudit works in isolation.
func TestCreateAndDeleteTargetWritesAuditEntry(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	staff, err := r.CreateStaff(ctx, "alice", "hunter222", StaffRoleAdmin, "")
	if err != nil {
		t.Fatalf("CreateStaff: %v", err)
	}

	var created Target
	err = r.WithTx(ctx, func(tx *Repo) error {
		var err error
		created, err = tx.CreateTarget(ctx, "Acme production", "https://tea.example.com/publisher/v1", "s3cr3t")
		if err != nil {
			return err
		}
		return tx.RecordAudit(ctx, AuditInput{
			StaffUUID: staff.UUID, Operation: "target.create",
			TargetType: "target", TargetID: created.UUID, Resulting: redact(created),
		})
	})
	if err != nil {
		t.Fatalf("WithTx (create): %v", err)
	}

	entries, err := r.ListAuditEntries(ctx, 10)
	if err != nil {
		t.Fatalf("ListAuditEntries: %v", err)
	}
	if len(entries) != 1 || entries[0].Operation != "target.create" || entries[0].TargetID != created.UUID {
		t.Fatalf("entries = %+v", entries)
	}
	// The bearer token must never end up in the audit trail.
	if strings.Contains(entries[0].ResultingState, "s3cr3t") {
		t.Fatalf("audit entry leaked the bearer token: %s", entries[0].ResultingState)
	}

	err = r.WithTx(ctx, func(tx *Repo) error {
		if err := tx.DeleteTarget(ctx, created.UUID); err != nil {
			return err
		}
		return tx.RecordAudit(ctx, AuditInput{
			StaffUUID: staff.UUID, Operation: "target.delete",
			TargetType: "target", TargetID: created.UUID, Previous: redact(created), Resulting: struct{}{},
		})
	})
	if err != nil {
		t.Fatalf("WithTx (delete): %v", err)
	}

	entries, err = r.ListAuditEntries(ctx, 10)
	if err != nil {
		t.Fatalf("ListAuditEntries (after delete): %v", err)
	}
	if len(entries) != 2 || entries[0].Operation != "target.delete" {
		t.Fatalf("entries = %+v", entries)
	}
}

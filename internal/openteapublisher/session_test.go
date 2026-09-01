// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"context"
	"testing"
)

func TestCreateSessionAndGetSessionStaff(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	staff, err := r.CreateStaff(ctx, "alice", "hunter222", StaffRoleAdmin)
	if err != nil {
		t.Fatalf("CreateStaff: %v", err)
	}

	token, expiresAt, err := r.CreateSession(ctx, staff.UUID)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if token == "" || expiresAt.IsZero() {
		t.Fatalf("token=%q expiresAt=%v", token, expiresAt)
	}

	got, err := r.GetSessionStaff(ctx, token)
	if err != nil {
		t.Fatalf("GetSessionStaff: %v", err)
	}
	if got.UUID != staff.UUID {
		t.Fatalf("got = %+v", got)
	}

	if err := r.DeleteSession(ctx, token); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := r.GetSessionStaff(ctx, token); err != ErrNotFound {
		t.Fatalf("GetSessionStaff after delete: err = %v, want ErrNotFound", err)
	}
}

func TestGetSessionStaffUnknownToken(t *testing.T) {
	r := newTestRepo(t)
	if _, err := r.GetSessionStaff(context.Background(), "not-a-real-token"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

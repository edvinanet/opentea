// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"context"
	"testing"
)

func TestCreateStaffAndVerifyLogin(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	created, err := r.CreateStaff(ctx, "alice", "hunter222", StaffRoleAdmin)
	if err != nil {
		t.Fatalf("CreateStaff: %v", err)
	}
	if created.Username != "alice" || created.Role != StaffRoleAdmin {
		t.Fatalf("created = %+v", created)
	}

	got, err := r.VerifyLogin(ctx, "alice", "hunter222")
	if err != nil {
		t.Fatalf("VerifyLogin (correct password): %v", err)
	}
	if got.UUID != created.UUID || got.Role != StaffRoleAdmin {
		t.Fatalf("VerifyLogin returned wrong staff account: %+v", got)
	}

	if _, err := r.VerifyLogin(ctx, "alice", "wrong-password"); err != ErrInvalidCredentials {
		t.Fatalf("VerifyLogin (wrong password): err = %v, want ErrInvalidCredentials", err)
	}
	if _, err := r.VerifyLogin(ctx, "nobody", "hunter222"); err != ErrInvalidCredentials {
		t.Fatalf("VerifyLogin (unknown username): err = %v, want ErrInvalidCredentials (not ErrNotFound, to avoid username enumeration)", err)
	}

	fetched, err := r.GetStaffByUUID(ctx, created.UUID)
	if err != nil {
		t.Fatalf("GetStaffByUUID: %v", err)
	}
	if fetched.Username != "alice" {
		t.Fatalf("fetched = %+v", fetched)
	}
	if _, err := r.GetStaffByUUID(ctx, "00000000-0000-4000-8000-000000000000"); err != ErrNotFound {
		t.Fatalf("GetStaffByUUID (unknown): err = %v, want ErrNotFound", err)
	}
}

func TestCreateStaffUsernameTaken(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	if _, err := r.CreateStaff(ctx, "alice", "hunter222", StaffRoleAdmin); err != nil {
		t.Fatalf("CreateStaff (first): %v", err)
	}
	if _, err := r.CreateStaff(ctx, "alice", "different-pw", StaffRoleMember); err != ErrUsernameTaken {
		t.Fatalf("CreateStaff (duplicate): err = %v, want ErrUsernameTaken", err)
	}
}

func TestCreateStaffPasswordTooShort(t *testing.T) {
	r := newTestRepo(t)
	if _, err := r.CreateStaff(context.Background(), "alice", "short", StaffRoleAdmin); err != ErrPasswordTooShort {
		t.Fatalf("err = %v, want ErrPasswordTooShort", err)
	}
}

func TestCreateStaffInvalidRole(t *testing.T) {
	r := newTestRepo(t)
	if _, err := r.CreateStaff(context.Background(), "alice", "hunter222", "superuser"); err != ErrInvalidRole {
		t.Fatalf("err = %v, want ErrInvalidRole", err)
	}
}

func TestRoleSatisfies(t *testing.T) {
	cases := []struct {
		staffRole, minRole string
		want               bool
	}{
		{StaffRoleAdmin, StaffRoleAdmin, true},
		{StaffRoleAdmin, StaffRoleMember, true},
		{StaffRoleMember, StaffRoleMember, true},
		{StaffRoleMember, StaffRoleAdmin, false},
	}
	for _, c := range cases {
		if got := RoleSatisfies(c.staffRole, c.minRole); got != c.want {
			t.Errorf("RoleSatisfies(%q, %q) = %v, want %v", c.staffRole, c.minRole, got, c.want)
		}
	}
}

func TestListAndDeleteStaff(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	admin, err := r.CreateStaff(ctx, "alice", "hunter222", StaffRoleAdmin)
	if err != nil {
		t.Fatalf("CreateStaff (admin): %v", err)
	}
	member, err := r.CreateStaff(ctx, "bob", "hunter333", StaffRoleMember)
	if err != nil {
		t.Fatalf("CreateStaff (member): %v", err)
	}

	list, err := r.ListStaff(ctx)
	if err != nil {
		t.Fatalf("ListStaff: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list = %+v, want 2 accounts", list)
	}

	// Deleting the member is fine.
	if err := r.DeleteStaff(ctx, member.UUID); err != nil {
		t.Fatalf("DeleteStaff (member): %v", err)
	}
	if _, err := r.GetStaffByUUID(ctx, member.UUID); err != ErrNotFound {
		t.Fatalf("GetStaffByUUID after delete: err = %v, want ErrNotFound", err)
	}

	// Deleting the last admin is not.
	if err := r.DeleteStaff(ctx, admin.UUID); err != ErrLastAdmin {
		t.Fatalf("DeleteStaff (last admin): err = %v, want ErrLastAdmin", err)
	}

	if err := r.DeleteStaff(ctx, "00000000-0000-4000-8000-000000000000"); err != ErrNotFound {
		t.Fatalf("DeleteStaff (unknown): err = %v, want ErrNotFound", err)
	}
}

// TestDeleteStaffAllowsRemovingOneOfMultipleAdmins confirms the
// last-admin check counts correctly rather than always rejecting an
// admin deletion.
func TestDeleteStaffAllowsRemovingOneOfMultipleAdmins(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	first, err := r.CreateStaff(ctx, "alice", "hunter222", StaffRoleAdmin)
	if err != nil {
		t.Fatalf("CreateStaff (first admin): %v", err)
	}
	if _, err := r.CreateStaff(ctx, "bob", "hunter333", StaffRoleAdmin); err != nil {
		t.Fatalf("CreateStaff (second admin): %v", err)
	}

	if err := r.DeleteStaff(ctx, first.UUID); err != nil {
		t.Fatalf("DeleteStaff (one of two admins): %v", err)
	}
}

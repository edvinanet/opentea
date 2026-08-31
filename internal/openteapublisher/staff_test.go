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

	created, err := r.CreateStaff(ctx, "alice", "hunter222")
	if err != nil {
		t.Fatalf("CreateStaff: %v", err)
	}
	if created.Username != "alice" {
		t.Fatalf("created = %+v", created)
	}

	got, err := r.VerifyLogin(ctx, "alice", "hunter222")
	if err != nil {
		t.Fatalf("VerifyLogin (correct password): %v", err)
	}
	if got.UUID != created.UUID {
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

	if _, err := r.CreateStaff(ctx, "alice", "hunter222"); err != nil {
		t.Fatalf("CreateStaff (first): %v", err)
	}
	if _, err := r.CreateStaff(ctx, "alice", "different-pw"); err != ErrUsernameTaken {
		t.Fatalf("CreateStaff (duplicate): err = %v, want ErrUsernameTaken", err)
	}
}

func TestCreateStaffPasswordTooShort(t *testing.T) {
	r := newTestRepo(t)
	if _, err := r.CreateStaff(context.Background(), "alice", "short"); err != ErrPasswordTooShort {
		t.Fatalf("err = %v, want ErrPasswordTooShort", err)
	}
}

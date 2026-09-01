// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"context"
	"testing"
)

func TestCreateGetListRevokeCICDCredential(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	target, err := r.CreateTarget(ctx, "Acme production", "https://tea.example.com/publisher/v1", "s3cr3t")
	if err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}

	created, token, err := r.CreateCICDCredential(ctx, target.UUID, "CI pipeline")
	if err != nil {
		t.Fatalf("CreateCICDCredential: %v", err)
	}
	if created.TargetUUID != target.UUID || created.Label != "CI pipeline" || created.RevokedAt != nil {
		t.Fatalf("created = %+v", created)
	}
	if token == "" {
		t.Fatal("token is empty")
	}

	got, err := r.GetCICDCredentialByToken(ctx, token)
	if err != nil {
		t.Fatalf("GetCICDCredentialByToken: %v", err)
	}
	if got.UUID != created.UUID {
		t.Fatalf("got = %+v", got)
	}
	if _, err := r.GetCICDCredentialByToken(ctx, "not-a-real-token"); err != ErrNotFound {
		t.Fatalf("GetCICDCredentialByToken (unknown): err = %v, want ErrNotFound", err)
	}

	list, err := r.ListCICDCredentials(ctx, target.UUID)
	if err != nil {
		t.Fatalf("ListCICDCredentials: %v", err)
	}
	if len(list) != 1 || list[0].UUID != created.UUID {
		t.Fatalf("list = %+v", list)
	}
	if all, err := r.ListCICDCredentials(ctx, ""); err != nil || len(all) != 1 {
		t.Fatalf("ListCICDCredentials (all targets): list=%+v err=%v", all, err)
	}

	if err := r.RevokeCICDCredential(ctx, created.UUID); err != nil {
		t.Fatalf("RevokeCICDCredential: %v", err)
	}
	// Revoking again is a no-op, not an error.
	if err := r.RevokeCICDCredential(ctx, created.UUID); err != nil {
		t.Fatalf("RevokeCICDCredential (again): %v", err)
	}
	if err := r.RevokeCICDCredential(ctx, "00000000-0000-4000-8000-000000000000"); err != ErrNotFound {
		t.Fatalf("RevokeCICDCredential (unknown): err = %v, want ErrNotFound", err)
	}

	// A revoked credential's token no longer resolves -- indistinguishable
	// from unknown, matching internal/repo.GetPublisherCredentialByToken's
	// own reasoning.
	if _, err := r.GetCICDCredentialByToken(ctx, token); err != ErrNotFound {
		t.Fatalf("GetCICDCredentialByToken (revoked): err = %v, want ErrNotFound", err)
	}
	revokedList, err := r.ListCICDCredentials(ctx, target.UUID)
	if err != nil || len(revokedList) != 1 || revokedList[0].RevokedAt == nil {
		t.Fatalf("ListCICDCredentials (after revoke): list=%+v err=%v, want RevokedAt set", revokedList, err)
	}
}

func TestCreateCICDCredentialUnknownTarget(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	if _, _, err := r.CreateCICDCredential(ctx, "00000000-0000-4000-8000-000000000000", "CI pipeline"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

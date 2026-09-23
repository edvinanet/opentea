// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"testing"
	"time"

	"github.com/oej/opentea/internal/model"
)

func TestAPIKeyGenerateAndVerify(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	user, err := r.CreateUser(ctx, "alice", "password1", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if createdAt, err := r.GetAPIKeyCreatedAt(ctx, user.UUID); err != nil || createdAt != nil {
		t.Fatalf("GetAPIKeyCreatedAt (before generating): createdAt=%v err=%v, want nil,nil", createdAt, err)
	}

	keyID, secret, err := r.SetAPIKey(ctx, user.UUID)
	if err != nil {
		t.Fatalf("SetAPIKey: %v", err)
	}
	if keyID == "" || secret == "" {
		t.Fatalf("expected non-empty keyID/secret, got %q/%q", keyID, secret)
	}

	createdAt, err := r.GetAPIKeyCreatedAt(ctx, user.UUID)
	if err != nil {
		t.Fatalf("GetAPIKeyCreatedAt: %v", err)
	}
	if createdAt == nil {
		t.Fatal("expected non-nil createdAt after generating a key")
	}

	got, err := r.VerifyAPIKey(ctx, keyID, secret)
	if err != nil {
		t.Fatalf("VerifyAPIKey: %v", err)
	}
	if got.UUID != user.UUID {
		t.Fatalf("VerifyAPIKey returned wrong user: %+v", got)
	}

	if _, err := r.VerifyAPIKey(ctx, "not-a-real-key-id", secret); err != ErrNotFound {
		t.Fatalf("VerifyAPIKey (unknown keyID): err = %v, want ErrNotFound", err)
	}
	if _, err := r.VerifyAPIKey(ctx, keyID, "wrong-secret"); err != ErrNotFound {
		t.Fatalf("VerifyAPIKey (wrong secret): err = %v, want ErrNotFound", err)
	}
}

func TestAPIKeyRegenerateInvalidatesOld(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	user, err := r.CreateUser(ctx, "alice", "password1", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	firstKeyID, firstSecret, err := r.SetAPIKey(ctx, user.UUID)
	if err != nil {
		t.Fatalf("SetAPIKey (first): %v", err)
	}
	secondKeyID, secondSecret, err := r.SetAPIKey(ctx, user.UUID)
	if err != nil {
		t.Fatalf("SetAPIKey (second): %v", err)
	}
	if firstKeyID == secondKeyID || firstSecret == secondSecret {
		t.Fatal("expected regeneration to produce a different keyID/secret")
	}

	if _, err := r.VerifyAPIKey(ctx, firstKeyID, firstSecret); err != ErrNotFound {
		t.Fatalf("old key should be invalidated: err = %v, want ErrNotFound", err)
	}
	got, err := r.VerifyAPIKey(ctx, secondKeyID, secondSecret)
	if err != nil {
		t.Fatalf("VerifyAPIKey (new key): %v", err)
	}
	if got.UUID != user.UUID {
		t.Fatalf("VerifyAPIKey returned wrong user: %+v", got)
	}
}

func TestAccessTokenCreateAndResolve(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	user, err := r.CreateUser(ctx, "alice", "password1", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	token, expiresAt, err := r.CreateAccessToken(ctx, user.UUID, time.Hour)
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty access token")
	}
	if !expiresAt.After(time.Now()) {
		t.Fatalf("expiresAt = %v, want in the future", expiresAt)
	}

	got, err := r.GetAccessTokenUser(ctx, token)
	if err != nil {
		t.Fatalf("GetAccessTokenUser: %v", err)
	}
	if got.UUID != user.UUID {
		t.Fatalf("GetAccessTokenUser returned wrong user: %+v", got)
	}

	if _, err := r.GetAccessTokenUser(ctx, "not-a-real-access-token"); err != ErrNotFound {
		t.Fatalf("GetAccessTokenUser (unknown): err = %v, want ErrNotFound", err)
	}
}

func TestAccessTokenExpired(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	user, err := r.CreateUser(ctx, "alice", "password1", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	token, _, err := r.CreateAccessToken(ctx, user.UUID, -time.Minute)
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}

	if _, err := r.GetAccessTokenUser(ctx, token); err != ErrNotFound {
		t.Fatalf("GetAccessTokenUser (expired): err = %v, want ErrNotFound", err)
	}
}

package repo

import (
	"context"
	"testing"
	"time"

	"github.com/oej/opentea/internal/model"
)

func TestSessionCreateGetDelete(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	user, err := r.CreateUser(ctx, "alice", "pw", model.RoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	token, expiresAt, err := r.CreateSession(ctx, user.UUID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
	if !expiresAt.After(time.Now()) {
		t.Fatalf("expiresAt = %v, want in the future", expiresAt)
	}

	got, err := r.GetSessionUser(ctx, token)
	if err != nil {
		t.Fatalf("GetSessionUser: %v", err)
	}
	if got.UUID != user.UUID {
		t.Fatalf("GetSessionUser returned wrong user: %+v", got)
	}

	if err := r.DeleteSession(ctx, token); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := r.GetSessionUser(ctx, token); err != ErrNotFound {
		t.Fatalf("GetSessionUser (after delete): err = %v, want ErrNotFound", err)
	}
}

func TestSessionExpiry(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	user, err := r.CreateUser(ctx, "alice", "pw", model.RoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Negative TTL creates an already-expired session.
	token, _, err := r.CreateSession(ctx, user.UUID, -time.Minute)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := r.GetSessionUser(ctx, token); err != ErrNotFound {
		t.Fatalf("GetSessionUser (expired): err = %v, want ErrNotFound", err)
	}
}

func TestGetSessionUserUnknownToken(t *testing.T) {
	r := newTestRepo(t)
	if _, err := r.GetSessionUser(context.Background(), "not-a-real-token"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

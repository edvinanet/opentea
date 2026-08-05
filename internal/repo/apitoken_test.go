package repo

import (
	"context"
	"testing"

	"github.com/oej/opentea/internal/model"
)

func TestAPITokenGenerateAndLookup(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	user, err := r.CreateUser(ctx, "alice", "password1", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if createdAt, err := r.GetAPITokenCreatedAt(ctx, user.UUID); err != nil || createdAt != nil {
		t.Fatalf("GetAPITokenCreatedAt (before generating): createdAt=%v err=%v, want nil,nil", createdAt, err)
	}

	token, err := r.SetAPIToken(ctx, user.UUID)
	if err != nil {
		t.Fatalf("SetAPIToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	createdAt, err := r.GetAPITokenCreatedAt(ctx, user.UUID)
	if err != nil {
		t.Fatalf("GetAPITokenCreatedAt: %v", err)
	}
	if createdAt == nil {
		t.Fatal("expected non-nil createdAt after generating a token")
	}

	got, err := r.GetUserByAPIToken(ctx, token)
	if err != nil {
		t.Fatalf("GetUserByAPIToken: %v", err)
	}
	if got.UUID != user.UUID {
		t.Fatalf("GetUserByAPIToken returned wrong user: %+v", got)
	}

	if _, err := r.GetUserByAPIToken(ctx, "not-a-real-token"); err != ErrNotFound {
		t.Fatalf("GetUserByAPIToken (invalid): err = %v, want ErrNotFound", err)
	}
}

func TestAPITokenRegenerateInvalidatesOld(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	user, err := r.CreateUser(ctx, "alice", "password1", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	first, err := r.SetAPIToken(ctx, user.UUID)
	if err != nil {
		t.Fatalf("SetAPIToken (first): %v", err)
	}
	second, err := r.SetAPIToken(ctx, user.UUID)
	if err != nil {
		t.Fatalf("SetAPIToken (second): %v", err)
	}
	if first == second {
		t.Fatal("expected regeneration to produce a different token")
	}

	if _, err := r.GetUserByAPIToken(ctx, first); err != ErrNotFound {
		t.Fatalf("old token should be invalidated: err = %v, want ErrNotFound", err)
	}
	got, err := r.GetUserByAPIToken(ctx, second)
	if err != nil {
		t.Fatalf("GetUserByAPIToken (new token): %v", err)
	}
	if got.UUID != user.UUID {
		t.Fatalf("GetUserByAPIToken returned wrong user: %+v", got)
	}
}

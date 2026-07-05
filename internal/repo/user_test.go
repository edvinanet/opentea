package repo

import (
	"context"
	"testing"

	"github.com/oej/opentea/internal/model"
)

func TestCreateUserAndVerifyLogin(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	created, err := r.CreateUser(ctx, "alice", "hunter2", model.RoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if created.Username != "alice" || created.Role != model.RoleAdmin {
		t.Fatalf("created = %+v", created)
	}

	got, err := r.VerifyLogin(ctx, "alice", "hunter2")
	if err != nil {
		t.Fatalf("VerifyLogin (correct password): %v", err)
	}
	if got.UUID != created.UUID {
		t.Fatalf("VerifyLogin returned wrong user: %+v", got)
	}

	if _, err := r.VerifyLogin(ctx, "alice", "wrong"); err != ErrInvalidCredentials {
		t.Fatalf("VerifyLogin (wrong password): err = %v, want ErrInvalidCredentials", err)
	}
	if _, err := r.VerifyLogin(ctx, "nobody", "hunter2"); err != ErrInvalidCredentials {
		t.Fatalf("VerifyLogin (unknown user): err = %v, want ErrInvalidCredentials (not ErrNotFound, to avoid username enumeration)", err)
	}
}

func TestCreateUserDuplicateUsername(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	if _, err := r.CreateUser(ctx, "bob", "pw", model.RoleConsumer); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := r.CreateUser(ctx, "bob", "otherpw", model.RoleAdmin); err != ErrUsernameTaken {
		t.Fatalf("err = %v, want ErrUsernameTaken", err)
	}
}

func TestListUsers(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	if _, err := r.CreateUser(ctx, "zed", "pw", model.RoleConsumer); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := r.CreateUser(ctx, "amy", "pw", model.RoleAdmin); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	users, err := r.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 2 || users[0].Username != "amy" || users[1].Username != "zed" {
		t.Fatalf("ListUsers = %+v, want [amy, zed] (sorted by username)", users)
	}
}

func TestDeleteUserLastAdminGuard(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	admin, err := r.CreateUser(ctx, "solo-admin", "pw", model.RoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := r.DeleteUser(ctx, admin.UUID); err != ErrLastAdmin {
		t.Fatalf("err = %v, want ErrLastAdmin", err)
	}

	// Adding a second admin allows deleting the first.
	second, err := r.CreateUser(ctx, "second-admin", "pw", model.RoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := r.DeleteUser(ctx, admin.UUID); err != nil {
		t.Fatalf("DeleteUser (with a second admin present): %v", err)
	}
	// The remaining admin is now the last one -- deleting them should fail again.
	if err := r.DeleteUser(ctx, second.UUID); err != ErrLastAdmin {
		t.Fatalf("err = %v, want ErrLastAdmin", err)
	}
}

func TestDeleteUserConsumerNotGuarded(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	if _, err := r.CreateUser(ctx, "admin", "pw", model.RoleAdmin); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	consumer, err := r.CreateUser(ctx, "consumer", "pw", model.RoleConsumer)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := r.DeleteUser(ctx, consumer.UUID); err != nil {
		t.Fatalf("DeleteUser (consumer role): %v", err)
	}
}

func TestDeleteUserNotFound(t *testing.T) {
	r := newTestRepo(t)
	if err := r.DeleteUser(context.Background(), "00000000-0000-4000-8000-000000000000"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

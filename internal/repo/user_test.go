// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package repo

import (
	"context"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/oej/opentea/internal/model"
)

func TestCreateUserAndVerifyLogin(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	created, err := r.CreateUser(ctx, "alice", "hunter22", model.RoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if created.Username != "alice" || created.Role != model.RoleAdmin {
		t.Fatalf("created = %+v", created)
	}

	got, err := r.VerifyLogin(ctx, "alice", "hunter22")
	if err != nil {
		t.Fatalf("VerifyLogin (correct password): %v", err)
	}
	if got.UUID != created.UUID {
		t.Fatalf("VerifyLogin returned wrong user: %+v", got)
	}

	if _, err := r.VerifyLogin(ctx, "alice", "wrong-password"); err != ErrInvalidCredentials {
		t.Fatalf("VerifyLogin (wrong password): err = %v, want ErrInvalidCredentials", err)
	}
	if _, err := r.VerifyLogin(ctx, "nobody", "hunter22"); err != ErrInvalidCredentials {
		t.Fatalf("VerifyLogin (unknown user): err = %v, want ErrInvalidCredentials (not ErrNotFound, to avoid username enumeration)", err)
	}
}

// TestVerifyLoginTimingDoesNotRevealUsernameExistence guards against a
// timing oracle: an unknown username used to return immediately (skipping
// bcrypt entirely) while a known username with a wrong password always
// paid bcrypt's ~100ms cost, letting a remote attacker enumerate valid
// usernames purely from response latency despite both paths returning the
// identical ErrInvalidCredentials. Timing assertions are inherently noisy,
// so this uses several iterations and a generous ratio bound (2x) rather
// than a tight one -- loose enough to avoid flaking under CI scheduler
// jitter, but tight enough that the fix being absent (unknown-username
// path near-instant, known-username path ~100ms per call) would still
// fail it by a wide margin.
func TestVerifyLoginTimingDoesNotRevealUsernameExistence(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	if _, err := r.CreateUser(ctx, "alice", "hunter22", model.RoleAdmin); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	const iterations = 5
	knownUserElapsed := timeVerifyLoginAttempts(ctx, r, "alice", "wrong-password", iterations)
	unknownUserElapsed := timeVerifyLoginAttempts(ctx, r, "nobody", "wrong-password", iterations)

	ratio := float64(unknownUserElapsed) / float64(knownUserElapsed)
	if ratio < 0.5 || ratio > 2.0 {
		t.Fatalf("unknown-username path took %v, known-username-wrong-password path took %v (ratio %.2f) -- want them within 2x of each other; a large gap suggests the dummy-hash comparison isn't running",
			unknownUserElapsed, knownUserElapsed, ratio)
	}
}

func timeVerifyLoginAttempts(ctx context.Context, r *Repo, username, password string, n int) time.Duration {
	start := time.Now()
	for i := 0; i < n; i++ {
		_, _ = r.VerifyLogin(ctx, username, password)
	}
	return time.Since(start)
}

func TestCreateUserRejectsShortPassword(t *testing.T) {
	r := newTestRepo(t)
	if _, err := r.CreateUser(context.Background(), "alice", "short", model.RoleAdmin); err != ErrPasswordTooShort {
		t.Fatalf("err = %v, want ErrPasswordTooShort", err)
	}
}

// TestCreateUserUsesBcryptCost confirms newly created passwords are hashed
// at 12, not bcrypt's own lower DefaultCost (10) -- the cost is encoded in
// the stored hash itself, so this reads it back the same way VerifyLogin's
// bcrypt.CompareHashAndPassword would. Deliberately asserts against the
// literal 12, not the bcryptCost constant this same behavior is driven by
// -- comparing against that constant would make this test pass no matter
// what bcryptCost is ever changed to, since both sides would move together.
func TestCreateUserUsesBcryptCost(t *testing.T) {
	const wantCost = 12
	ctx := context.Background()
	r := newTestRepo(t)

	if _, err := r.CreateUser(ctx, "alice", "password1", model.RoleAdmin); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	var hash string
	if err := r.db.QueryRowContext(ctx, `SELECT password_hash FROM user WHERE username = ?`, "alice").Scan(&hash); err != nil {
		t.Fatalf("query password_hash: %v", err)
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		t.Fatalf("bcrypt.Cost: %v", err)
	}
	if cost != wantCost {
		t.Fatalf("cost = %d, want %d", cost, wantCost)
	}
}

func TestCreateUserDuplicateUsername(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	if _, err := r.CreateUser(ctx, "bob", "password1", model.RoleConsumer); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := r.CreateUser(ctx, "bob", "otherpass1", model.RoleAdmin); err != ErrUsernameTaken {
		t.Fatalf("err = %v, want ErrUsernameTaken", err)
	}
}

func TestListUsers(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	if _, err := r.CreateUser(ctx, "zed", "password1", model.RoleConsumer); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := r.CreateUser(ctx, "amy", "password1", model.RoleAdmin); err != nil {
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

	admin, err := r.CreateUser(ctx, "solo-admin", "password1", model.RoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := r.DeleteUser(ctx, admin.UUID); err != ErrLastAdmin {
		t.Fatalf("err = %v, want ErrLastAdmin", err)
	}

	// Adding a second admin allows deleting the first.
	second, err := r.CreateUser(ctx, "second-admin", "password1", model.RoleAdmin)
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

	if _, err := r.CreateUser(ctx, "admin", "password1", model.RoleAdmin); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	consumer, err := r.CreateUser(ctx, "consumer", "password1", model.RoleConsumer)
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

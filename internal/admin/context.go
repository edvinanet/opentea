// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package admin

import (
	"context"

	"github.com/oej/opentea/internal/model"
)

// actorContextKey is an unexported type so this package's context key can't
// collide with any key set by another package.
type actorContextKey struct{}

// withActor returns a context carrying user, stashed by requireRole once
// per request and read by write handlers that need to attribute an
// admin_audit_log row to someone (see audit.go).
func withActor(ctx context.Context, user model.User) context.Context {
	return context.WithValue(ctx, actorContextKey{}, user)
}

// actorFromContext returns the authenticated admin user for this request.
// Every route reaching a handler has already passed through requireRole,
// which always sets this, so the zero value should never surface in
// practice -- but returning it rather than panicking keeps a missing actor
// a harmless empty audit field instead of a crash.
func actorFromContext(ctx context.Context) model.User {
	user, _ := ctx.Value(actorContextKey{}).(model.User)
	return user
}

// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package api

import (
	"context"

	"github.com/oej/opentea/internal/authz"
)

// principalContextKey is an unexported type so this package's context key
// can't collide with any key set by another package -- the standard Go
// context-key-collision-avoidance idiom.
type principalContextKey struct{}

// withPrincipal returns a context carrying p, resolved once by
// resolvePrincipal and read by every /tea/v1 handler via
// principalFromContext. This is the only /tea/v1-internal request-scoped
// value carried this way -- don't grow this into a general "context bag"
// pattern.
func withPrincipal(ctx context.Context, p authz.Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, p)
}

// principalFromContext returns the resolved principal, or the zero value
// (anonymous) if none was set. That should never happen for a request that
// passed through resolvePrincipal, but resolving to "anonymous" rather than
// panicking is the fail-safe default: anonymous is the MOST restrictive
// starting point (only 'everyone'-scoped grants can ever apply), not the
// least, so a missing principal can never accidentally over-grant access.
func principalFromContext(ctx context.Context) authz.Principal {
	p, _ := ctx.Value(principalContextKey{}).(authz.Principal)
	return p
}

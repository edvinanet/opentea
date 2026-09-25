// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package api

import (
	"context"

	"github.com/oej/opentea/internal/authz"
)

// principalContextKey and invalidBearerContextKey are unexported types so
// this package's context keys can't collide with any key set by another
// package -- the standard Go context-key-collision-avoidance idiom.
type principalContextKey struct{}
type invalidBearerContextKey struct{}

// withPrincipal returns a context carrying p, resolved once by
// resolvePrincipal and read by every /tea/v1 handler via
// principalFromContext. Deliberately kept to these two closely-related
// request-scoped values (this and withInvalidBearer below) -- don't grow
// this into a general "context bag" pattern.
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

// withInvalidBearer marks the context as carrying an invalid/expired
// bearer token -- resolvePrincipal sets this instead of rejecting
// outright, since whether that should actually block the request depends
// on whether the resource it turns out to be for requires authentication
// at all (spec: a public endpoint ignores a presented token, valid or
// not; only writeAuthzDenial, once a specific resource's authz decision is
// in, knows that). The principal itself stays anonymous either way --
// this is purely about choosing the right challenge if the request does
// end up denied for requiring authentication.
func withInvalidBearer(ctx context.Context) context.Context {
	return context.WithValue(ctx, invalidBearerContextKey{}, true)
}

// invalidBearerFromContext reports whether withInvalidBearer was set.
func invalidBearerFromContext(ctx context.Context) bool {
	v, _ := ctx.Value(invalidBearerContextKey{}).(bool)
	return v
}

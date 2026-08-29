// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package api

import (
	"net/http"

	"github.com/oej/opentea/internal/authz"
	"github.com/oej/opentea/internal/httpx"
)

// decide runs authz.Decide for the current request's principal, without
// writing any response -- used both by authorize (below) and by list
// endpoints' filterAuthorized closures, which need a plain (bool, error)
// to filter rows with, not an early HTTP response.
func (s *Server) decide(r *http.Request, capability authz.Capability, resource authz.Resource) (bool, error) {
	decision, err := authz.Decide(r.Context(), s.repo, principalFromContext(r.Context()), capability, resource)
	if err != nil {
		return false, err
	}
	return decision.Allowed, nil
}

// authorize checks capability against resource for the current request's
// principal and writes the appropriate response on anything but success:
// a Decide error is treated as fail-closed (spec Sec 24) and rendered as a
// generic 500, exactly like any other internal error; a denial renders as
// 404, never 403 -- unauthorized and nonexistent are deliberately
// indistinguishable on /tea/v1 (spec Sec 18), so a future contributor
// should not "helpfully" add a 403 path here. 403 stays reserved for
// /admin/v1's role-gated writes.
//
// Returns true if the caller should continue handling the request, false
// if authorize has already written a response and the caller must return
// immediately.
func (s *Server) authorize(w http.ResponseWriter, r *http.Request, capability authz.Capability, resource authz.Resource) bool {
	allowed, err := s.decide(r, capability, resource)
	if err != nil {
		httpx.InternalError(w, r, err)
		return false
	}
	if !allowed {
		httpx.NotFound(w)
		return false
	}
	return true
}

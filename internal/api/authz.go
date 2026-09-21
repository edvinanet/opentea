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
// generic 500, exactly like any other internal error; a denial goes
// through writeAuthzDenial below -- 404 for an authenticated-but-
// unauthorized caller (unauthorized and nonexistent stay deliberately
// indistinguishable on /tea/v1, spec Sec 18) or 401 for an anonymous one
// (TEA 1.0's own 401-unauthorized text: "A protected object shall not
// answer 404 solely because the client is unauthenticated"). 403 stays
// reserved for /admin/v1's role-gated writes -- a future contributor
// should not "helpfully" add one here.
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
		s.writeAuthzDenial(w, r)
		return false
	}
	return true
}

// writeAuthzDenial writes the correct response for an authz denial --
// shared by authorize above and discovery.go's own hand-rolled denial
// branch (discovery resolves its resource from a TEI/PURL match rather
// than a path UUID, so it can't call authorize directly, but the response
// rule is identical). TEA 1.0's 401-unauthorized text draws the line on
// whether a valid token was presented at all, not on what it's entitled
// to: "protected endpoints return 401 when no valid token is presented";
// an authenticated-but-unauthorized caller still gets the existing,
// deliberately indistinguishable-from-nonexistent 404 (spec Sec 18,
// 403-forbidden's own text: "servers may instead conceal the existence of
// a resource... by answering 404").
func (s *Server) writeAuthzDenial(w http.ResponseWriter, r *http.Request) {
	if !principalFromContext(r.Context()).IsAuthenticated() {
		httpx.UnauthorizedBearer(w, "", "authentication required")
		return
	}
	httpx.NotFound(w)
}

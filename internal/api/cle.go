// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/oej/opentea/internal/authz"
	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/pkg/tea"
)

func (s *Server) cleByProduct(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequestTyped(w, tea.ErrorInvalidRequest, "invalid uuid")
		return
	}
	if _, err := s.repo.GetProduct(r.Context(), uuid); errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	// lifecycle.history.read, not product.read: capability independence
	// (spec Sec 12.2) means product access must not imply lifecycle access.
	// This endpoint returns the full CLE document (history + current
	// status are not separately addressable in this codebase), so the more
	// restrictive of the two lifecycle capabilities is the one checked.
	if !s.authorize(w, r, authz.CapLifecycleHistoryRead, authz.Resource{ProductUUID: uuid}) {
		return
	}
	s.writeCLE(w, r, repo.OwnerProduct, uuid)
}

func (s *Server) cleByProductRelease(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequestTyped(w, tea.ErrorInvalidRequest, "invalid uuid")
		return
	}
	if _, err := s.repo.GetProductRelease(r.Context(), uuid); errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if !s.authorize(w, r, authz.CapLifecycleHistoryRead, authz.Resource{ProductReleaseUUID: uuid}) {
		return
	}
	s.writeCLE(w, r, repo.OwnerProductRelease, uuid)
}

func (s *Server) cleByComponent(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequestTyped(w, tea.ErrorInvalidRequest, "invalid uuid")
		return
	}
	if _, err := s.repo.GetComponent(r.Context(), uuid); errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if !s.authorize(w, r, authz.CapLifecycleHistoryRead, authz.Resource{ComponentUUID: uuid}) {
		return
	}
	s.writeCLE(w, r, repo.OwnerComponent, uuid)
}

func (s *Server) cleByComponentRelease(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequestTyped(w, tea.ErrorInvalidRequest, "invalid uuid")
		return
	}
	if _, err := s.repo.GetComponentRelease(r.Context(), uuid); errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if !s.authorize(w, r, authz.CapLifecycleHistoryRead, authz.Resource{ComponentReleaseUUID: uuid}) {
		return
	}
	s.writeCLE(w, r, repo.OwnerComponentRelease, uuid)
}

// writeCLE is called only after the caller has already confirmed ownerUUID
// itself exists and is authorized (each of the 4 handlers above does its
// own existence + authz check first), so unlike GetCLE it doesn't need
// either here -- GetCLERevision returns 0 (not an error) for an owner with
// no CLE data yet, matching GetCLE's own "empty CLE is valid" semantics.
func (s *Server) writeCLE(w http.ResponseWriter, r *http.Request, ownerType, ownerUUID string) {
	revision, err := s.repo.GetCLERevision(r.Context(), ownerType, ownerUUID)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if s.conditional(w, r, cacheControlRevalidate, "cle", ownerType, ownerUUID, strconv.FormatInt(revision, 10)) {
		return
	}

	cle, err := s.repo.GetCLE(r.Context(), ownerType, ownerUUID)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, cle)
}

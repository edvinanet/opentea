// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package admin

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/repo"
)

type createReleaseGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func (s *Server) createReleaseGroup(w http.ResponseWriter, r *http.Request) {
	var req createReleaseGroupRequest
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	if req.Name == "" {
		httpx.BadRequest(w, "name is required")
		return
	}

	actor := actorFromContext(r.Context())
	var group model.ReleaseGroup
	err := s.repo.WithTx(r.Context(), func(tx *repo.Repo) error {
		var err error
		group, err = tx.CreateReleaseGroup(r.Context(), req.Name, req.Description)
		if err != nil {
			return err
		}
		return auditWrite(r.Context(), tx, actor.UUID, requestIDFrom(r), "releaseGroup.create", "release_group", group.UUID, nil, group)
	})
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, group)
}

func (s *Server) listReleaseGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.repo.ListReleaseGroups(r.Context())
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, groups)
}

type releaseGroupResponse struct {
	model.ReleaseGroup
	Members []string `json:"members"`
}

func (s *Server) getReleaseGroup(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	g, err := s.repo.GetReleaseGroup(r.Context(), uuid)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	members, err := s.repo.ListReleaseGroupMembers(r.Context(), uuid)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, releaseGroupResponse{ReleaseGroup: g, Members: members})
}

func (s *Server) deleteReleaseGroup(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}

	actor := actorFromContext(r.Context())
	err = s.repo.WithTx(r.Context(), func(tx *repo.Repo) error {
		before, err := tx.GetReleaseGroup(r.Context(), uuid)
		if err != nil {
			return err
		}
		if err := tx.DeleteReleaseGroup(r.Context(), uuid); err != nil {
			return err
		}
		return auditWrite(r.Context(), tx, actor.UUID, requestIDFrom(r), "releaseGroup.delete", "release_group", uuid, before, nil)
	})
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type releaseGroupMemberRequest struct {
	ProductReleaseUUID string `json:"productReleaseUuid"`
}

func (s *Server) addReleaseGroupMember(w http.ResponseWriter, r *http.Request) {
	groupUUID, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	var req releaseGroupMemberRequest
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	if req.ProductReleaseUUID == "" {
		httpx.BadRequest(w, "productReleaseUuid is required")
		return
	}

	actor := actorFromContext(r.Context())
	err = s.repo.WithTx(r.Context(), func(tx *repo.Repo) error {
		if err := tx.AddReleaseGroupMember(r.Context(), groupUUID, req.ProductReleaseUUID); err != nil {
			return err
		}
		return auditWrite(r.Context(), tx, actor.UUID, requestIDFrom(r), "releaseGroup.memberAdd", "release_group", groupUUID, nil, req)
	})
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeReleaseGroupMember(w http.ResponseWriter, r *http.Request) {
	groupUUID, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	releaseUUID, err := httpx.PathUUID(r, "releaseUuid")
	if err != nil {
		httpx.BadRequest(w, "invalid releaseUuid")
		return
	}

	actor := actorFromContext(r.Context())
	err = s.repo.WithTx(r.Context(), func(tx *repo.Repo) error {
		if err := tx.RemoveReleaseGroupMember(r.Context(), groupUUID, releaseUUID); err != nil {
			return err
		}
		return auditWrite(r.Context(), tx, actor.UUID, requestIDFrom(r), "releaseGroup.memberRemove", "release_group", groupUUID, releaseGroupMemberRequest{ProductReleaseUUID: releaseUUID}, nil)
	})
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

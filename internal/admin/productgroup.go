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

type createProductGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func (s *Server) createProductGroup(w http.ResponseWriter, r *http.Request) {
	var req createProductGroupRequest
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	if req.Name == "" {
		httpx.BadRequest(w, "name is required")
		return
	}

	actor := actorFromContext(r.Context())
	var group model.ProductGroup
	err := s.repo.WithTx(r.Context(), func(tx *repo.Repo) error {
		var err error
		group, err = tx.CreateProductGroup(r.Context(), req.Name, req.Description)
		if err != nil {
			return err
		}
		return auditWrite(r.Context(), tx, actor.UUID, requestIDFrom(r), "productGroup.create", "product_group", group.UUID, nil, group)
	})
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, group)
}

func (s *Server) listProductGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.repo.ListProductGroups(r.Context())
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, groups)
}

type productGroupResponse struct {
	model.ProductGroup
	Members []string `json:"members"`
}

func (s *Server) getProductGroup(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	g, err := s.repo.GetProductGroup(r.Context(), uuid)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	members, err := s.repo.ListProductGroupMembers(r.Context(), uuid)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, productGroupResponse{ProductGroup: g, Members: members})
}

func (s *Server) deleteProductGroup(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}

	actor := actorFromContext(r.Context())
	err = s.repo.WithTx(r.Context(), func(tx *repo.Repo) error {
		before, err := tx.GetProductGroup(r.Context(), uuid)
		if err != nil {
			return err
		}
		if err := tx.DeleteProductGroup(r.Context(), uuid); err != nil {
			return err
		}
		return auditWrite(r.Context(), tx, actor.UUID, requestIDFrom(r), "productGroup.delete", "product_group", uuid, before, nil)
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

type productGroupMemberRequest struct {
	ProductUUID string `json:"productUuid"`
}

func (s *Server) addProductGroupMember(w http.ResponseWriter, r *http.Request) {
	groupUUID, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	var req productGroupMemberRequest
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	if req.ProductUUID == "" {
		httpx.BadRequest(w, "productUuid is required")
		return
	}

	actor := actorFromContext(r.Context())
	err = s.repo.WithTx(r.Context(), func(tx *repo.Repo) error {
		if err := tx.AddProductGroupMember(r.Context(), groupUUID, req.ProductUUID); err != nil {
			return err
		}
		return auditWrite(r.Context(), tx, actor.UUID, requestIDFrom(r), "productGroup.memberAdd", "product_group", groupUUID, nil, req)
	})
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeProductGroupMember(w http.ResponseWriter, r *http.Request) {
	groupUUID, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	productUUID, err := httpx.PathUUID(r, "productUuid")
	if err != nil {
		httpx.BadRequest(w, "invalid productUuid")
		return
	}

	actor := actorFromContext(r.Context())
	err = s.repo.WithTx(r.Context(), func(tx *repo.Repo) error {
		if err := tx.RemoveProductGroupMember(r.Context(), groupUUID, productUUID); err != nil {
			return err
		}
		return auditWrite(r.Context(), tx, actor.UUID, requestIDFrom(r), "productGroup.memberRemove", "product_group", groupUUID, productGroupMemberRequest{ProductUUID: productUUID}, nil)
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

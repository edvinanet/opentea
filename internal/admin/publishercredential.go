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

// createPublisherCredentialRequest is /publisher/v1's Layer B/D bearer
// credential issuance request (design/publisher-service.md §10.2/§10.4) --
// an operator action, not part of the standard protocol, hence it lives
// here under /admin/v1 rather than in internal/publisher. See
// internal/publisher's package doc comment for what "full"/"cicd" may do.
type createPublisherCredentialRequest struct {
	Label string `json:"label"`
	Scope string `json:"scope"`
}

// createPublisherCredentialResponse embeds the raw token alongside the
// stored record -- the only time it's ever available; GET never returns
// it (see model.PublisherCredential's own json tags, which have no token
// field at all).
type createPublisherCredentialResponse struct {
	model.PublisherCredential
	Token string `json:"token"`
}

func (s *Server) createPublisherCredential(w http.ResponseWriter, r *http.Request) {
	var req createPublisherCredentialRequest
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	if req.Label == "" {
		httpx.BadRequest(w, "label is required")
		return
	}
	if req.Scope != model.PublisherScopeFull && req.Scope != model.PublisherScopeCICD {
		httpx.BadRequest(w, "scope must be \"full\" or \"cicd\"")
		return
	}

	actor := actorFromContext(r.Context())
	var resp createPublisherCredentialResponse
	err := s.repo.WithTx(r.Context(), func(tx *repo.Repo) error {
		cred, token, err := tx.CreatePublisherCredential(r.Context(), req.Label, req.Scope)
		if err != nil {
			return err
		}
		resp = createPublisherCredentialResponse{PublisherCredential: cred, Token: token}
		return auditWrite(r.Context(), tx, actor.UUID, requestIDFrom(r), "publisherCredential.create", "publisher_credential", cred.UUID, nil, cred)
	})
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (s *Server) listPublisherCredentials(w http.ResponseWriter, r *http.Request) {
	creds, err := s.repo.ListPublisherCredentials(r.Context())
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, creds)
}

func (s *Server) revokePublisherCredential(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}

	actor := actorFromContext(r.Context())
	err = s.repo.WithTx(r.Context(), func(tx *repo.Repo) error {
		if err := tx.RevokePublisherCredential(r.Context(), uuid); err != nil {
			return err
		}
		return auditWrite(r.Context(), tx, actor.UUID, requestIDFrom(r), "publisherCredential.revoke", "publisher_credential", uuid, nil, nil)
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

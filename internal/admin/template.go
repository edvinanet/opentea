// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package admin

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/authz"
	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/repo"
)

// capabilityRuleRequest is the JSON shape of one rule in a template create/
// revision request body -- validated against validCapabilities/
// validArtifactTypes/validDecisions (authz_validate.go) before it ever
// reaches the repo layer.
type capabilityRuleRequest struct {
	Capability   authz.Capability   `json:"capability"`
	ArtifactType authz.ArtifactType `json:"artifactType,omitempty"`
	Decision     string             `json:"decision"`
}

func toModelRules(rules []capabilityRuleRequest) []model.TemplateCapabilityRule {
	out := make([]model.TemplateCapabilityRule, len(rules))
	for i, r := range rules {
		out[i] = model.TemplateCapabilityRule{
			Capability:   r.Capability,
			ArtifactType: r.ArtifactType,
			Decision:     r.Decision,
		}
	}
	return out
}

type createTemplateRequest struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description,omitempty"`
	Comment     string                  `json:"comment,omitempty"`
	Rules       []capabilityRuleRequest `json:"rules"`
}

type createTemplateResponse struct {
	model.Template
	Revision model.TemplateRevision `json:"revision"`
}

func (s *Server) createTemplate(w http.ResponseWriter, r *http.Request) {
	var req createTemplateRequest
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	if req.Name == "" {
		httpx.BadRequest(w, "name is required")
		return
	}
	if msg := validateRules(req.Rules); msg != "" {
		httpx.BadRequest(w, msg)
		return
	}

	actor := actorFromContext(r.Context())
	var resp createTemplateResponse
	err := s.repo.WithTx(r.Context(), func(tx *repo.Repo) error {
		tmpl, rev, err := tx.CreateTemplate(r.Context(), req.Name, req.Description, toModelRules(req.Rules), actor.UUID, req.Comment)
		if err != nil {
			return err
		}
		if err := auditWrite(r.Context(), tx, actor.UUID, requestIDFrom(r), "template.create", "template", tmpl.UUID, nil, tmpl); err != nil {
			return err
		}
		resp = createTemplateResponse{Template: tmpl, Revision: rev}
		return nil
	})
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := s.repo.ListTemplates(r.Context())
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, templates)
}

type templateResponse struct {
	model.Template
	ActiveRules []model.TemplateCapabilityRule `json:"activeRules,omitempty"`
}

func (s *Server) getTemplate(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	tmpl, err := s.repo.GetTemplate(r.Context(), uuid)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}

	resp := templateResponse{Template: tmpl}
	if tmpl.ActiveRevision > 0 {
		rev, err := s.repo.GetTemplateRevision(r.Context(), uuid, tmpl.ActiveRevision)
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		resp.ActiveRules = rev.Rules
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

type createTemplateRevisionRequest struct {
	Rules   []capabilityRuleRequest `json:"rules"`
	Comment string                  `json:"comment,omitempty"`
}

func (s *Server) createTemplateRevision(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	var req createTemplateRevisionRequest
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	if msg := validateRules(req.Rules); msg != "" {
		httpx.BadRequest(w, msg)
		return
	}

	actor := actorFromContext(r.Context())
	var rev model.TemplateRevision
	err = s.repo.WithTx(r.Context(), func(tx *repo.Repo) error {
		var err error
		rev, err = tx.CreateTemplateRevision(r.Context(), uuid, toModelRules(req.Rules), actor.UUID, req.Comment)
		if err != nil {
			return err
		}
		return auditWrite(r.Context(), tx, actor.UUID, requestIDFrom(r), "template.reviseCreate", "template", uuid, nil, rev)
	})
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, rev)
}

type activateTemplateRevisionRequest struct {
	Revision int `json:"revision"`
}

func (s *Server) activateTemplateRevision(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	var req activateTemplateRevisionRequest
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	if req.Revision < 1 {
		httpx.BadRequest(w, "revision must be >= 1")
		return
	}

	actor := actorFromContext(r.Context())
	var updated model.Template
	err = s.repo.WithTx(r.Context(), func(tx *repo.Repo) error {
		before, err := tx.GetTemplate(r.Context(), uuid)
		if err != nil {
			return err
		}
		if err := tx.ActivateTemplateRevision(r.Context(), uuid, req.Revision); err != nil {
			return err
		}
		updated, err = tx.GetTemplate(r.Context(), uuid)
		if err != nil {
			return err
		}
		return auditWrite(r.Context(), tx, actor.UUID, requestIDFrom(r), "template.activate", "template", uuid, before, updated)
	})
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}

	actor := actorFromContext(r.Context())
	err = s.repo.WithTx(r.Context(), func(tx *repo.Repo) error {
		before, err := tx.GetTemplate(r.Context(), uuid)
		if err != nil {
			return err
		}
		if err := tx.DeleteTemplate(r.Context(), uuid); err != nil {
			return err
		}
		return auditWrite(r.Context(), tx, actor.UUID, requestIDFrom(r), "template.delete", "template", uuid, before, nil)
	})
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if errors.Is(err, repo.ErrTemplateInUse) {
		httpx.WriteJSON(w, http.StatusConflict, struct {
			Message string `json:"message"`
		}{"template is referenced by at least one entitlement"})
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

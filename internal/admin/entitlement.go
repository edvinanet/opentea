package admin

import (
	"errors"
	"net/http"
	"time"

	"github.com/oej/opentea/internal/authz"
	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/repo"
)

type createEntitlementRequest struct {
	SubjectType       authz.SubjectType `json:"subjectType"`
	SubjectID         string            `json:"subjectId,omitempty"`
	TemplateUUID      string            `json:"templateUuid"`
	TemplateRevision  int               `json:"templateRevision"`
	ResourceType      string            `json:"resourceType"`
	ResourceID        string            `json:"resourceId,omitempty"`
	ValidFrom         *time.Time        `json:"validFrom,omitempty"`
	ValidUntil        *time.Time        `json:"validUntil,omitempty"`
	GrantingAuthority string            `json:"grantingAuthority"`
}

func (req createEntitlementRequest) validate() string {
	if !validSubjectTypes[req.SubjectType] {
		return "invalid subjectType"
	}
	if req.SubjectType == authz.SubjectPrincipal && req.SubjectID == "" {
		return "subjectId is required when subjectType is \"principal\""
	}
	if req.SubjectType != authz.SubjectPrincipal && req.SubjectID != "" {
		return "subjectId must be empty unless subjectType is \"principal\""
	}
	if req.TemplateUUID == "" {
		return "templateUuid is required"
	}
	if req.TemplateRevision < 1 {
		return "templateRevision must be >= 1"
	}
	if !validResourceTypes[req.ResourceType] {
		return "invalid resourceType"
	}
	if req.ResourceType == model.ResourceAllProducts && req.ResourceID != "" {
		return "resourceId must be empty when resourceType is \"all_products\""
	}
	if req.ResourceType != model.ResourceAllProducts && req.ResourceID == "" {
		return "resourceId is required unless resourceType is \"all_products\""
	}
	if req.GrantingAuthority == "" {
		return "grantingAuthority is required"
	}
	return ""
}

func (s *Server) createEntitlement(w http.ResponseWriter, r *http.Request) {
	var req createEntitlementRequest
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	if msg := req.validate(); msg != "" {
		httpx.BadRequest(w, msg)
		return
	}

	actor := actorFromContext(r.Context())
	var created model.Entitlement
	err := s.repo.WithTx(r.Context(), func(tx *repo.Repo) error {
		var err error
		created, err = tx.CreateEntitlement(r.Context(), repo.EntitlementInput{
			SubjectType:       req.SubjectType,
			SubjectID:         req.SubjectID,
			TemplateUUID:      req.TemplateUUID,
			TemplateRevision:  req.TemplateRevision,
			ResourceType:      req.ResourceType,
			ResourceID:        req.ResourceID,
			ValidFrom:         req.ValidFrom,
			ValidUntil:        req.ValidUntil,
			GrantingAuthority: req.GrantingAuthority,
		}, actor.UUID)
		if err != nil {
			return err
		}
		return auditWrite(r.Context(), tx, actor.UUID, requestIDFrom(r), "entitlement.create", "entitlement", created.UUID, nil, created)
	})
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, created)
}

func (s *Server) listEntitlements(w http.ResponseWriter, r *http.Request) {
	filter := repo.EntitlementFilter{
		SubjectType:  r.URL.Query().Get("subjectType"),
		SubjectID:    r.URL.Query().Get("subjectId"),
		ResourceType: r.URL.Query().Get("resourceType"),
		ResourceID:   r.URL.Query().Get("resourceId"),
	}
	entitlements, err := s.repo.ListEntitlements(r.Context(), filter)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, entitlements)
}

func (s *Server) getEntitlement(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	e, err := s.repo.GetEntitlement(r.Context(), uuid)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.NotFound(w)
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, e)
}

type updateEntitlementStatusRequest struct {
	Status string `json:"status"`
}

func (s *Server) updateEntitlementStatus(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	var req updateEntitlementStatusRequest
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	switch req.Status {
	case model.EntitlementActive, model.EntitlementSuspended, model.EntitlementRevoked:
	default:
		httpx.BadRequest(w, "status must be \"active\", \"suspended\", or \"revoked\"")
		return
	}

	actor := actorFromContext(r.Context())
	var updated model.Entitlement
	err = s.repo.WithTx(r.Context(), func(tx *repo.Repo) error {
		before, err := tx.GetEntitlement(r.Context(), uuid)
		if err != nil {
			return err
		}
		updated, err = tx.UpdateEntitlementStatus(r.Context(), uuid, req.Status)
		if err != nil {
			return err
		}
		return auditWrite(r.Context(), tx, actor.UUID, requestIDFrom(r), "entitlement.statusChange", "entitlement", uuid, before, updated)
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

type updateEntitlementValidityRequest struct {
	ValidFrom  *time.Time `json:"validFrom,omitempty"`
	ValidUntil *time.Time `json:"validUntil,omitempty"`
}

func (s *Server) updateEntitlementValidity(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}
	var req updateEntitlementValidityRequest
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid JSON body: "+err.Error())
		return
	}

	actor := actorFromContext(r.Context())
	var updated model.Entitlement
	err = s.repo.WithTx(r.Context(), func(tx *repo.Repo) error {
		before, err := tx.GetEntitlement(r.Context(), uuid)
		if err != nil {
			return err
		}
		updated, err = tx.UpdateEntitlementValidity(r.Context(), uuid, req.ValidFrom, req.ValidUntil)
		if err != nil {
			return err
		}
		return auditWrite(r.Context(), tx, actor.UUID, requestIDFrom(r), "entitlement.validityChange", "entitlement", uuid, before, updated)
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

func (s *Server) deleteEntitlement(w http.ResponseWriter, r *http.Request) {
	uuid, err := httpx.PathUUID(r, "uuid")
	if err != nil {
		httpx.BadRequest(w, "invalid uuid")
		return
	}

	actor := actorFromContext(r.Context())
	err = s.repo.WithTx(r.Context(), func(tx *repo.Repo) error {
		before, err := tx.GetEntitlement(r.Context(), uuid)
		if err != nil {
			return err
		}
		if err := tx.DeleteEntitlement(r.Context(), uuid); err != nil {
			return err
		}
		return auditWrite(r.Context(), tx, actor.UUID, requestIDFrom(r), "entitlement.delete", "entitlement", uuid, before, nil)
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

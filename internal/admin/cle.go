package admin

import (
	"errors"
	"net/http"
	"time"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/pkg/tea"
)

type createCLEEventRequest struct {
	Type                string                    `json:"type"`
	Effective           time.Time                 `json:"effective"`
	Published           time.Time                 `json:"published"`
	Version             string                    `json:"version,omitempty"`
	Versions            []tea.CLEVersionSpecifier `json:"versions,omitempty"`
	SupportID           string                    `json:"supportId,omitempty"`
	License             string                    `json:"license,omitempty"`
	SupersededByVersion string                    `json:"supersededByVersion,omitempty"`
	Identifiers         []tea.Identifier          `json:"identifiers,omitempty"`
	EventID             *int                      `json:"eventId,omitempty"`
	Reason              string                    `json:"reason,omitempty"`
	Description         string                    `json:"description,omitempty"`
	References          []string                  `json:"references,omitempty"`
}

func (s *Server) createCLEEventForOwner(ownerType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ownerUUID, err := httpx.PathUUID(r, "uuid")
		if err != nil {
			httpx.BadRequest(w, "invalid uuid")
			return
		}
		var req createCLEEventRequest
		if err := decodeJSON(r, &req); err != nil {
			httpx.BadRequest(w, "invalid JSON body: "+err.Error())
			return
		}
		if req.Type == "" {
			httpx.BadRequest(w, "type is required")
			return
		}
		if req.Effective.IsZero() || req.Published.IsZero() {
			httpx.BadRequest(w, "effective and published are required")
			return
		}

		e, err := s.repo.CreateCLEEvent(r.Context(), ownerType, ownerUUID, repo.CLEEventInput{
			Type:                req.Type,
			Effective:           req.Effective,
			Published:           req.Published,
			Version:             req.Version,
			Versions:            req.Versions,
			SupportID:           req.SupportID,
			License:             req.License,
			SupersededByVersion: req.SupersededByVersion,
			Identifiers:         req.Identifiers,
			EventID:             req.EventID,
			Reason:              req.Reason,
			Description:         req.Description,
			References:          req.References,
		})
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, e)
	}
}

func (s *Server) createCLEDefinitionForOwner(ownerType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ownerUUID, err := httpx.PathUUID(r, "uuid")
		if err != nil {
			httpx.BadRequest(w, "invalid uuid")
			return
		}
		var def tea.CLESupportDefinition
		if err := decodeJSON(r, &def); err != nil {
			httpx.BadRequest(w, "invalid JSON body: "+err.Error())
			return
		}
		if def.ID == "" || def.Description == "" {
			httpx.BadRequest(w, "id and description are required")
			return
		}

		created, err := s.repo.CreateCLESupportDefinition(r.Context(), ownerType, ownerUUID, def)
		if errors.Is(err, repo.ErrNotFound) {
			httpx.NotFound(w)
			return
		}
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, created)
	}
}

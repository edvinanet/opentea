// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/httpx"
)

// auditableTarget is Target minus BearerToken -- audit_log.resulting_state
// shouldn't duplicate a live credential into a second table just to
// record that a target was added.
type auditableTarget struct {
	UUID      string
	Label     string
	BaseURL   string
	CreatedAt string
}

func redact(t Target) auditableTarget {
	return auditableTarget{UUID: t.UUID, Label: t.Label, BaseURL: t.BaseURL, CreatedAt: formatTime(t.CreatedAt)}
}

func (s *Server) createTargetForm(w http.ResponseWriter, r *http.Request, staff Staff) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	label := r.PostFormValue("label")
	baseURL := r.PostFormValue("baseUrl")
	bearerToken := r.PostFormValue("bearerToken")

	if label == "" || baseURL == "" || bearerToken == "" {
		s.rerenderDashboardWithError(w, r, staff, "label, base URL, and bearer token are all required")
		return
	}

	err := s.repo.WithTx(r.Context(), func(tx *Repo) error {
		created, err := tx.CreateTarget(r.Context(), label, baseURL, bearerToken)
		if err != nil {
			return err
		}
		return tx.RecordAudit(r.Context(), AuditInput{
			StaffUUID: staff.UUID, RequestID: httpx.RequestID(r.Context()),
			Operation: "target.create", TargetType: "target", TargetID: created.UUID,
			Resulting: redact(created),
		})
	})
	if err != nil {
		s.rerenderDashboardWithError(w, r, staff, "internal error, please try again")
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) deleteTargetForm(w http.ResponseWriter, r *http.Request, staff Staff) {
	uuid := r.PathValue("uuid")

	existing, err := s.repo.GetTarget(r.Context(), uuid)
	if errors.Is(err, ErrNotFound) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err != nil {
		s.rerenderDashboardWithError(w, r, staff, "internal error, please try again")
		return
	}

	err = s.repo.WithTx(r.Context(), func(tx *Repo) error {
		if err := tx.DeleteTarget(r.Context(), uuid); err != nil {
			return err
		}
		return tx.RecordAudit(r.Context(), AuditInput{
			StaffUUID: staff.UUID, RequestID: httpx.RequestID(r.Context()),
			Operation: "target.delete", TargetType: "target", TargetID: uuid,
			Previous: redact(existing), Resulting: struct{}{},
		})
	})
	if err != nil {
		s.rerenderDashboardWithError(w, r, staff, "internal error, please try again")
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) rerenderDashboardWithError(w http.ResponseWriter, r *http.Request, staff Staff, message string) {
	targets, err := s.repo.ListTargets(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.renderAuthenticated(w, "dashboard", pageData{Staff: staff, Targets: targets, Error: message})
}

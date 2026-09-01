// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/httpx"
)

// auditableCICDCredential is CICDCredential minus nothing secret to
// redact (the token itself never lives on this struct -- only its hash is
// stored, and CreateCICDCredential's raw return value is never passed to
// RecordAudit) -- named for symmetry with targets.go/staffmanagement.go's
// own auditable* convention.
type auditableCICDCredential struct {
	UUID       string
	TargetUUID string
	Label      string
}

func auditableCredential(c CICDCredential) auditableCICDCredential {
	return auditableCICDCredential{UUID: c.UUID, TargetUUID: c.TargetUUID, Label: c.Label}
}

func (s *Server) cicdCredentialsPage(w http.ResponseWriter, r *http.Request, staff Staff) {
	s.renderCICDCredentialsPage(w, r, staff, "", "")
}

func (s *Server) createCICDCredentialForm(w http.ResponseWriter, r *http.Request, staff Staff) {
	if err := r.ParseForm(); err != nil {
		s.renderCICDCredentialsPage(w, r, staff, "", "invalid form submission")
		return
	}
	targetUUID := r.PostFormValue("targetUuid")
	label := r.PostFormValue("label")
	if targetUUID == "" || label == "" {
		s.renderCICDCredentialsPage(w, r, staff, "", "target and label are required")
		return
	}

	var created CICDCredential
	var token string
	err := s.repo.WithTx(r.Context(), func(tx *Repo) error {
		var err error
		created, token, err = tx.CreateCICDCredential(r.Context(), targetUUID, label)
		if err != nil {
			return err
		}
		return tx.RecordAudit(r.Context(), AuditInput{
			StaffUUID: staff.UUID, RequestID: httpx.RequestID(r.Context()),
			Operation: "cicd_credential.create", TargetType: "cicd_credential", TargetID: created.UUID,
			Resulting: auditableCredential(created),
		})
	})
	if errors.Is(err, ErrNotFound) {
		s.renderCICDCredentialsPage(w, r, staff, "", "unknown target")
		return
	}
	if err != nil {
		s.renderCICDCredentialsPage(w, r, staff, "", "internal error, please try again")
		return
	}

	// Unlike every other create-form in this app, the raw token can only
	// ever be shown once -- it's never stored, only its hash is (see
	// CreateCICDCredential) -- so this renders the page directly instead
	// of redirecting, with the one-time value inline.
	s.renderCICDCredentialsPage(w, r, staff, token, "")
}

func (s *Server) revokeCICDCredentialForm(w http.ResponseWriter, r *http.Request, staff Staff) {
	uuid := r.PathValue("uuid")

	existing, err := s.repo.ListCICDCredentials(r.Context(), "")
	if err != nil {
		s.renderCICDCredentialsPage(w, r, staff, "", "internal error, please try again")
		return
	}
	var previous *CICDCredential
	for i := range existing {
		if existing[i].UUID == uuid {
			previous = &existing[i]
			break
		}
	}
	if previous == nil {
		http.Redirect(w, r, "/cicd-credentials", http.StatusSeeOther)
		return
	}

	err = s.repo.WithTx(r.Context(), func(tx *Repo) error {
		if err := tx.RevokeCICDCredential(r.Context(), uuid); err != nil {
			return err
		}
		return tx.RecordAudit(r.Context(), AuditInput{
			StaffUUID: staff.UUID, RequestID: httpx.RequestID(r.Context()),
			Operation: "cicd_credential.revoke", TargetType: "cicd_credential", TargetID: uuid,
			Previous: auditableCredential(*previous), Resulting: struct{}{},
		})
	})
	if err != nil {
		s.renderCICDCredentialsPage(w, r, staff, "", "internal error, please try again")
		return
	}
	http.Redirect(w, r, "/cicd-credentials", http.StatusSeeOther)
}

func (s *Server) renderCICDCredentialsPage(w http.ResponseWriter, r *http.Request, staff Staff, createdToken, message string) {
	credentials, err := s.repo.ListCICDCredentials(r.Context(), "")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	targets, err := s.repo.ListTargets(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	targetLabels := make(map[string]string, len(targets))
	for _, t := range targets {
		targetLabels[t.UUID] = t.Label
	}
	s.renderAuthenticated(w, "cicdcredentials", pageData{
		Staff: staff, Error: message,
		Targets: targets, CICDCredentials: credentials, TargetLabels: targetLabels,
		CreatedCICDToken: createdToken,
	})
}

// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"errors"
	"net/http"

	"github.com/oej/opentea/internal/httpx"
)

// auditableStaff is Staff minus nothing -- unlike auditableTarget
// (targets.go), Staff has no secret field to redact (the password hash
// never leaves staff.go).
type auditableStaff struct {
	UUID      string
	Username  string
	Role      string
	CreatedAt string
}

func redactStaff(s Staff) auditableStaff {
	return auditableStaff{UUID: s.UUID, Username: s.Username, Role: s.Role, CreatedAt: formatTime(s.CreatedAt)}
}

func (s *Server) staffPage(w http.ResponseWriter, r *http.Request, staff Staff) {
	list, err := s.repo.ListStaff(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.renderAuthenticated(w, "staff", pageData{Staff: staff, StaffList: list})
}

func (s *Server) createStaffForm(w http.ResponseWriter, r *http.Request, staff Staff) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/staff", http.StatusSeeOther)
		return
	}
	username := r.PostFormValue("username")
	password := r.PostFormValue("password")
	role := r.PostFormValue("role")

	if username == "" || password == "" {
		s.rerenderStaffWithError(w, r, staff, "username and password are required")
		return
	}

	var created Staff
	err := s.repo.WithTx(r.Context(), func(tx *Repo) error {
		var err error
		created, err = tx.CreateStaff(r.Context(), username, password, role)
		if err != nil {
			return err
		}
		return tx.RecordAudit(r.Context(), AuditInput{
			StaffUUID: staff.UUID, RequestID: httpx.RequestID(r.Context()),
			Operation: "staff.create", TargetType: "staff", TargetID: created.UUID,
			Resulting: redactStaff(created),
		})
	})
	if errors.Is(err, ErrUsernameTaken) {
		s.rerenderStaffWithError(w, r, staff, "username already taken")
		return
	}
	if errors.Is(err, ErrPasswordTooShort) || errors.Is(err, ErrInvalidRole) {
		s.rerenderStaffWithError(w, r, staff, err.Error())
		return
	}
	if err != nil {
		s.rerenderStaffWithError(w, r, staff, "internal error, please try again")
		return
	}
	http.Redirect(w, r, "/staff", http.StatusSeeOther)
}

func (s *Server) deleteStaffForm(w http.ResponseWriter, r *http.Request, staff Staff) {
	uuid := r.PathValue("uuid")

	existing, err := s.repo.GetStaffByUUID(r.Context(), uuid)
	if errors.Is(err, ErrNotFound) {
		http.Redirect(w, r, "/staff", http.StatusSeeOther)
		return
	}
	if err != nil {
		s.rerenderStaffWithError(w, r, staff, "internal error, please try again")
		return
	}

	err = s.repo.WithTx(r.Context(), func(tx *Repo) error {
		if err := tx.DeleteStaff(r.Context(), uuid); err != nil {
			return err
		}
		return tx.RecordAudit(r.Context(), AuditInput{
			StaffUUID: staff.UUID, RequestID: httpx.RequestID(r.Context()),
			Operation: "staff.delete", TargetType: "staff", TargetID: uuid,
			Previous: redactStaff(existing), Resulting: struct{}{},
		})
	})
	if errors.Is(err, ErrLastAdmin) {
		s.rerenderStaffWithError(w, r, staff, "cannot delete the last admin account")
		return
	}
	if err != nil {
		s.rerenderStaffWithError(w, r, staff, "internal error, please try again")
		return
	}
	http.Redirect(w, r, "/staff", http.StatusSeeOther)
}

func (s *Server) rerenderStaffWithError(w http.ResponseWriter, r *http.Request, staff Staff, message string) {
	list, err := s.repo.ListStaff(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.renderAuthenticated(w, "staff", pageData{Staff: staff, StaffList: list, Error: message})
}

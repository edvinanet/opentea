// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"log/slog"
	"net/http"
)

// pageData is the single data type passed to every template -- only the
// fields relevant to a given page are populated. Mirrors
// internal/webadmin/render.go's own pageData convention.
type pageData struct {
	Staff Staff
	Error string

	Targets []Target
}

func (s *Server) renderAuthenticated(w http.ResponseWriter, page string, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates[page].ExecuteTemplate(w, "layout", data); err != nil {
		slog.Error("render template failed", "page", page, "error", err)
	}
}

func (s *Server) renderLogin(w http.ResponseWriter, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates["login"].ExecuteTemplate(w, "login", data); err != nil {
		slog.Error("render login template failed", "error", err)
	}
}

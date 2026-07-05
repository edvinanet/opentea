package webadmin

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/oej/opentea/internal/model"
)

// pageData is the single data type passed to every template -- only the
// fields relevant to a given page are populated. Small enough a GUI that
// per-page types would be pure overhead.
type pageData struct {
	User    model.User
	Error   string
	RootURL string
	OrgName string

	Stats model.Stats

	Users []model.User

	TokenInfo *time.Time // nil if the user has no API token yet
	NewToken  string     // set only right after generation; shown once
}

func (s *Server) renderAuthenticated(w http.ResponseWriter, page string, data pageData) {
	data.RootURL = s.cfg.RootURL
	data.OrgName = s.cfg.OrgName
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates[page].ExecuteTemplate(w, "layout", data); err != nil {
		slog.Error("render template failed", "page", page, "error", err)
	}
}

func (s *Server) renderLogin(w http.ResponseWriter, data pageData) {
	data.OrgName = s.cfg.OrgName
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates["login"].ExecuteTemplate(w, "login", data); err != nil {
		slog.Error("render login template failed", "error", err)
	}
}

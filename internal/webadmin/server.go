// Package webadmin implements the browser-based admin GUI (mounted at
// /admin/ui/...), server-rendered with html/template. It's a thin
// presentation layer over the same internal/repo methods the JSON
// /admin/v1 API (internal/admin) uses.
package webadmin

import (
	"embed"
	"html/template"
	"net/http"

	"github.com/oej/opentea/internal/config"
	"github.com/oej/opentea/internal/repo"
)

//go:embed templates/*.html
var templatesFS embed.FS

type Server struct {
	repo      *repo.Repo
	cfg       config.Config
	templates map[string]*template.Template
}

// NewRouter builds the /admin/ui/... mux.
func NewRouter(r *repo.Repo, cfg config.Config) http.Handler {
	srv := &Server{repo: r, cfg: cfg, templates: loadTemplates()}
	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	return mux
}

// loadTemplates parses each page against the shared layout, in its own
// isolated template set -- keeping the "content" block name reusable
// across pages without collisions. login.html stands alone (no nav/layout,
// since there's no logged-in user to show it for).
func loadTemplates() map[string]*template.Template {
	out := map[string]*template.Template{
		"login": template.Must(template.ParseFS(templatesFS, "templates/login.html")),
	}
	for _, page := range []string{"dashboard", "users", "token"} {
		out[page] = template.Must(template.ParseFS(templatesFS, "templates/layout.html", "templates/"+page+".html"))
	}
	return out
}

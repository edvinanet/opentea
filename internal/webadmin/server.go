// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Package webadmin implements the browser-based admin GUI (mounted at
// /admin/ui/...), server-rendered with html/template. It's a thin
// presentation layer over the same internal/repo methods the JSON
// /admin/v1 API (internal/admin) uses.
package webadmin

import (
	"embed"
	"html/template"
	"net/http"
	"time"

	"github.com/oej/opentea/internal/config"
	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
)

// maxWebadminBody bounds every /admin/ui request body -- generous for the
// small HTML forms this package actually has (login, logout, create-user,
// delete-user, generate-token), far below the 1 GiB upload/import caps
// internal/admin uses for its own, unrelated large-file routes. A var (not
// const), matching internal/bundle's maxZipEntrySize convention, so tests
// can temporarily lower it instead of generating a 64 KiB request body.
var maxWebadminBody int64 = 64 << 10 // 64 KiB

// webadminBodyReadTimeout bounds how long reading a request body may take
// -- cmd/opentea/main.go's top-level http.Server deliberately leaves
// ReadTimeout at zero (needed for /admin/v1's large uploads, a fully
// separate handler tree from this one), so without this an unauthenticated
// client could hold a connection open indefinitely via a slow-drip request
// body (the connection isn't idle, so IdleTimeout doesn't help either). A
// var so tests can lower it rather than waiting out the real 10s.
var webadminBodyReadTimeout = 10 * time.Second

// limitBody bounds every /admin/ui request's body size and read time -- see
// maxWebadminBody/webadminBodyReadTimeout's doc comments for why.
//
// NOTE: webadmin has no file-upload routes today -- if one is ever added
// here, it needs its own larger/slower-body exemption from this wrapper
// (see internal/admin/upload.go's maxUploadBody for the pattern used on
// the /admin/v1 side), or it will fail confusingly against these small-form
// limits.
func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxWebadminBody)
		_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(webadminBodyReadTimeout))
		next.ServeHTTP(w, r)
	})
}

//go:embed templates/*.html
var templatesFS embed.FS

// Server holds the dependencies for the /admin/ui browser GUI handlers.
type Server struct {
	repo         *repo.Repo
	cfg          config.Config
	templates    map[string]*template.Template
	loginLimiter *httpx.LoginLimiter
}

// NewRouter builds the /admin/ui/... mux.
func NewRouter(r *repo.Repo, cfg config.Config) http.Handler {
	srv := &Server{repo: r, cfg: cfg, templates: loadTemplates(), loginLimiter: httpx.NewLoginLimiter()}
	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	return limitBody(mux)
}

// templateFuncs are helpers available to every page template.
var templateFuncs = template.FuncMap{
	// yesNo safely renders a *bool (e.g. ProductRelease/ComponentRelease's
	// PreRelease field). html/template's {{if}} treats any non-nil pointer
	// as true regardless of the pointee's value, so a bare {{if .PreRelease}}
	// would render "yes" even for a non-nil pointer to false -- go through
	// this instead of dereferencing directly in a template.
	"yesNo": func(b *bool) string {
		if b != nil && *b {
			return "yes"
		}
		return "no"
	},
}

// loadTemplates parses each page against the shared layout, in its own
// isolated template set -- keeping the "content" block name reusable
// across pages without collisions. login.html stands alone (no nav/layout,
// since there's no logged-in user to show it for).
func loadTemplates() map[string]*template.Template {
	out := map[string]*template.Template{
		"login": template.Must(template.New("login").Funcs(templateFuncs).ParseFS(templatesFS, "templates/login.html")),
	}
	for _, page := range []string{"dashboard", "users", "token", "products", "product", "productRelease", "components", "component", "componentRelease"} {
		out[page] = template.Must(template.New("layout").Funcs(templateFuncs).ParseFS(templatesFS, "templates/layout.html", "templates/"+page+".html"))
	}
	return out
}

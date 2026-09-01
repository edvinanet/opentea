// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import (
	"embed"
	"html/template"
	"net/http"
	"time"

	"github.com/oej/opentea/internal/httpx"
)

// Config holds the settings NewRouter needs -- RootURL for the
// same-origin CSRF check and the session cookie's Secure flag,
// TrustProxyHeaders for httpx.IsSecure/ClientIP's X-Forwarded-* trust
// model (see their own doc comments). cmd/openteapublisher's own process-
// level config (listen address, DB path, TLS) carries additional fields
// this package doesn't need.
type Config struct {
	RootURL           string
	TrustProxyHeaders bool
}

// maxRequestBody bounds every request body -- generous for the small HTML
// forms this app has (login, add/delete target), matching
// internal/webadmin's own maxWebadminBody convention.
var maxRequestBody int64 = 64 << 10 // 64 KiB

// bodyReadTimeout bounds how long reading a request body may take, so an
// unauthenticated client can't hold a connection open indefinitely via a
// slow-drip request body -- matches internal/webadmin's own
// webadminBodyReadTimeout convention/reasoning.
var bodyReadTimeout = 10 * time.Second

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
		_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(bodyReadTimeout))
		next.ServeHTTP(w, r)
	})
}

//go:embed templates/*.html
var templatesFS embed.FS

// Server holds the dependencies for opentea-publisher's HTTP handlers.
type Server struct {
	repo         *Repo
	cfg          Config
	templates    map[string]*template.Template
	loginLimiter *httpx.LoginLimiter
}

// NewRouter builds the full opentea-publisher mux.
func NewRouter(r *Repo, cfg Config) http.Handler {
	srv := &Server{repo: r, cfg: cfg, templates: loadTemplates(), loginLimiter: httpx.NewLoginLimiter()}
	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	// WithRequestID feeds RecordAudit's RequestID (targets.go) -- lets an
	// audit entry be correlated back to server logs for the same request.
	return httpx.WithRequestID(limitBody(mux))
}

// loadTemplates parses each page against the shared layout, in its own
// isolated template set -- keeping the "content" block name reusable
// across pages without collisions. login.html stands alone (no nav/layout,
// since there's no logged-in staff member to show it for) -- mirrors
// internal/webadmin/server.go's loadTemplates exactly.
func loadTemplates() map[string]*template.Template {
	out := map[string]*template.Template{
		"login": template.Must(template.New("login").ParseFS(templatesFS, "templates/login.html")),
	}
	for _, page := range []string{"dashboard", "staff"} {
		out[page] = template.Must(template.New("layout").ParseFS(templatesFS, "templates/layout.html", "templates/"+page+".html"))
	}
	return out
}

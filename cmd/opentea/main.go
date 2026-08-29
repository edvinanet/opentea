// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Command opentea runs the TEA consumer read API (spec-conformant, at
// /tea/v1 by default -- see TEA_API_BASE_PATH) plus a blob server
// (/files/{sha256}), and the operator-facing surface -- the unofficial admin
// ingestion API (/admin/v1) and the admin web GUI (/admin/ui) -- from a
// single process. Both surfaces share one listener (TEA_LISTEN_ADDR) unless
// TEA_ADMIN_LISTEN_ADDR is set, in which case the admin surface binds
// separately -- see config.Config.AdminListenAddr. Run
// `opentea createadmin -username=... -password=...` to bootstrap the first
// admin user before logging into the GUI.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/oej/opentea/internal/admin"
	"github.com/oej/opentea/internal/api"
	"github.com/oej/opentea/internal/config"
	"github.com/oej/opentea/internal/db"
	"github.com/oej/opentea/internal/files"
	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/internal/storage"
	"github.com/oej/opentea/internal/webadmin"
)

// shutdownGrace bounds how long a SIGINT/SIGTERM shutdown waits for
// in-flight requests (including large blob uploads/downloads, see the
// http.Server comment below) to finish before forcing connections closed.
const shutdownGrace = 30 * time.Second

func main() {
	startedAt := time.Now()

	if len(os.Args) > 1 && os.Args[1] == "createadmin" {
		runCreateAdmin(os.Args[2:])
		return
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		log.Fatalf("config error: TEA_TLS_CERT_FILE and TEA_TLS_KEY_FILE must both be set, or both left empty")
	}

	sqlDB, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	blobStore, err := storage.NewFSStorage(cfg.BlobDir)
	if err != nil {
		log.Fatalf("open blob storage: %v", err)
	}

	r := repo.New(sqlDB)
	tlsEnabled := cfg.TLSCertFile != ""

	var servers []*http.Server
	if cfg.AdminListenAddr == "" {
		servers = []*http.Server{buildServer(cfg.ListenAddr, newMux(r, blobStore, cfg, startedAt), tlsEnabled)}
		slog.Info("opentea server starting", "addr", cfg.ListenAddr, "rootURL", cfg.RootURL, "dbPath", cfg.DBPath, "blobDir", cfg.BlobDir, "tls", tlsEnabled)
	} else {
		servers = []*http.Server{
			buildServer(cfg.ListenAddr, newAPIMux(r, blobStore, cfg), tlsEnabled),
			buildServer(cfg.AdminListenAddr, newAdminMux(r, blobStore, cfg, startedAt), tlsEnabled),
		}
		slog.Info("opentea server starting", "apiAddr", cfg.ListenAddr, "adminAddr", cfg.AdminListenAddr, "rootURL", cfg.RootURL, "dbPath", cfg.DBPath, "blobDir", cfg.BlobDir, "tls", tlsEnabled)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, len(servers))
	for _, srv := range servers {
		go func(srv *http.Server) {
			if tlsEnabled {
				serveErr <- srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
			} else {
				serveErr <- srv.ListenAndServe()
			}
		}(srv)
	}

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server stopped: %v", err)
		}
	case <-ctx.Done():
		stop()
		slog.Info("shutdown signal received, waiting for in-flight requests", "grace", shutdownGrace)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		var wg sync.WaitGroup
		var mu sync.Mutex
		var shutdownErrs []error
		for _, srv := range servers {
			wg.Add(1)
			go func(srv *http.Server) {
				defer wg.Done()
				if err := srv.Shutdown(shutdownCtx); err != nil {
					mu.Lock()
					shutdownErrs = append(shutdownErrs, fmt.Errorf("%s: %w", srv.Addr, err))
					mu.Unlock()
				}
			}(srv)
		}
		wg.Wait()
		if len(shutdownErrs) > 0 {
			log.Fatalf("graceful shutdown failed: %v", errors.Join(shutdownErrs...))
		}
		slog.Info("server stopped cleanly")
	}
}

// buildServer applies the shared http.Server tuning (timeouts, TLS minimum
// version) to one listener. Factored out since a deployment with
// config.Config.AdminListenAddr set runs two of these concurrently -- see
// main -- and they must be tuned identically.
func buildServer(addr string, handler http.Handler, tlsEnabled bool) *http.Server {
	// ReadHeaderTimeout guards against slow-header (slowloris-style)
	// connections; ReadTimeout/WriteTimeout are deliberately left at the
	// zero value (no limit) since this server streams large blob
	// uploads/downloads (up to 1 GiB, see internal/admin/upload.go) that a
	// blanket whole-request timeout would risk truncating on a slow but
	// legitimate connection. IdleTimeout still bounds genuinely idle
	// keep-alive connections.
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if tlsEnabled {
		// Pinned explicitly rather than left at crypto/tls's own default so
		// this server's minimum doesn't silently change if that default
		// ever does; 1.2 is still the current floor most TEA client
		// tooling can be expected to support, with 1.3 negotiated
		// automatically whenever both ends support it.
		srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return srv
}

// newMux wires every sub-router onto a single mux -- config.Config.AdminListenAddr
// unset, today's default single-listener behavior. Factored out so the
// integration test can build the exact same handler tree against an
// httptest.Server without duplicating the wiring.
func newMux(r *repo.Repo, blobStore storage.Storage, cfg config.Config, startedAt time.Time) http.Handler {
	mux := http.NewServeMux()
	registerAPIRoutes(mux, r, blobStore, cfg)
	registerAdminRoutes(mux, r, blobStore, cfg, startedAt)
	return httpx.WithRequestID(securityHeaders(mux, cfg))
}

// newAPIMux wires only the consumer-facing surface (/tea/v1 + /files) --
// used for the ListenAddr listener when AdminListenAddr splits the admin
// surface onto its own listener.
func newAPIMux(r *repo.Repo, blobStore storage.Storage, cfg config.Config) http.Handler {
	mux := http.NewServeMux()
	registerAPIRoutes(mux, r, blobStore, cfg)
	return httpx.WithRequestID(securityHeaders(mux, cfg))
}

// newAdminMux wires only the operator-facing surface (/admin/v1 + /admin/ui)
// -- used for the AdminListenAddr listener when it's set.
func newAdminMux(r *repo.Repo, blobStore storage.Storage, cfg config.Config, startedAt time.Time) http.Handler {
	mux := http.NewServeMux()
	registerAdminRoutes(mux, r, blobStore, cfg, startedAt)
	return httpx.WithRequestID(securityHeaders(mux, cfg))
}

func registerAPIRoutes(mux *http.ServeMux, r *repo.Repo, blobStore storage.Storage, cfg config.Config) {
	mux.Handle(cfg.APIBasePath+"/", api.NewRouter(r, cfg))
	mux.Handle("/files/", files.NewHandler(r, blobStore))
}

func registerAdminRoutes(mux *http.ServeMux, r *repo.Repo, blobStore storage.Storage, cfg config.Config, startedAt time.Time) {
	mux.Handle("/admin/v1/", admin.NewRouter(r, blobStore, cfg, startedAt))
	mux.Handle("/admin/ui/", webadmin.NewRouter(r, cfg))
}

// adminCSP has no script-src at all -- internal/webadmin's templates never
// emit a <script> tag, so scripts are simply disallowed outright rather
// than allowlisted, which also covers any XSS that might otherwise sneak
// one in. style-src allows 'unsafe-inline' for the templates' own <style>
// blocks/attributes (all static, developer-authored, never built from
// request or DB data); nothing else is fetched from anywhere (no external
// fonts/images/CDN links), hence default-src 'none'.
const adminCSP = "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'"

// securityHeaders sets response headers that don't vary per request and
// aren't any individual handler's concern, applied uniformly across every
// route (api, admin, webadmin, files) rather than duplicated per package --
// they're harmless on the JSON/binary responses they don't functionally
// affect, and this is the one place the whole handler tree is assembled.
func securityHeaders(next http.Handler, cfg config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", adminCSP)
		if httpx.IsSecure(r, cfg.TrustProxyHeaders) {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

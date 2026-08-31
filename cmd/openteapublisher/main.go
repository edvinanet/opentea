// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

// Command openteapublisher runs opentea-publisher, the GUI publisher
// platform (design/publisher-service.md §4, §17): a separate service from
// opentea itself (own binary, own database, own deployment) that signs
// locally and calls a target TEA server's /publisher/v1 as a credentialed
// client. Run `openteapublisher createstaff -username=... -password=...`
// to bootstrap the first staff account before logging in.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oej/opentea/internal/openteapublisher"
)

// shutdownGrace bounds how long a SIGINT/SIGTERM shutdown waits for
// in-flight requests to finish before forcing connections closed --
// matches cmd/opentea's own convention.
const shutdownGrace = 30 * time.Second

func main() {
	if len(os.Args) > 1 && os.Args[1] == "createstaff" {
		runCreateStaff(os.Args[2:])
		return
	}

	cfg := loadConfig()
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		log.Fatalf("config error: OPENTEAPUBLISHER_TLS_CERT_FILE and OPENTEAPUBLISHER_TLS_KEY_FILE must both be set, or both left empty")
	}

	sqlDB, err := openteapublisher.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	r := openteapublisher.New(sqlDB)
	tlsEnabled := cfg.TLSCertFile != ""

	handler := openteapublisher.NewRouter(r, openteapublisher.Config{
		RootURL:           cfg.RootURL,
		TrustProxyHeaders: cfg.TrustProxyHeaders,
	})
	srv := buildServer(cfg.ListenAddr, handler, tlsEnabled)
	slog.Info("opentea-publisher starting", "addr", cfg.ListenAddr, "rootURL", cfg.RootURL, "dbPath", cfg.DBPath, "tls", tlsEnabled)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		if tlsEnabled {
			serveErr <- srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			serveErr <- srv.ListenAndServe()
		}
	}()

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
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Fatalf("graceful shutdown failed: %v", err)
		}
		slog.Info("server stopped cleanly")
	}
}

// buildServer applies the same timeout/TLS-minimum-version tuning
// cmd/opentea's own buildServer does.
func buildServer(addr string, handler http.Handler, tlsEnabled bool) *http.Server {
	srv := &http.Server{
		Addr: addr,
		// ReadHeaderTimeout guards against slow-header (slowloris-style)
		// connections; ReadTimeout/WriteTimeout are left at zero (no
		// overall body deadline) since openteapublisher's own limitBody
		// middleware already bounds request bodies and read time
		// per-request.
		ReadHeaderTimeout: 10 * time.Second,
		Handler:           handler,
	}
	if tlsEnabled {
		// Pinned explicitly rather than left at crypto/tls's own default,
		// matching cmd/opentea's own reasoning -- see its buildServer.
		srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return srv
}

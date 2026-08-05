// Command opentea runs the TEA consumer read API (spec-conformant, at
// /tea/v1), the unofficial admin ingestion API (/admin/v1), the admin web
// GUI (/admin/ui), and a blob server (/files/{sha256}) from a single
// process. Run `opentea createadmin -username=... -password=...` to
// bootstrap the first admin user before logging into the GUI.
package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/oej/opentea/internal/admin"
	"github.com/oej/opentea/internal/api"
	"github.com/oej/opentea/internal/config"
	"github.com/oej/opentea/internal/db"
	"github.com/oej/opentea/internal/files"
	"github.com/oej/opentea/internal/repo"
	"github.com/oej/opentea/internal/storage"
	"github.com/oej/opentea/internal/webadmin"
)

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
	mux := newMux(r, blobStore, cfg, startedAt)

	tlsEnabled := cfg.TLSCertFile != ""
	slog.Info("opentea server starting", "addr", cfg.ListenAddr, "rootURL", cfg.RootURL, "dbPath", cfg.DBPath, "blobDir", cfg.BlobDir, "tls", tlsEnabled)

	// ReadHeaderTimeout guards against slow-header (slowloris-style)
	// connections; ReadTimeout/WriteTimeout are deliberately left at the
	// zero value (no limit) since this server streams large blob
	// uploads/downloads (up to 1 GiB, see internal/admin/upload.go) that a
	// blanket whole-request timeout would risk truncating on a slow but
	// legitimate connection. IdleTimeout still bounds genuinely idle
	// keep-alive connections.
	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if tlsEnabled {
		err = srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
	} else {
		err = srv.ListenAndServe()
	}
	if err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

// newMux wires the sub-routers together. Factored out so the integration
// test can build the exact same handler tree against an httptest.Server
// without duplicating the wiring.
func newMux(r *repo.Repo, blobStore storage.Storage, cfg config.Config, startedAt time.Time) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/tea/v1/", api.NewRouter(r, cfg))
	mux.Handle("/admin/v1/", admin.NewRouter(r, blobStore, cfg, startedAt))
	mux.Handle("/admin/ui/", webadmin.NewRouter(r, cfg))
	mux.Handle("/files/", files.NewHandler(r, blobStore))
	return mux
}

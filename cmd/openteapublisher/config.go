// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import "os"

// config holds opentea-publisher's process-level configuration. Small and
// local -- not internal/config.Config, which carries opentea-specific
// fields (APIBasePath, Versions, TrustArchitectureEnabled) that don't
// apply here (design/publisher-service.md §17.4).
type config struct {
	ListenAddr string
	DBPath     string
	RootURL    string

	// TLSCertFile/TLSKeyFile: if both are set, the server listens with TLS
	// instead of plain HTTP. Leave both empty for plain HTTP (the default).
	TLSCertFile string
	TLSKeyFile  string

	// TrustProxyHeaders: see internal/config.Config's own field of the
	// same name -- identical trust model (openteapublisher.Config.TrustProxyHeaders,
	// httpx.IsSecure/ClientIP), only false (off) by default here too.
	TrustProxyHeaders bool
}

// loadConfig builds a config from environment variables, same "env var,
// else built-in default" precedence internal/config.Load uses (no config
// file support here -- not worth the extra layer for this app's small
// settings surface).
func loadConfig() config {
	return config{
		ListenAddr:        resolve("OPENTEAPUBLISHER_LISTEN_ADDR", ":8090"),
		DBPath:            resolve("OPENTEAPUBLISHER_DB_PATH", "data/openteapublisher.db"),
		RootURL:           resolve("OPENTEAPUBLISHER_ROOT_URL", "http://localhost:8090"),
		TLSCertFile:       resolve("OPENTEAPUBLISHER_TLS_CERT_FILE", ""),
		TLSKeyFile:        resolve("OPENTEAPUBLISHER_TLS_KEY_FILE", ""),
		TrustProxyHeaders: resolve("OPENTEAPUBLISHER_TRUST_PROXY_HEADERS", "false") == "true",
	}
}

func resolve(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

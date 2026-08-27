// Package config holds server configuration, layered as: built-in defaults
// < config file < environment variables (environment variables always win,
// matching systemd EnvironmentFile= conventions).
package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Config holds the server's runtime configuration, built by Load.
type Config struct {
	ListenAddr string
	DBPath     string
	BlobDir    string
	RootURL    string
	Versions   []string

	// APIBasePath is the URL path prefix this process's own mux serves the
	// consumer read API under (e.g. "/tea/v1", the default -- or "/v0.4.0"
	// for a standalone deployment that wants to answer literally at the
	// path TEA's discovery spec has clients construct: <endpoint's
	// url>/v<negotiated-version>/...). Deliberately independent of RootURL:
	// RootURL is what this deployment claims as its externally-reachable
	// origin (e.g. in discovery responses), which may sit behind a reverse
	// proxy that terminates a completely different external path (say,
	// "/v1/") and rewrites it to whatever APIBasePath this process actually
	// listens on internally -- the two are set separately on purpose, don't
	// assume one can be derived from the other. Covers a single path only:
	// a deployment that needs to serve more than one TEA_VERSIONS entry at
	// its own literal /v{version}/ path simultaneously (rather than behind
	// a version-aware proxy) isn't supported by one opentea process today.
	APIBasePath string

	// OrgName identifies the organisation running this server. Optional;
	// shown in the admin GUI and in GET /admin/v1/stats when set.
	OrgName string

	// TLSCertFile/TLSKeyFile: if both are set, the server listens with TLS
	// (http.ListenAndServeTLS) instead of plain HTTP. Leave both empty for
	// plain HTTP (the default).
	TLSCertFile string
	TLSKeyFile  string

	// TrustProxyHeaders, if set, makes the server treat incoming
	// X-Forwarded-* headers from a reverse proxy as trustworthy: an
	// X-Forwarded-Proto: https header is taken as proof the connection is
	// effectively over TLS (used for the session cookie's Secure flag and
	// HSTS) even though the connection this process itself sees is plain
	// HTTP, and the right-most entry of X-Forwarded-For is used as the real
	// client IP for admin-login rate limiting (internal/webadmin/loginlimiter.go's
	// clientIP) instead of RemoteAddr (which would otherwise be the proxy's
	// own address for every client). Both are the normal shape of a
	// TLS-terminating reverse proxy deployment. Off by default: these
	// headers are exactly as trustworthy as any other client-supplied
	// header unless something in front of this process is actually
	// guaranteed to set (and strip any client-supplied copy of) them, which
	// is a deployment fact this process can't verify on its own -- only
	// turn this on if that's actually true of your deployment.
	TrustProxyHeaders bool

	// TrustArchitectureEnabled declares whether this deployment operates
	// under oej's TEA Trust Architecture overlay ("Trusted TEA") rather
	// than plain TEA. Display-only in this phase -- shown in the admin GUI
	// as the deployment's declared profile, but does not enforce
	// evidence-bundle requirements on any /admin/v1 write path (see
	// internal/trust's phased plan; enforcement is a later phase). Off
	// (plain TEA) by default.
	TrustArchitectureEnabled bool
}

// defaultConfigFile is checked automatically if TEA_CONFIG_FILE isn't set.
// It's fine for this file not to exist -- it's entirely optional.
const defaultConfigFile = "/etc/opentea/opentea.conf"

// Load builds the Config, returning an error if an explicitly-requested
// config file (via TEA_CONFIG_FILE) is missing or malformed. A missing
// *default* config file is not an error -- it's optional.
func Load() (Config, error) {
	path, required := configFilePath()
	fileValues, err := loadConfigFile(path, required)
	if err != nil {
		return Config{}, err
	}

	basePath, err := normalizeBasePath(resolve("TEA_API_BASE_PATH", fileValues, "/tea/v1"))
	if err != nil {
		return Config{}, err
	}

	return Config{
		ListenAddr:               resolve("TEA_LISTEN_ADDR", fileValues, ":8080"),
		DBPath:                   resolve("TEA_DB_PATH", fileValues, "data/opentea.db"),
		BlobDir:                  resolve("TEA_BLOB_DIR", fileValues, "data/blobs"),
		RootURL:                  resolve("TEA_ROOT_URL", fileValues, "http://localhost:8080"),
		Versions:                 splitCSV(resolve("TEA_VERSIONS", fileValues, "0.4.0")),
		APIBasePath:              basePath,
		OrgName:                  resolve("TEA_ORG_NAME", fileValues, ""),
		TLSCertFile:              resolve("TEA_TLS_CERT_FILE", fileValues, ""),
		TLSKeyFile:               resolve("TEA_TLS_KEY_FILE", fileValues, ""),
		TrustProxyHeaders:        resolve("TEA_TRUST_PROXY_HEADERS", fileValues, "false") == "true",
		TrustArchitectureEnabled: resolve("TEA_TRUST_ARCHITECTURE", fileValues, "false") == "true",
	}, nil
}

// configFilePath returns the config file to load: an explicit
// TEA_CONFIG_FILE if set (required=true, since the operator pointed at it on
// purpose -- a missing/bad file is then an error), otherwise the
// conventional default path (required=false -- silently skipped if absent).
func configFilePath() (path string, required bool) {
	if p, ok := os.LookupEnv("TEA_CONFIG_FILE"); ok && p != "" {
		return p, true
	}
	return defaultConfigFile, false
}

// resolve applies the precedence order: env var > config file > fallback.
func resolve(key string, fileValues map[string]string, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	if v, ok := fileValues[key]; ok && v != "" {
		return v
	}
	return fallback
}

// normalizeBasePath enforces the one shape every consumer of Config.APIBasePath
// relies on: a leading slash, no trailing slash (so simple concatenation --
// APIBasePath+"/product/{uuid}" -- always produces a clean path), and non-empty
// (serving the whole API at "/" would collide with the admin/files routes
// mounted alongside it in cmd/opentea/main.go).
func normalizeBasePath(raw string) (string, error) {
	p := strings.TrimSuffix(raw, "/")
	if p == "" {
		return "", fmt.Errorf("TEA_API_BASE_PATH must not be empty or \"/\"")
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p, nil
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// loadConfigFile parses a simple "KEY=VALUE" file, one entry per line:
// blank lines and lines starting with "#" are ignored, everything else is
// split on the first "=" with surrounding whitespace trimmed. There's no
// quoting -- values are taken verbatim after trimming.
func loadConfigFile(path string, required bool) (map[string]string, error) {
	f, err := os.Open(path) //nolint:gosec // path is either the fixed default config path or the operator's own TEA_CONFIG_FILE env var, not remote input
	if err != nil {
		if os.IsNotExist(err) && !required {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("open config file %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	values := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("config file %q: invalid line (expected KEY=VALUE): %q", path, line)
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}
	return values, nil
}

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

type Config struct {
	ListenAddr string
	DBPath     string
	BlobDir    string
	RootURL    string
	Versions   []string

	// OrgName identifies the organisation running this server. Optional;
	// shown in the admin GUI and in GET /admin/v1/stats when set.
	OrgName string

	// TLSCertFile/TLSKeyFile: if both are set, the server listens with TLS
	// (http.ListenAndServeTLS) instead of plain HTTP. Leave both empty for
	// plain HTTP (the default).
	TLSCertFile string
	TLSKeyFile  string
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

	return Config{
		ListenAddr:  resolve("TEA_LISTEN_ADDR", fileValues, ":8080"),
		DBPath:      resolve("TEA_DB_PATH", fileValues, "data/opentea.db"),
		BlobDir:     resolve("TEA_BLOB_DIR", fileValues, "data/blobs"),
		RootURL:     resolve("TEA_ROOT_URL", fileValues, "http://localhost:8080"),
		Versions:    splitCSV(resolve("TEA_VERSIONS", fileValues, "0.4.0")),
		OrgName:     resolve("TEA_ORG_NAME", fileValues, ""),
		TLSCertFile: resolve("TEA_TLS_CERT_FILE", fileValues, ""),
		TLSKeyFile:  resolve("TEA_TLS_KEY_FILE", fileValues, ""),
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
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) && !required {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("open config file %q: %w", path, err)
	}
	defer f.Close()

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

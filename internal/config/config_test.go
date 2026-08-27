package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// No TEA_CONFIG_FILE set -> falls back to the (almost certainly absent,
	// in a test environment) default path, which is fine and not an error.
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want :8080", cfg.ListenAddr)
	}
	if cfg.DBPath != "data/opentea.db" {
		t.Errorf("DBPath = %q", cfg.DBPath)
	}
	if cfg.OrgName != "" {
		t.Errorf("OrgName = %q, want empty", cfg.OrgName)
	}
	if cfg.TLSCertFile != "" || cfg.TLSKeyFile != "" {
		t.Errorf("TLS fields = %q/%q, want both empty", cfg.TLSCertFile, cfg.TLSKeyFile)
	}
	if cfg.APIBasePath != "/tea/v1" {
		t.Errorf("APIBasePath = %q, want /tea/v1", cfg.APIBasePath)
	}
}

func TestAPIBasePathNormalization(t *testing.T) {
	cases := []struct {
		env, want string
	}{
		{"/v0.4.0", "/v0.4.0"},
		{"/v0.4.0/", "/v0.4.0"}, // trailing slash stripped
		{"v0.4.0", "/v0.4.0"},   // leading slash added
		{"/tea/v1/", "/tea/v1"},
	}
	for _, tc := range cases {
		t.Run(tc.env, func(t *testing.T) {
			t.Setenv("TEA_API_BASE_PATH", tc.env)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.APIBasePath != tc.want {
				t.Errorf("APIBasePath = %q, want %q", cfg.APIBasePath, tc.want)
			}
		})
	}
}

func TestAPIBasePathEmptyIsError(t *testing.T) {
	t.Setenv("TEA_API_BASE_PATH", "/")
	if _, err := Load(); err == nil {
		t.Fatal("expected an error for TEA_API_BASE_PATH=/")
	}
}

func TestLoadFromConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opentea.conf")
	content := "# a comment\n\nTEA_LISTEN_ADDR=:9090\nTEA_ORG_NAME=Acme Corp\nTEA_DB_PATH = /custom/path.db \n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("TEA_CONFIG_FILE", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ListenAddr != ":9090" {
		t.Errorf("ListenAddr = %q, want :9090", cfg.ListenAddr)
	}
	if cfg.OrgName != "Acme Corp" {
		t.Errorf("OrgName = %q, want %q", cfg.OrgName, "Acme Corp")
	}
	if cfg.DBPath != "/custom/path.db" {
		t.Errorf("DBPath = %q, want trimmed /custom/path.db", cfg.DBPath)
	}
	// Untouched by the file -- still the built-in default.
	if cfg.BlobDir != "data/blobs" {
		t.Errorf("BlobDir = %q, want default", cfg.BlobDir)
	}
}

func TestEnvVarOverridesConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opentea.conf")
	if err := os.WriteFile(path, []byte("TEA_ORG_NAME=From File\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("TEA_CONFIG_FILE", path)
	t.Setenv("TEA_ORG_NAME", "From Env")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.OrgName != "From Env" {
		t.Fatalf("OrgName = %q, want env var to win (%q)", cfg.OrgName, "From Env")
	}
}

func TestLoadExplicitConfigFileMissingIsError(t *testing.T) {
	t.Setenv("TEA_CONFIG_FILE", filepath.Join(t.TempDir(), "does-not-exist.conf"))
	if _, err := Load(); err == nil {
		t.Fatal("expected an error for an explicitly-requested missing config file")
	}
}

func TestLoadMalformedConfigFileIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opentea.conf")
	if err := os.WriteFile(path, []byte("this line has no equals sign\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("TEA_CONFIG_FILE", path)

	if _, err := Load(); err == nil {
		t.Fatal("expected an error for a malformed config file line")
	}
}

func TestTLSFieldsFromConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opentea.conf")
	content := "TEA_TLS_CERT_FILE=/etc/opentea/tls/cert.pem\nTEA_TLS_KEY_FILE=/etc/opentea/tls/key.pem\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("TEA_CONFIG_FILE", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TLSCertFile != "/etc/opentea/tls/cert.pem" || cfg.TLSKeyFile != "/etc/opentea/tls/key.pem" {
		t.Fatalf("TLS fields = %q/%q", cfg.TLSCertFile, cfg.TLSKeyFile)
	}
}

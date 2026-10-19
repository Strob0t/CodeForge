package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mustLoadEnv is loadEnv for tests whose environment is valid.
func mustLoadEnv(t *testing.T, cfg *Config) {
	t.Helper()
	if err := loadEnv(cfg); err != nil {
		t.Fatalf("loadEnv: %v", err)
	}
}

// A config file named with -config or CODEFORGE_CONFIG_FILE must exist: a
// typo would otherwise start the Core silently on defaults (KI-213). The
// default codeforge.yaml stays optional (zero-config startup).
func TestLoadWithCLI_ExplicitConfigFileMustExist(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	tests := []struct {
		name  string
		env   string
		flags []string
	}{
		{"env", missing, nil},
		{"flag", "", []string{"-config", missing}},
		{"flag over a valid env file", writeTestSecret(t, "server: {port: \"7000\"}\n"), []string{"-config", missing}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APP_ENV", "development")
			t.Setenv("CODEFORGE_CONFIG_FILE", tt.env)
			flags, err := ParseFlags(tt.flags)
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = LoadWithCLI(flags)
			if err == nil {
				t.Fatal("LoadWithCLI accepted a missing explicit config file")
			}
			if !strings.Contains(err.Error(), missing) {
				t.Errorf("error %q does not name the file", err)
			}
		})
	}
}

func TestLoadWithCLI_DefaultConfigFileIsOptional(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("APP_ENV", "development")
	t.Setenv("CODEFORGE_CONFIG_FILE", "")
	if _, err := os.Stat(DefaultConfigFile); err == nil {
		t.Fatal("test directory unexpectedly has a codeforge.yaml")
	}
	flags, err := ParseFlags(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadWithCLI(flags); err != nil {
		t.Fatalf("LoadWithCLI without codeforge.yaml: %v", err)
	}
}

// A typed environment value that does not parse is an error naming the key,
// not a warning that leaves the default in place (KI-213).
func TestLoadEnv_InvalidTypedValueFails(t *testing.T) {
	tests := []struct {
		key   string
		value string
	}{
		{"CODEFORGE_QUARANTINE_ENABLED", "yes"},
		{"CODEFORGE_PG_MAX_CONNS", "fifty"},
		{"CODEFORGE_PG_MAX_CONNS", "99999999999"},
		{"CODEFORGE_BREAKER_TIMEOUT", "30"},
		{"CODEFORGE_RATE_RPS", "1,5"},
		{"CODEFORGE_AUTH_ENABLED", "off"},
	}
	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			t.Setenv(tt.key, tt.value)
			cfg := Defaults()
			err := loadEnv(&cfg)
			if err == nil {
				t.Fatalf("loadEnv accepted %s=%q", tt.key, tt.value)
			}
			if !strings.Contains(err.Error(), tt.key) {
				t.Errorf("error %q does not name %s", err, tt.key)
			}

			t.Setenv("APP_ENV", "development")
			if _, err := loadFromNoYAML(t); err == nil {
				t.Errorf("LoadFrom accepted %s=%q", tt.key, tt.value)
			}
		})
	}
}

func TestLoadEnv_ValidTypedValues(t *testing.T) {
	t.Setenv("CODEFORGE_QUARANTINE_ENABLED", "true")
	t.Setenv("CODEFORGE_PG_MAX_CONNS", "25")
	t.Setenv("CODEFORGE_BREAKER_TIMEOUT", "45s")
	cfg := Defaults()
	if err := loadEnv(&cfg); err != nil {
		t.Fatalf("loadEnv: %v", err)
	}
	if !cfg.Quarantine.Enabled || cfg.Postgres.MaxConns != 25 || cfg.Breaker.Timeout.String() != "45s" {
		t.Errorf("values not applied: quarantine=%v max_conns=%d breaker=%s",
			cfg.Quarantine.Enabled, cfg.Postgres.MaxConns, cfg.Breaker.Timeout)
	}
}

// auth.enabled=false makes every request a platform admin: production
// refuses it (KI-213).
func TestValidate_AuthDisabledInProduction(t *testing.T) {
	tests := []struct {
		appEnv  string
		wantErr bool
	}{
		{"production", true},
		{"development", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run("APP_ENV="+tt.appEnv, func(t *testing.T) {
			cfg := Defaults()
			cfg.AppEnv = tt.appEnv
			cfg.Auth.Enabled = false
			cfg.Postgres.DSN = "postgres://u:p@db:5432/codeforge?sslmode=require"
			err := validate(&cfg)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "auth.enabled") {
					t.Fatalf("validate = %v, want an auth.enabled error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("validate: %v", err)
			}
		})
	}
}

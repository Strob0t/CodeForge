package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Integration tests that exercise the full LoadFrom pipeline:
// defaults < YAML < environment variables.

func TestLoadFrom_FullHierarchy(t *testing.T) {
	// YAML sets port=9090, env overrides to 7070. Env must win.
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(yamlPath, []byte(`
server:
  port: "9090"
logging:
  level: "debug"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("APP_ENV", "development") // required for default JWT secret
	t.Setenv("CODEFORGE_PORT", "7070")
	t.Setenv("CODEFORGE_LOG_LEVEL", "warn")

	cfg, err := LoadFrom(yamlPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if cfg.Server.Port != "7070" {
		t.Errorf("env should override YAML: got port %q, want 7070", cfg.Server.Port)
	}
	if cfg.Logging.Level != "warn" {
		t.Errorf("env should override YAML: got level %q, want warn", cfg.Logging.Level)
	}
}

func TestLoadFrom_YAMLPartialOverride(t *testing.T) {
	// YAML sets only logging.level; all other fields keep defaults.
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(yamlPath, []byte(`
logging:
  level: "error"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("APP_ENV", "development") // required for default JWT secret

	cfg, err := LoadFrom(yamlPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if cfg.Logging.Level != "error" {
		t.Errorf("got level %q, want error", cfg.Logging.Level)
	}
	// Defaults preserved
	if cfg.Server.Port != "8080" {
		t.Errorf("default port should be 8080, got %q", cfg.Server.Port)
	}
	if cfg.Postgres.MaxConns != 50 {
		t.Errorf("default max_conns should be 50, got %d", cfg.Postgres.MaxConns)
	}
	// Note: NATS.URL may be overridden by NATS_URL env var in devcontainers,
	// so we only check that it's non-empty (validation would catch empty).
	if cfg.NATS.URL == "" {
		t.Error("NATS URL should not be empty")
	}
}

func TestLoadFrom_EnvInvalidValues(t *testing.T) {
	// Invalid env values fail the load and every one is named (KI-213); they
	// used to be ignored with a warning, leaving the defaults in place.
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(yamlPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("APP_ENV", "development") // required for default JWT secret
	t.Setenv("CODEFORGE_PG_MAX_CONNS", "notanumber")
	t.Setenv("CODEFORGE_BREAKER_TIMEOUT", "invalid-duration")
	t.Setenv("CODEFORGE_RATE_RPS", "abc")

	_, err := LoadFrom(yamlPath)
	if err == nil {
		t.Fatal("LoadFrom accepted invalid env values")
	}
	for _, key := range []string{"CODEFORGE_PG_MAX_CONNS", "CODEFORGE_BREAKER_TIMEOUT", "CODEFORGE_RATE_RPS"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not name %s", err, key)
		}
	}
}

func TestLoadFrom_MissingYAMLFile(t *testing.T) {
	// Non-existent YAML => pure defaults, no error.
	t.Setenv("APP_ENV", "development") // required for default JWT secret

	cfg, err := LoadFrom("/nonexistent/path/to/config.yaml")
	if err != nil {
		t.Fatalf("missing YAML should not error, got %v", err)
	}

	if cfg.Server.Port != "8080" {
		t.Errorf("expected default port 8080, got %q", cfg.Server.Port)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("expected default log level info, got %q", cfg.Logging.Level)
	}
}

func TestLoadFrom_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(yamlPath, []byte(`{{{invalid yaml`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFrom(yamlPath)
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
}

func TestLoadFrom_ValidationAfterOverride(t *testing.T) {
	// YAML sets port to empty string => validation error.
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(yamlPath, []byte(`
server:
  port: ""
`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFrom(yamlPath)
	if err == nil {
		t.Fatal("expected validation error for empty port, got nil")
	}
}

func TestLoadFrom_OrchestratorOverrides(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(yamlPath, []byte(`
orchestrator:
  max_parallel: 8
  mode: "full_auto"
  decompose_model: "anthropic/claude-3-haiku"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("APP_ENV", "development") // required for default JWT secret

	cfg, err := LoadFrom(yamlPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if cfg.Orchestrator.MaxParallel != 8 {
		t.Errorf("got max_parallel %d, want 8", cfg.Orchestrator.MaxParallel)
	}
	if cfg.Orchestrator.Mode != "full_auto" {
		t.Errorf("got mode %q, want full_auto", cfg.Orchestrator.Mode)
	}
	if cfg.Orchestrator.DecomposeModel != "anthropic/claude-3-haiku" {
		t.Errorf("got decompose_model %q, want anthropic/claude-3-haiku", cfg.Orchestrator.DecomposeModel)
	}
	// Unchanged orchestrator defaults
	if cfg.Orchestrator.PingPongMaxRounds != 3 {
		t.Errorf("default ping_pong_max_rounds should be 3, got %d", cfg.Orchestrator.PingPongMaxRounds)
	}
}

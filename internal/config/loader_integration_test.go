package config

import (
	"os"
	"path/filepath"
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
	if cfg.Postgres.MaxConns != 15 {
		t.Errorf("default max_conns should be 15, got %d", cfg.Postgres.MaxConns)
	}
	// Note: NATS.URL may be overridden by NATS_URL env var in devcontainers,
	// so we only check that it's non-empty (validation would catch empty).
	if cfg.NATS.URL == "" {
		t.Error("NATS URL should not be empty")
	}
}

func TestLoadFrom_EnvInvalidValues(t *testing.T) {
	// Invalid env values are silently ignored; defaults survive.
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(yamlPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CODEFORGE_PG_MAX_CONNS", "notanumber")
	t.Setenv("CODEFORGE_BREAKER_TIMEOUT", "invalid-duration")
	t.Setenv("CODEFORGE_RATE_RPS", "abc")

	cfg, err := LoadFrom(yamlPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if cfg.Postgres.MaxConns != 15 {
		t.Errorf("invalid int env should be ignored: got max_conns %d, want 15", cfg.Postgres.MaxConns)
	}
	if cfg.Breaker.Timeout.String() != "30s" {
		t.Errorf("invalid duration env should be ignored: got %v, want 30s", cfg.Breaker.Timeout)
	}
	if cfg.Rate.RequestsPerSecond != 10 {
		t.Errorf("invalid float env should be ignored: got %v, want 10", cfg.Rate.RequestsPerSecond)
	}
}

func TestLoadFrom_MissingYAMLFile(t *testing.T) {
	// Non-existent YAML => pure defaults, no error.
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

func TestReload_UpdatesFields(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "cfg.yaml")

	// Initial config
	if err := os.WriteFile(yamlPath, []byte(`
logging:
  level: "info"
rate:
  burst: 50
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(yamlPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	holder := NewHolder(cfg, yamlPath)

	// Verify initial
	got := holder.Get()
	if got.Logging.Level != "info" {
		t.Fatalf("initial level should be info, got %q", got.Logging.Level)
	}

	// Update YAML
	if err := os.WriteFile(yamlPath, []byte(`
logging:
  level: "debug"
rate:
  burst: 200
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Reload
	if err := holder.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	got = holder.Get()
	if got.Logging.Level != "debug" {
		t.Errorf("after reload: got level %q, want debug", got.Logging.Level)
	}
	if got.Rate.Burst != 200 {
		t.Errorf("after reload: got burst %d, want 200", got.Rate.Burst)
	}
}

func TestReload_ValidationFails_PreservesOld(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "cfg.yaml")

	// Valid initial config
	if err := os.WriteFile(yamlPath, []byte(`
server:
  port: "9090"
logging:
  level: "info"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(yamlPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	holder := NewHolder(cfg, yamlPath)

	// Write invalid config (empty port)
	if err := os.WriteFile(yamlPath, []byte(`
server:
  port: ""
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Reload should fail
	if err := holder.Reload(); err == nil {
		t.Fatal("expected reload to fail for invalid config")
	}

	// Old config preserved
	got := holder.Get()
	if got.Server.Port != "9090" {
		t.Errorf("old config should be preserved: got port %q, want 9090", got.Server.Port)
	}
	if got.Logging.Level != "info" {
		t.Errorf("old config should be preserved: got level %q, want info", got.Logging.Level)
	}
}

func TestReload_EnvOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "cfg.yaml")

	if err := os.WriteFile(yamlPath, []byte(`
logging:
  level: "info"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(yamlPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	holder := NewHolder(cfg, yamlPath)

	// Set env before reload
	t.Setenv("CODEFORGE_LOG_LEVEL", "error")

	if err := holder.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	got := holder.Get()
	if got.Logging.Level != "error" {
		t.Errorf("env should override YAML on reload: got %q, want error", got.Logging.Level)
	}
}

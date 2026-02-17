package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.Server.Port != "8080" {
		t.Errorf("expected port 8080, got %s", cfg.Server.Port)
	}
	if cfg.Postgres.MaxConns != 15 {
		t.Errorf("expected max_conns 15, got %d", cfg.Postgres.MaxConns)
	}
	if cfg.Breaker.Timeout != 30*time.Second {
		t.Errorf("expected breaker timeout 30s, got %v", cfg.Breaker.Timeout)
	}
}

func TestLoadYAMLOverride(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "test.yaml")

	content := `
server:
  port: "9090"
  cors_origin: "http://example.com"
postgres:
  max_conns: 20
logging:
  level: "debug"
`
	if err := os.WriteFile(yamlPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Defaults()
	if err := loadYAML(&cfg, yamlPath); err != nil {
		t.Fatal(err)
	}

	if cfg.Server.Port != "9090" {
		t.Errorf("expected port 9090, got %s", cfg.Server.Port)
	}
	if cfg.Server.CORSOrigin != "http://example.com" {
		t.Errorf("expected cors http://example.com, got %s", cfg.Server.CORSOrigin)
	}
	if cfg.Postgres.MaxConns != 20 {
		t.Errorf("expected max_conns 20, got %d", cfg.Postgres.MaxConns)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected log level debug, got %s", cfg.Logging.Level)
	}
	// Unchanged fields keep defaults
	if cfg.NATS.URL != "nats://localhost:4222" {
		t.Errorf("expected default NATS URL, got %s", cfg.NATS.URL)
	}
}

func TestLoadYAMLMissing(t *testing.T) {
	cfg := Defaults()
	err := loadYAML(&cfg, "/nonexistent/path.yaml")
	if err != nil {
		t.Errorf("missing YAML should not error, got %v", err)
	}
}

func TestEnvOverride(t *testing.T) {
	cfg := Defaults()

	t.Setenv("CODEFORGE_PORT", "7070")
	t.Setenv("DATABASE_URL", "postgres://test:test@db:5432/test")
	t.Setenv("CODEFORGE_PG_MAX_CONNS", "25")
	t.Setenv("CODEFORGE_LOG_LEVEL", "warn")
	t.Setenv("CODEFORGE_BREAKER_TIMEOUT", "1m")

	loadEnv(&cfg)

	if cfg.Server.Port != "7070" {
		t.Errorf("expected port 7070, got %s", cfg.Server.Port)
	}
	if cfg.Postgres.DSN != "postgres://test:test@db:5432/test" {
		t.Errorf("expected test DSN, got %s", cfg.Postgres.DSN)
	}
	if cfg.Postgres.MaxConns != 25 {
		t.Errorf("expected max_conns 25, got %d", cfg.Postgres.MaxConns)
	}
	if cfg.Logging.Level != "warn" {
		t.Errorf("expected log level warn, got %s", cfg.Logging.Level)
	}
	if cfg.Breaker.Timeout != time.Minute {
		t.Errorf("expected breaker timeout 1m, got %v", cfg.Breaker.Timeout)
	}
}

func TestValidateRequired(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*Config)
		errMsg string
	}{
		{
			name:   "empty port",
			modify: func(c *Config) { c.Server.Port = "" },
			errMsg: "server.port is required",
		},
		{
			name:   "empty DSN",
			modify: func(c *Config) { c.Postgres.DSN = "" },
			errMsg: "postgres.dsn is required",
		},
		{
			name:   "empty NATS URL",
			modify: func(c *Config) { c.NATS.URL = "" },
			errMsg: "nats.url is required",
		},
		{
			name:   "zero max_conns",
			modify: func(c *Config) { c.Postgres.MaxConns = 0 },
			errMsg: "postgres.max_conns must be >= 1",
		},
		{
			name:   "zero breaker failures",
			modify: func(c *Config) { c.Breaker.MaxFailures = 0 },
			errMsg: "breaker.max_failures must be >= 1",
		},
		{
			name:   "zero rate burst",
			modify: func(c *Config) { c.Rate.Burst = 0 },
			errMsg: "rate.burst must be >= 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults()
			tt.modify(&cfg)
			err := validate(&cfg)
			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.errMsg)
			}
			if err.Error() != tt.errMsg {
				t.Errorf("expected %q, got %q", tt.errMsg, err.Error())
			}
		})
	}
}

func TestValidateDefaults(t *testing.T) {
	cfg := Defaults()
	if err := validate(&cfg); err != nil {
		t.Errorf("defaults should validate, got %v", err)
	}
}

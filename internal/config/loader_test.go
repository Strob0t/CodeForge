package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.Server.Port != "8080" {
		t.Errorf("expected port 8080, got %s", cfg.Server.Port)
	}
	if cfg.Postgres.MaxConns != 50 {
		t.Errorf("expected max_conns 50, got %d", cfg.Postgres.MaxConns)
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
	cfg.AppEnv = "development"
	// Default JWT secret is now empty; ensureSecrets auto-generates one.
	if err := ensureSecrets(&cfg); err != nil {
		t.Fatalf("ensureSecrets failed: %v", err)
	}
	if err := validate(&cfg); err != nil {
		t.Errorf("defaults should validate in development, got %v", err)
	}
}

func TestValidate_EmptyJWTSecretRejected(t *testing.T) {
	tests := []struct {
		name   string
		appEnv string
	}{
		{"empty env", ""},
		{"staging", "staging"},
		{"production", "production"},
		{"development", "development"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.AppEnv = tt.appEnv
			// Skip ensureSecrets to test that validate rejects empty secret.
			err := validate(&cfg)
			if err == nil {
				t.Fatal("expected error for empty JWT secret")
			}
			if !strings.Contains(err.Error(), "jwt_secret is required") {
				t.Errorf("expected jwt_secret required error, got: %v", err)
			}
		})
	}
}

func TestValidate_LowEntropyJWTSecretRejectedInProd(t *testing.T) {
	cfg := Defaults()
	cfg.AppEnv = "production"
	cfg.Auth.JWTSecret = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 34 chars, but very low entropy
	err := validate(&cfg)
	if err == nil {
		t.Fatal("expected error for low-entropy JWT secret in production")
	}
	if !strings.Contains(err.Error(), "insufficient entropy") {
		t.Errorf("expected entropy error, got: %v", err)
	}
}

func TestValidate_JWTSecretMinLength(t *testing.T) {
	cfg := Defaults()
	cfg.AppEnv = "production"
	cfg.Auth.JWTSecret = "too-short" // 9 chars, below 32
	err := validate(&cfg)
	if err == nil {
		t.Fatal("expected error for short JWT secret")
	}
	if !strings.Contains(err.Error(), "at least 32 characters") {
		t.Errorf("expected length error, got: %v", err)
	}
}

func TestValidate_AdminPasswordRejectedInNonDev(t *testing.T) {
	passwords := []string{"changeme123", "Changeme123", "admin", "password", "CHANGE_ME_ON_FIRST_BOOT"}
	for _, pw := range passwords {
		t.Run(pw, func(t *testing.T) {
			cfg := Defaults()
			cfg.AppEnv = "staging"
			cfg.Auth.JWTSecret = "this-is-a-long-enough-secret-for-production-use-ok"
			cfg.Auth.DefaultAdminPass = pw
			err := validate(&cfg)
			if err == nil {
				t.Fatalf("expected error for default admin password %q in non-dev", pw)
			}
			if !strings.Contains(err.Error(), "well-known default") {
				t.Errorf("expected admin password error, got: %v", err)
			}
		})
	}
}

func TestValidate_AdminPasswordAllowedInDev(t *testing.T) {
	cfg := Defaults()
	cfg.AppEnv = "development"
	cfg.Auth.DefaultAdminPass = "changeme123"
	// Provide a valid JWT secret so we can test the admin password path.
	if err := ensureSecrets(&cfg); err != nil {
		t.Fatalf("ensureSecrets failed: %v", err)
	}
	err := validate(&cfg)
	if err != nil {
		t.Errorf("default admin password should be allowed in development, got: %v", err)
	}
}

func TestValidate_PostgresSSLDisableRejectedInProduction(t *testing.T) {
	cfg := Defaults()
	cfg.AppEnv = "production"
	cfg.Auth.JWTSecret = "this-is-a-long-enough-secret-for-production-use-ok"
	cfg.Postgres.DSN = "postgres://user:pass@host:5432/db?sslmode=disable"
	err := validate(&cfg)
	if err == nil {
		t.Fatal("expected error for sslmode=disable in production")
	}
	if !strings.Contains(err.Error(), "sslmode=disable") {
		t.Errorf("expected sslmode error, got: %v", err)
	}
}

func TestValidate_PostgresSSLPreferAllowedInProduction(t *testing.T) {
	cfg := Defaults()
	cfg.AppEnv = "production"
	cfg.Auth.JWTSecret = "this-is-a-long-enough-secret-for-production-use-ok"
	// Default DSN already uses sslmode=prefer
	err := validate(&cfg)
	if err != nil {
		t.Errorf("sslmode=prefer should be allowed in production, got: %v", err)
	}
}

func TestPolicyDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.Policy.DefaultProfile != "headless-safe-sandbox" {
		t.Errorf("expected default profile 'headless-safe-sandbox', got %q", cfg.Policy.DefaultProfile)
	}
	if cfg.Policy.CustomDir != "" {
		t.Errorf("expected empty custom dir, got %q", cfg.Policy.CustomDir)
	}
}

func TestPolicyYAMLOverride(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "test.yaml")
	content := `
policy:
  default_profile: "plan-readonly"
  custom_dir: "/etc/codeforge/policies"
`
	if err := os.WriteFile(yamlPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Defaults()
	if err := loadYAML(&cfg, yamlPath); err != nil {
		t.Fatal(err)
	}

	if cfg.Policy.DefaultProfile != "plan-readonly" {
		t.Errorf("expected 'plan-readonly', got %q", cfg.Policy.DefaultProfile)
	}
	if cfg.Policy.CustomDir != "/etc/codeforge/policies" {
		t.Errorf("expected '/etc/codeforge/policies', got %q", cfg.Policy.CustomDir)
	}
}

func TestPolicyEnvOverride(t *testing.T) {
	cfg := Defaults()

	t.Setenv("CODEFORGE_POLICY_DEFAULT", "trusted-mount-autonomous")
	t.Setenv("CODEFORGE_POLICY_DIR", "/custom/policies")

	loadEnv(&cfg)

	if cfg.Policy.DefaultProfile != "trusted-mount-autonomous" {
		t.Errorf("expected 'trusted-mount-autonomous', got %q", cfg.Policy.DefaultProfile)
	}
	if cfg.Policy.CustomDir != "/custom/policies" {
		t.Errorf("expected '/custom/policies', got %q", cfg.Policy.CustomDir)
	}
}

func TestParseFlags(t *testing.T) {
	flags, err := ParseFlags([]string{"--port", "9090", "--log-level", "debug"})
	if err != nil {
		t.Fatal(err)
	}

	if flags.Port == nil || *flags.Port != "9090" {
		t.Errorf("expected port 9090, got %v", flags.Port)
	}
	if flags.LogLevel == nil || *flags.LogLevel != "debug" {
		t.Errorf("expected log-level debug, got %v", flags.LogLevel)
	}
	// Unset flags remain nil
	if flags.DSN != nil {
		t.Errorf("expected nil DSN, got %v", *flags.DSN)
	}
	if flags.NatsURL != nil {
		t.Errorf("expected nil NatsURL, got %v", *flags.NatsURL)
	}
	if flags.ConfigPath != nil {
		t.Errorf("expected nil ConfigPath, got %v", *flags.ConfigPath)
	}
}

func TestParseFlagsShorthand(t *testing.T) {
	flags, err := ParseFlags([]string{"-p", "7070", "-c", "custom.yaml"})
	if err != nil {
		t.Fatal(err)
	}

	if flags.Port == nil || *flags.Port != "7070" {
		t.Errorf("expected port 7070, got %v", flags.Port)
	}
	if flags.ConfigPath == nil || *flags.ConfigPath != "custom.yaml" {
		t.Errorf("expected config custom.yaml, got %v", flags.ConfigPath)
	}
}

func TestRoutingDefaults(t *testing.T) {
	cfg := Defaults()
	if !cfg.Routing.Enabled {
		t.Error("routing should be enabled by default")
	}
}

func TestRoutingEnvOverride(t *testing.T) {
	cfg := Defaults()
	t.Setenv("CODEFORGE_ROUTING_ENABLED", "false")
	loadEnv(&cfg)
	if cfg.Routing.Enabled {
		t.Error("expected routing.enabled=false from env override")
	}
}

func TestRoutingYAMLOverride(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "test.yaml")
	content := `
routing:
  enabled: false
`
	if err := os.WriteFile(yamlPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Defaults()
	if err := loadYAML(&cfg, yamlPath); err != nil {
		t.Fatal(err)
	}
	if cfg.Routing.Enabled {
		t.Error("expected routing.enabled=false from YAML override")
	}
}

func TestAgentContextEnvOverride(t *testing.T) {
	cfg := Defaults()
	t.Setenv("CODEFORGE_AGENT_CONTEXT_ENABLED", "true")
	t.Setenv("CODEFORGE_AGENT_CONTEXT_BUDGET", "4096")
	t.Setenv("CODEFORGE_AGENT_CONTEXT_PROMPT_RESERVE", "1024")
	loadEnv(&cfg)
	if !cfg.Agent.ContextEnabled {
		t.Error("expected agent.context_enabled=true from env override")
	}
	if cfg.Agent.ContextBudget != 4096 {
		t.Errorf("expected context_budget=4096, got %d", cfg.Agent.ContextBudget)
	}
	if cfg.Agent.ContextPromptReserve != 1024 {
		t.Errorf("expected context_prompt_reserve=1024, got %d", cfg.Agent.ContextPromptReserve)
	}
}

func TestQuarantineEnvOverride(t *testing.T) {
	cfg := Defaults()
	t.Setenv("CODEFORGE_QUARANTINE_ENABLED", "true")
	t.Setenv("CODEFORGE_QUARANTINE_THRESHOLD", "0.5")
	t.Setenv("CODEFORGE_QUARANTINE_BLOCK_THRESHOLD", "0.85")
	t.Setenv("CODEFORGE_QUARANTINE_MIN_TRUST_BYPASS", "full")
	t.Setenv("CODEFORGE_QUARANTINE_EXPIRY_HOURS", "48")
	loadEnv(&cfg)
	if !cfg.Quarantine.Enabled {
		t.Error("expected quarantine.enabled=true from env override")
	}
	if cfg.Quarantine.QuarantineThreshold != 0.5 {
		t.Errorf("expected quarantine_threshold=0.5, got %f", cfg.Quarantine.QuarantineThreshold)
	}
	if cfg.Quarantine.BlockThreshold != 0.85 {
		t.Errorf("expected block_threshold=0.85, got %f", cfg.Quarantine.BlockThreshold)
	}
	if cfg.Quarantine.MinTrustBypass != "full" {
		t.Errorf("expected min_trust_bypass=full, got %s", cfg.Quarantine.MinTrustBypass)
	}
	if cfg.Quarantine.ExpiryHours != 48 {
		t.Errorf("expected expiry_hours=48, got %d", cfg.Quarantine.ExpiryHours)
	}
}

func TestLSPEnvOverride(t *testing.T) {
	cfg := Defaults()
	t.Setenv("CODEFORGE_LSP_ENABLED", "true")
	loadEnv(&cfg)
	if !cfg.LSP.Enabled {
		t.Error("expected lsp.enabled=true from env override")
	}
}

func TestReviewRouterEnvOverride(t *testing.T) {
	cfg := Defaults()
	t.Setenv("CODEFORGE_ORCH_REVIEW_ROUTER_ENABLED", "true")
	t.Setenv("CODEFORGE_ORCH_REVIEW_CONFIDENCE_THRESHOLD", "0.6")
	t.Setenv("CODEFORGE_ORCH_REVIEW_ROUTER_MODEL", "gpt-4o")
	loadEnv(&cfg)
	if !cfg.Orchestrator.ReviewRouterEnabled {
		t.Error("expected review_router_enabled=true from env override")
	}
	if cfg.Orchestrator.ReviewConfidenceThreshold != 0.6 {
		t.Errorf("expected review_confidence_threshold=0.6, got %f", cfg.Orchestrator.ReviewConfidenceThreshold)
	}
	if cfg.Orchestrator.ReviewRouterModel != "gpt-4o" {
		t.Errorf("expected review_router_model=gpt-4o, got %s", cfg.Orchestrator.ReviewRouterModel)
	}
}

func TestParseFlagsInvalid(t *testing.T) {
	_, err := ParseFlags([]string{"--unknown-flag"})
	if err == nil {
		t.Error("expected error for unknown flag, got nil")
	}
}

func TestApplyCLI(t *testing.T) {
	cfg := Defaults()

	port := "3333"
	logLevel := "error"
	dsn := "postgres://cli:cli@localhost/cli"
	natsURL := "nats://cli:4222"

	applyCLI(&cfg, CLIFlags{
		Port:     &port,
		LogLevel: &logLevel,
		DSN:      &dsn,
		NatsURL:  &natsURL,
	})

	if cfg.Server.Port != "3333" {
		t.Errorf("expected port 3333, got %s", cfg.Server.Port)
	}
	if cfg.Logging.Level != "error" {
		t.Errorf("expected log level error, got %s", cfg.Logging.Level)
	}
	if cfg.Postgres.DSN != "postgres://cli:cli@localhost/cli" {
		t.Errorf("expected CLI DSN, got %s", cfg.Postgres.DSN)
	}
	if cfg.NATS.URL != "nats://cli:4222" {
		t.Errorf("expected CLI NATS URL, got %s", cfg.NATS.URL)
	}
}

func TestApplyCLINilFlags(t *testing.T) {
	cfg := Defaults()
	original := cfg

	// All-nil flags should change nothing.
	applyCLI(&cfg, CLIFlags{})

	if cfg.Server.Port != original.Server.Port {
		t.Errorf("port changed from %s to %s", original.Server.Port, cfg.Server.Port)
	}
	if cfg.Logging.Level != original.Logging.Level {
		t.Errorf("log level changed from %s to %s", original.Logging.Level, cfg.Logging.Level)
	}
}

func TestCLIOverridesEnv(t *testing.T) {
	// CLI flags must win over ENV.
	t.Setenv("APP_ENV", "development") // required for default JWT secret
	t.Setenv("CODEFORGE_PORT", "7070")
	t.Setenv("CODEFORGE_LOG_LEVEL", "warn")

	flags, err := ParseFlags([]string{"--port", "3333", "--log-level", "error"})
	if err != nil {
		t.Fatal(err)
	}

	cfg, _, err := LoadWithCLI(flags)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Server.Port != "3333" {
		t.Errorf("expected CLI port 3333 to override ENV 7070, got %s", cfg.Server.Port)
	}
	if cfg.Logging.Level != "error" {
		t.Errorf("expected CLI log-level error to override ENV warn, got %s", cfg.Logging.Level)
	}
}

func TestConfigFileEnvOverride(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "env-config.yaml")
	content := `
server:
  port: "6666"
`
	if err := os.WriteFile(yamlPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("APP_ENV", "development") // required for default JWT secret
	t.Setenv("CODEFORGE_CONFIG_FILE", yamlPath)

	// No CLI flag for config path — env var should be used.
	flags, err := ParseFlags(nil)
	if err != nil {
		t.Fatal(err)
	}

	cfg, resolvedPath, err := LoadWithCLI(flags)
	if err != nil {
		t.Fatal(err)
	}

	if resolvedPath != yamlPath {
		t.Errorf("expected resolved path %s from env, got %s", yamlPath, resolvedPath)
	}
	if cfg.Server.Port != "6666" {
		t.Errorf("expected port 6666 from env-specified YAML, got %s", cfg.Server.Port)
	}
}

func TestCLIConfigOverridesEnvConfig(t *testing.T) {
	dir := t.TempDir()

	// Env config file
	envPath := filepath.Join(dir, "env-config.yaml")
	if err := os.WriteFile(envPath, []byte(`server: { port: "6666" }`), 0o644); err != nil {
		t.Fatal(err)
	}

	// CLI config file
	cliPath := filepath.Join(dir, "cli-config.yaml")
	if err := os.WriteFile(cliPath, []byte(`server: { port: "7777" }`), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("APP_ENV", "development") // required for default JWT secret
	t.Setenv("CODEFORGE_CONFIG_FILE", envPath)

	flags, err := ParseFlags([]string{"--config", cliPath})
	if err != nil {
		t.Fatal(err)
	}

	cfg, resolvedPath, err := LoadWithCLI(flags)
	if err != nil {
		t.Fatal(err)
	}

	if resolvedPath != cliPath {
		t.Errorf("expected CLI config path %s to win over env %s, got %s", cliPath, envPath, resolvedPath)
	}
	if cfg.Server.Port != "7777" {
		t.Errorf("expected port 7777 from CLI config, got %s", cfg.Server.Port)
	}
}

func TestLoadWithCLICustomConfig(t *testing.T) {
	t.Setenv("APP_ENV", "development") // required for default JWT secret
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "custom.yaml")
	content := `
server:
  port: "5555"
`
	if err := os.WriteFile(yamlPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	flags, err := ParseFlags([]string{"--config", yamlPath})
	if err != nil {
		t.Fatal(err)
	}

	cfg, resolvedPath, err := LoadWithCLI(flags)
	if err != nil {
		t.Fatal(err)
	}

	if resolvedPath != yamlPath {
		t.Errorf("expected resolved path %s, got %s", yamlPath, resolvedPath)
	}
	if cfg.Server.Port != "5555" {
		t.Errorf("expected port 5555 from custom YAML, got %s", cfg.Server.Port)
	}
}

func TestEnsureSecrets_AutoGeneratesJWT(t *testing.T) {
	cfg := Defaults()
	if cfg.Auth.JWTSecret != "" {
		t.Fatal("expected empty default JWT secret")
	}
	if err := ensureSecrets(&cfg); err != nil {
		t.Fatalf("ensureSecrets failed: %v", err)
	}
	if cfg.Auth.JWTSecret == "" {
		t.Error("expected auto-generated JWT secret, got empty")
	}
	if len(cfg.Auth.JWTSecret) < 64 {
		t.Errorf("expected at least 64 hex chars from 32-byte token, got %d", len(cfg.Auth.JWTSecret))
	}
}

func TestEnsureSecrets_PreservesExplicit(t *testing.T) {
	cfg := Defaults()
	explicit := "my-explicitly-set-secret-that-should-not-change"
	cfg.Auth.JWTSecret = explicit
	if err := ensureSecrets(&cfg); err != nil {
		t.Fatalf("ensureSecrets failed: %v", err)
	}
	if cfg.Auth.JWTSecret != explicit {
		t.Errorf("ensureSecrets overwrote explicit secret: got %q", cfg.Auth.JWTSecret)
	}
}

func TestIsLowEntropySecret(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		wantLow bool
	}{
		{"too short", "abc", true},
		{"31 chars", "abcdefghijklmnopqrstuvwxyz12345", true},
		{"all same char", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},
		{"hex token 64 chars", "a1b2c3d4e5f67890a1b2c3d4e5f67890a1b2c3d4e5f67890a1b2c3d4e5f67890", false},
		{"alphanumeric random", "Xk9mL2pR7wQn4tFvB8jC3hGdE5sAyU0z", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isLowEntropySecret(tt.secret)
			if got != tt.wantLow {
				t.Errorf("isLowEntropySecret(%q) = %v, want %v", tt.secret, got, tt.wantLow)
			}
		})
	}
}

func TestSensitiveFieldsExcludedFromJSON(t *testing.T) {
	cfg := Defaults()
	cfg.InternalKey = "test-internal-key"
	cfg.Auth.JWTSecret = "test-jwt-secret-that-is-long-enough"
	cfg.Auth.LLMKeyEncryptionSecret = "test-llm-enc-secret"
	cfg.LiteLLM.MasterKey = "test-master-key"
	cfg.Webhook.GitHubSecret = "test-github-secret"
	cfg.Webhook.GitLabToken = "test-gitlab-token"
	cfg.Webhook.PlaneSecret = "test-plane-secret"
	cfg.Notification.SMTPPassword = "test-smtp-pass"
	cfg.GitHub.ClientSecret = "test-client-secret"
	cfg.Plane.APIToken = "test-plane-token"
	cfg.A2A.APIKeys = []string{"test-a2a-key"}
	cfg.MCP.APIKey = "test-mcp-key"

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	jsonStr := string(data)

	sensitiveValues := []struct {
		field string
		value string
	}{
		{"InternalKey", "test-internal-key"},
		{"JWTSecret", "test-jwt-secret-that-is-long-enough"},
		{"LLMKeyEncryptionSecret", "test-llm-enc-secret"},
		{"MasterKey", "test-master-key"},
		{"GitHubSecret", "test-github-secret"},
		{"GitLabToken", "test-gitlab-token"},
		{"PlaneSecret", "test-plane-secret"},
		{"SMTPPassword", "test-smtp-pass"},
		{"ClientSecret", "test-client-secret"},
		{"APIToken (Plane)", "test-plane-token"},
		{"APIKeys (A2A)", "test-a2a-key"},
		{"APIKey (MCP)", "test-mcp-key"},
	}

	for _, sv := range sensitiveValues {
		if strings.Contains(jsonStr, sv.value) {
			t.Errorf("sensitive field %s value %q leaked in JSON output", sv.field, sv.value)
		}
	}
}

func TestBlockedJWTSecrets(t *testing.T) {
	blocked := []string{
		"codeforge-dev-jwt-secret-change-in-production",
		"e2e-test-secret-key-minimum-32-bytes-long",
	}

	for _, secret := range blocked {
		t.Run(secret[:20]+"...", func(t *testing.T) {
			cfg := Defaults()
			cfg.Auth.Enabled = true
			cfg.Auth.JWTSecret = secret
			cfg.AppEnv = "staging"
			cfg.Postgres.DSN = "postgres://u:p@localhost:5432/db?sslmode=require"

			err := validate(&cfg)
			if err == nil {
				t.Errorf("expected error for blocked secret %q in staging, got nil", secret)
			}
		})
	}

	// Verify blocked secrets are allowed in development
	t.Run("allowed in development", func(t *testing.T) {
		cfg := Defaults()
		cfg.Auth.Enabled = true
		cfg.Auth.JWTSecret = "e2e-test-secret-key-minimum-32-bytes-long"
		cfg.AppEnv = "development"
		cfg.Postgres.DSN = "postgres://u:p@localhost:5432/db?sslmode=disable"

		err := validate(&cfg)
		if err != nil {
			t.Errorf("expected no error for blocked secret in development, got: %v", err)
		}
	})
}

func TestSSLModeRejectedInStaging(t *testing.T) {
	tests := []struct {
		name    string
		appEnv  string
		dsn     string
		wantErr bool
	}{
		{"staging + sslmode=disable", "staging", "postgres://u:p@localhost:5432/db?sslmode=disable", true},
		{"production + sslmode=disable", "production", "postgres://u:p@localhost:5432/db?sslmode=disable", true},
		{"development + sslmode=disable", "development", "postgres://u:p@localhost:5432/db?sslmode=disable", false},
		{"staging + sslmode=require", "staging", "postgres://u:p@localhost:5432/db?sslmode=require", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Auth.Enabled = true
			cfg.Auth.JWTSecret = "Xk9mL2pR7wQn4tFvB8jC3hGdE5sAyU0zABCDEFGH"
			cfg.AppEnv = tt.appEnv
			cfg.Postgres.DSN = tt.dsn

			err := validate(&cfg)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for %s, got nil", tt.name)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error for %s: %v", tt.name, err)
			}
		})
	}
}

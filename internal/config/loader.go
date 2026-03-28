package config

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	cfcrypto "github.com/Strob0t/CodeForge/internal/crypto"
)

// DefaultConfigFile is the path checked for YAML configuration.
const DefaultConfigFile = "codeforge.yaml"

// CLIFlags holds command-line flag values. Nil pointers indicate unset flags
// that should not override the config. Use ParseFlags to populate this struct.
type CLIFlags struct {
	ConfigPath *string
	Port       *string
	LogLevel   *string
	DSN        *string
	NatsURL    *string
}

// ParseFlags parses command-line arguments into CLIFlags.
// Call this before Load/LoadWithCLI. Passing nil args parses os.Args[1:].
func ParseFlags(args []string) (CLIFlags, error) {
	var flags CLIFlags

	fs := flag.NewFlagSet("codeforge", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to YAML config file")
	fs.StringVar(configPath, "c", "", "path to YAML config file (shorthand)")
	port := fs.String("port", "", "HTTP server port")
	fs.StringVar(port, "p", "", "HTTP server port (shorthand)")
	logLevel := fs.String("log-level", "", "logging level (debug, info, warn, error)")
	dsn := fs.String("dsn", "", "PostgreSQL connection string")
	natsURL := fs.String("nats-url", "", "NATS server URL")

	if err := fs.Parse(args); err != nil {
		return flags, fmt.Errorf("parse flags: %w", err)
	}

	// Only set pointers for flags that were explicitly provided.
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "config", "c":
			flags.ConfigPath = configPath
		case "port", "p":
			flags.Port = port
		case "log-level":
			flags.LogLevel = logLevel
		case "dsn":
			flags.DSN = dsn
		case "nats-url":
			flags.NatsURL = natsURL
		}
	})

	return flags, nil
}

// Load returns a Config using the hierarchy: defaults < YAML < ENV.
// YAML file is optional; missing file is not an error.
func Load() (*Config, error) {
	return LoadFrom(DefaultConfigFile)
}

// LoadWithCLI returns a Config using the full hierarchy:
// defaults < YAML < ENV < CLI flags. The YAML path can be overridden
// via CLIFlags.ConfigPath.
func LoadWithCLI(flags CLIFlags) (*Config, string, error) {
	yamlPath := DefaultConfigFile
	if v := os.Getenv("CODEFORGE_CONFIG_FILE"); v != "" {
		yamlPath = v
	}
	if flags.ConfigPath != nil {
		yamlPath = *flags.ConfigPath
	}

	cfg := Defaults()

	if err := loadYAML(&cfg, yamlPath); err != nil {
		return nil, "", fmt.Errorf("config yaml: %w", err)
	}

	loadEnv(&cfg)
	applyCLI(&cfg, flags)

	if err := ensureSecrets(&cfg); err != nil {
		return nil, "", fmt.Errorf("config secrets: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, "", fmt.Errorf("config validate: %w", err)
	}

	return &cfg, yamlPath, nil
}

// LoadFrom returns a Config loaded from the given YAML path using the
// hierarchy: defaults < YAML < ENV. The YAML file is optional.
func LoadFrom(yamlPath string) (*Config, error) {
	cfg := Defaults()

	if err := loadYAML(&cfg, yamlPath); err != nil {
		return nil, fmt.Errorf("config yaml: %w", err)
	}

	loadEnv(&cfg)

	if err := ensureSecrets(&cfg); err != nil {
		return nil, fmt.Errorf("config secrets: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("config validate: %w", err)
	}

	return &cfg, nil
}

// applyCLI overlays CLI flag values onto cfg. Only non-nil flags override.
func applyCLI(cfg *Config, flags CLIFlags) {
	if flags.Port != nil {
		cfg.Server.Port = *flags.Port
	}
	if flags.LogLevel != nil {
		cfg.Logging.Level = *flags.LogLevel
	}
	if flags.DSN != nil {
		cfg.Postgres.DSN = *flags.DSN
	}
	if flags.NatsURL != nil {
		cfg.NATS.URL = *flags.NatsURL
	}
}

// loadYAML reads the YAML file and unmarshals it over cfg.
// Returns nil if the file does not exist.
func loadYAML(cfg *Config, path string) error {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is validated by caller
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	return nil
}

// loadEnv overlays environment variables onto cfg.
// Only non-empty env values override the current config.
func loadEnv(cfg *Config) {
	// Top-level
	setString(&cfg.AppEnv, "APP_ENV")
	setString(&cfg.InternalKey, "CODEFORGE_INTERNAL_KEY")

	setString(&cfg.Server.Port, "CODEFORGE_PORT")
	setString(&cfg.Server.CORSOrigin, "CODEFORGE_CORS_ORIGIN")
	setString(&cfg.Postgres.DSN, "DATABASE_URL")
	setTyped(&cfg.Postgres.MaxConns, "CODEFORGE_PG_MAX_CONNS", func(s string) (int32, error) { n, err := strconv.ParseInt(s, 10, 32); return int32(n), err })
	setTyped(&cfg.Postgres.MinConns, "CODEFORGE_PG_MIN_CONNS", func(s string) (int32, error) { n, err := strconv.ParseInt(s, 10, 32); return int32(n), err })
	setTyped(&cfg.Postgres.MaxConnLifetime, "CODEFORGE_PG_MAX_CONN_LIFETIME", time.ParseDuration)
	setTyped(&cfg.Postgres.MaxConnIdleTime, "CODEFORGE_PG_MAX_CONN_IDLE_TIME", time.ParseDuration)
	setTyped(&cfg.Postgres.HealthCheck, "CODEFORGE_PG_HEALTH_CHECK", time.ParseDuration)
	setString(&cfg.NATS.URL, "NATS_URL")
	setString(&cfg.LiteLLM.URL, "LITELLM_BASE_URL")
	setString(&cfg.LiteLLM.MasterKey, "LITELLM_MASTER_KEY")
	setString(&cfg.LiteLLM.ConversationModel, "CODEFORGE_CONVERSATION_MODEL")
	setString(&cfg.Logging.Level, "CODEFORGE_LOG_LEVEL")
	setString(&cfg.Logging.Service, "CODEFORGE_LOG_SERVICE")
	setTyped(&cfg.Logging.Async, "CODEFORGE_LOG_ASYNC", strconv.ParseBool)
	setTyped(&cfg.Breaker.MaxFailures, "CODEFORGE_BREAKER_MAX_FAILURES", strconv.Atoi)
	setTyped(&cfg.Breaker.Timeout, "CODEFORGE_BREAKER_TIMEOUT", time.ParseDuration)
	setTyped(&cfg.Rate.RequestsPerSecond, "CODEFORGE_RATE_RPS", func(s string) (float64, error) { return strconv.ParseFloat(s, 64) })
	setTyped(&cfg.Rate.Burst, "CODEFORGE_RATE_BURST", strconv.Atoi)
	setTyped(&cfg.Rate.CleanupInterval, "CODEFORGE_RATE_CLEANUP_INTERVAL", time.ParseDuration)
	setTyped(&cfg.Rate.MaxIdleTime, "CODEFORGE_RATE_MAX_IDLE_TIME", time.ParseDuration)
	setTyped(&cfg.Rate.AuthPerSecond, "CODEFORGE_RATE_AUTH_RPS", func(s string) (float64, error) { return strconv.ParseFloat(s, 64) })
	setTyped(&cfg.Rate.AuthBurst, "CODEFORGE_RATE_AUTH_BURST", strconv.Atoi)
	setTyped(&cfg.Git.MaxConcurrent, "CODEFORGE_GIT_MAX_CONCURRENT", strconv.Atoi)
	setString(&cfg.Policy.DefaultProfile, "CODEFORGE_POLICY_DEFAULT")
	setString(&cfg.Policy.CustomDir, "CODEFORGE_POLICY_DIR")
	setString(&cfg.Workspace.Root, "CODEFORGE_WORKSPACE_ROOT")
	setString(&cfg.Workspace.PipelineDir, "CODEFORGE_WORKSPACE_PIPELINE_DIR")
	setTyped(&cfg.Runtime.StallThreshold, "CODEFORGE_STALL_THRESHOLD", strconv.Atoi)
	setTyped(&cfg.Runtime.StallMaxRetries, "CODEFORGE_STALL_MAX_RETRIES", strconv.Atoi)
	setTyped(&cfg.Runtime.QualityGateTimeout, "CODEFORGE_QG_TIMEOUT", time.ParseDuration)
	setString(&cfg.Runtime.DefaultDeliverMode, "CODEFORGE_DELIVER_MODE")
	setString(&cfg.Runtime.DefaultTestCommand, "CODEFORGE_TEST_COMMAND")
	setString(&cfg.Runtime.DefaultLintCommand, "CODEFORGE_LINT_COMMAND")
	setString(&cfg.Runtime.DeliveryCommitPrefix, "CODEFORGE_COMMIT_PREFIX")
	setTyped(&cfg.Runtime.HeartbeatInterval, "CODEFORGE_HEARTBEAT_INTERVAL", time.ParseDuration)
	setTyped(&cfg.Runtime.HeartbeatTimeout, "CODEFORGE_HEARTBEAT_TIMEOUT", time.ParseDuration)
	setTyped(&cfg.Runtime.ApprovalTimeoutSeconds, "CODEFORGE_APPROVAL_TIMEOUT_SECONDS", strconv.Atoi)

	// Idempotency
	setString(&cfg.Idempotency.Bucket, "CODEFORGE_IDEMPOTENCY_BUCKET")
	setTyped(&cfg.Idempotency.TTL, "CODEFORGE_IDEMPOTENCY_TTL", time.ParseDuration)

	// Hybrid
	setString(&cfg.Runtime.Hybrid.CommandImage, "CODEFORGE_HYBRID_IMAGE")
	setString(&cfg.Runtime.Hybrid.MountMode, "CODEFORGE_HYBRID_MOUNT_MODE")

	// Sandbox
	setTyped(&cfg.Runtime.Sandbox.MemoryMB, "CODEFORGE_SANDBOX_MEMORY_MB", strconv.Atoi)
	setTyped(&cfg.Runtime.Sandbox.CPUQuota, "CODEFORGE_SANDBOX_CPU_QUOTA", strconv.Atoi)
	setTyped(&cfg.Runtime.Sandbox.PidsLimit, "CODEFORGE_SANDBOX_PIDS_LIMIT", strconv.Atoi)
	setTyped(&cfg.Runtime.Sandbox.StorageGB, "CODEFORGE_SANDBOX_STORAGE_GB", strconv.Atoi)
	setString(&cfg.Runtime.Sandbox.NetworkMode, "CODEFORGE_SANDBOX_NETWORK")
	setString(&cfg.Runtime.Sandbox.Image, "CODEFORGE_SANDBOX_IMAGE")

	// Cache
	setTyped(&cfg.Cache.L1MaxSizeMB, "CODEFORGE_CACHE_L1_SIZE_MB", func(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) })
	setString(&cfg.Cache.L2Bucket, "CODEFORGE_CACHE_L2_BUCKET")
	setTyped(&cfg.Cache.L2TTL, "CODEFORGE_CACHE_L2_TTL", time.ParseDuration)

	// Orchestrator
	setTyped(&cfg.Orchestrator.MaxParallel, "CODEFORGE_ORCH_MAX_PARALLEL", strconv.Atoi)
	setTyped(&cfg.Orchestrator.PingPongMaxRounds, "CODEFORGE_ORCH_PINGPONG_MAX_ROUNDS", strconv.Atoi)
	setTyped(&cfg.Orchestrator.ConsensusQuorum, "CODEFORGE_ORCH_CONSENSUS_QUORUM", strconv.Atoi)
	setString(&cfg.Orchestrator.Mode, "CODEFORGE_ORCH_MODE")
	setString(&cfg.Orchestrator.DecomposeModel, "CODEFORGE_ORCH_DECOMPOSE_MODEL")
	setTyped(&cfg.Orchestrator.DecomposeMaxTokens, "CODEFORGE_ORCH_DECOMPOSE_MAX_TOKENS", strconv.Atoi)
	setTyped(&cfg.Orchestrator.MaxTeamSize, "CODEFORGE_ORCH_MAX_TEAM_SIZE", strconv.Atoi)
	setTyped(&cfg.Orchestrator.DefaultContextBudget, "CODEFORGE_ORCH_CONTEXT_BUDGET", strconv.Atoi)
	setTyped(&cfg.Orchestrator.PromptReserve, "CODEFORGE_ORCH_PROMPT_RESERVE", strconv.Atoi)
	setTyped(&cfg.Orchestrator.SubAgentEnabled, "CODEFORGE_ORCH_SUBAGENT_ENABLED", strconv.ParseBool)
	setString(&cfg.Orchestrator.SubAgentModel, "CODEFORGE_ORCH_SUBAGENT_MODEL")
	setTyped(&cfg.Orchestrator.SubAgentMaxQueries, "CODEFORGE_ORCH_SUBAGENT_MAX_QUERIES", strconv.Atoi)
	setTyped(&cfg.Orchestrator.SubAgentRerank, "CODEFORGE_ORCH_SUBAGENT_RERANK", strconv.ParseBool)
	setTyped(&cfg.Orchestrator.SubAgentTimeout, "CODEFORGE_ORCH_SUBAGENT_TIMEOUT", time.ParseDuration)
	setTyped(&cfg.Orchestrator.ReviewRouterEnabled, "CODEFORGE_ORCH_REVIEW_ROUTER_ENABLED", strconv.ParseBool)
	setTyped(&cfg.Orchestrator.ReviewConfidenceThreshold, "CODEFORGE_ORCH_REVIEW_CONFIDENCE_THRESHOLD", func(s string) (float64, error) { return strconv.ParseFloat(s, 64) })
	setString(&cfg.Orchestrator.ReviewRouterModel, "CODEFORGE_ORCH_REVIEW_ROUTER_MODEL")

	// GraphRAG
	setTyped(&cfg.Orchestrator.GraphEnabled, "CODEFORGE_ORCH_GRAPH_ENABLED", strconv.ParseBool)
	setTyped(&cfg.Orchestrator.GraphMaxHops, "CODEFORGE_ORCH_GRAPH_MAX_HOPS", strconv.Atoi)
	setTyped(&cfg.Orchestrator.GraphTopK, "CODEFORGE_ORCH_GRAPH_TOP_K", strconv.Atoi)
	setTyped(&cfg.Orchestrator.GraphHopDecay, "CODEFORGE_ORCH_GRAPH_HOP_DECAY", func(s string) (float64, error) { return strconv.ParseFloat(s, 64) })

	// Context re-ranking
	setTyped(&cfg.Orchestrator.ContextRerankEnabled, "CODEFORGE_CONTEXT_RERANK_ENABLED", strconv.ParseBool)
	setString(&cfg.Orchestrator.ContextRerankModel, "CODEFORGE_CONTEXT_RERANK_MODEL")

	// Webhook
	setString(&cfg.Webhook.GitHubSecret, "CODEFORGE_WEBHOOK_GITHUB_SECRET")
	setString(&cfg.Webhook.GitLabToken, "CODEFORGE_WEBHOOK_GITLAB_TOKEN")
	setString(&cfg.Webhook.PlaneSecret, "CODEFORGE_WEBHOOK_PLANE_SECRET")

	// Notification
	setString(&cfg.Notification.SlackWebhookURL, "CODEFORGE_NOTIFICATION_SLACK_WEBHOOK_URL")
	setString(&cfg.Notification.DiscordWebhookURL, "CODEFORGE_NOTIFICATION_DISCORD_WEBHOOK_URL")

	// OpenTelemetry
	setTyped(&cfg.OTEL.Enabled, "CODEFORGE_OTEL_ENABLED", strconv.ParseBool)
	setString(&cfg.OTEL.Endpoint, "CODEFORGE_OTEL_ENDPOINT")
	setString(&cfg.OTEL.ServiceName, "CODEFORGE_OTEL_SERVICE_NAME")
	setTyped(&cfg.OTEL.Insecure, "CODEFORGE_OTEL_INSECURE", strconv.ParseBool)
	setTyped(&cfg.OTEL.SampleRate, "CODEFORGE_OTEL_SAMPLE_RATE", func(s string) (float64, error) { return strconv.ParseFloat(s, 64) })

	// A2A
	setTyped(&cfg.A2A.Enabled, "CODEFORGE_A2A_ENABLED", strconv.ParseBool)
	setString(&cfg.A2A.BaseURL, "CODEFORGE_A2A_BASE_URL")
	setStringSlice(&cfg.A2A.APIKeys, "CODEFORGE_A2A_API_KEYS")
	setString(&cfg.A2A.Transport, "CODEFORGE_A2A_TRANSPORT")
	setTyped(&cfg.A2A.MaxTasks, "CODEFORGE_A2A_MAX_TASKS", strconv.Atoi)
	setTyped(&cfg.A2A.AllowOpen, "CODEFORGE_A2A_ALLOW_OPEN", strconv.ParseBool)
	setTyped(&cfg.A2A.Streaming, "CODEFORGE_A2A_STREAMING", strconv.ParseBool)

	// AG-UI
	setTyped(&cfg.AGUI.Enabled, "CODEFORGE_AGUI_ENABLED", strconv.ParseBool)

	// MCP
	setTyped(&cfg.MCP.Enabled, "CODEFORGE_MCP_ENABLED", strconv.ParseBool)
	setString(&cfg.MCP.ServersDir, "CODEFORGE_MCP_SERVERS_DIR")
	setTyped(&cfg.MCP.ServerPort, "CODEFORGE_MCP_SERVER_PORT", strconv.Atoi)

	// Agent
	setString(&cfg.Agent.DefaultModel, "CODEFORGE_AGENT_DEFAULT_MODEL")
	setTyped(&cfg.Agent.MaxContextTokens, "CODEFORGE_AGENT_MAX_CONTEXT_TOKENS", strconv.Atoi)
	setTyped(&cfg.Agent.MaxLoopIterations, "CODEFORGE_AGENT_MAX_LOOP_ITERATIONS", strconv.Atoi)
	setTyped(&cfg.Agent.AgenticByDefault, "CODEFORGE_AGENT_AGENTIC_BY_DEFAULT", strconv.ParseBool)
	setTyped(&cfg.Agent.ToolOutputMaxChars, "CODEFORGE_AGENT_TOOL_OUTPUT_MAX_CHARS", strconv.Atoi)
	setTyped(&cfg.Agent.ContextEnabled, "CODEFORGE_AGENT_CONTEXT_ENABLED", strconv.ParseBool)
	setTyped(&cfg.Agent.ContextBudget, "CODEFORGE_AGENT_CONTEXT_BUDGET", strconv.Atoi)
	setTyped(&cfg.Agent.ContextPromptReserve, "CODEFORGE_AGENT_CONTEXT_PROMPT_RESERVE", strconv.Atoi)
	setTyped(&cfg.Agent.ConversationRolloutCount, "CODEFORGE_AGENT_CONVERSATION_ROLLOUT_COUNT", strconv.Atoi)
	setTyped(&cfg.Agent.SummarizeThreshold, "CODEFORGE_SUMMARIZE_THRESHOLD", strconv.Atoi)

	// Quarantine
	setTyped(&cfg.Quarantine.Enabled, "CODEFORGE_QUARANTINE_ENABLED", strconv.ParseBool)
	setTyped(&cfg.Quarantine.QuarantineThreshold, "CODEFORGE_QUARANTINE_THRESHOLD", func(s string) (float64, error) { return strconv.ParseFloat(s, 64) })
	setTyped(&cfg.Quarantine.BlockThreshold, "CODEFORGE_QUARANTINE_BLOCK_THRESHOLD", func(s string) (float64, error) { return strconv.ParseFloat(s, 64) })
	setString(&cfg.Quarantine.MinTrustBypass, "CODEFORGE_QUARANTINE_MIN_TRUST_BYPASS")
	setTyped(&cfg.Quarantine.ExpiryHours, "CODEFORGE_QUARANTINE_EXPIRY_HOURS", strconv.Atoi)

	// LSP
	setTyped(&cfg.LSP.Enabled, "CODEFORGE_LSP_ENABLED", strconv.ParseBool)

	// Auth
	setTyped(&cfg.Auth.Enabled, "CODEFORGE_AUTH_ENABLED", strconv.ParseBool)
	setString(&cfg.Auth.JWTSecret, "CODEFORGE_AUTH_JWT_SECRET")
	setTyped(&cfg.Auth.AccessTokenExpiry, "CODEFORGE_AUTH_ACCESS_EXPIRY", time.ParseDuration)
	setTyped(&cfg.Auth.RefreshTokenExpiry, "CODEFORGE_AUTH_REFRESH_EXPIRY", time.ParseDuration)
	setTyped(&cfg.Auth.BcryptCost, "CODEFORGE_AUTH_BCRYPT_COST", strconv.Atoi)
	setString(&cfg.Auth.DefaultAdminEmail, "CODEFORGE_AUTH_ADMIN_EMAIL")
	setString(&cfg.Auth.DefaultAdminPass, "CODEFORGE_AUTH_ADMIN_PASS")
	setTyped(&cfg.Auth.AutoGenerateInitialPassword, "CODEFORGE_AUTH_AUTO_GENERATE_PASSWORD", strconv.ParseBool)
	setString(&cfg.Auth.InitialPasswordFile, "CODEFORGE_AUTH_INITIAL_PASSWORD_FILE")
	setTyped(&cfg.Auth.SetupTimeoutMinutes, "CODEFORGE_AUTH_SETUP_TIMEOUT_MINUTES", strconv.Atoi)

	// LiteLLM health polling
	setTyped(&cfg.LiteLLM.HealthPollInterval, "CODEFORGE_LITELLM_HEALTH_POLL_INTERVAL", time.ParseDuration)

	// Copilot
	setTyped(&cfg.Copilot.Enabled, "CODEFORGE_COPILOT_ENABLED", strconv.ParseBool)
	setString(&cfg.Copilot.HostsFilePath, "CODEFORGE_COPILOT_HOSTS_FILE")

	// GitHub OAuth
	setString(&cfg.GitHub.ClientID, "GITHUB_CLIENT_ID")
	setString(&cfg.GitHub.ClientSecret, "GITHUB_CLIENT_SECRET")
	setString(&cfg.GitHub.CallbackURL, "GITHUB_CALLBACK_URL")

	// Routing
	setTyped(&cfg.Routing.Enabled, "CODEFORGE_ROUTING_ENABLED", strconv.ParseBool)

	// Experience Pool
	setTyped(&cfg.Experience.Enabled, "CODEFORGE_EXPERIENCE_ENABLED", strconv.ParseBool)
	setTyped(&cfg.Experience.ConfidenceThreshold, "CODEFORGE_EXPERIENCE_CONFIDENCE_THRESHOLD", func(s string) (float64, error) { return strconv.ParseFloat(s, 64) })
	setTyped(&cfg.Experience.MaxEntries, "CODEFORGE_EXPERIENCE_MAX_ENTRIES", strconv.Atoi)

	// Email / SMTP (for feedback providers)
	setString(&cfg.Notification.SMTPHost, "CODEFORGE_SMTP_HOST")
	setTyped(&cfg.Notification.SMTPPort, "CODEFORGE_SMTP_PORT", strconv.Atoi)
	setString(&cfg.Notification.SMTPFrom, "CODEFORGE_SMTP_FROM")
	setString(&cfg.Notification.SMTPPassword, "CODEFORGE_SMTP_PASSWORD")

	// Benchmark
	setTyped(&cfg.Benchmark.WatchdogTimeout, "CODEFORGE_BENCHMARK_WATCHDOG_TIMEOUT", time.ParseDuration)

	// Ollama
	setString(&cfg.Ollama.BaseURL, "OLLAMA_BASE_URL")

	// Plane
	setString(&cfg.Plane.APIToken, "CODEFORGE_PLANE_API_TOKEN")

	// Retention
	setTyped(&cfg.Retention.Sessions, "CODEFORGE_RETENTION_SESSIONS", time.ParseDuration)
	setTyped(&cfg.Retention.Conversations, "CODEFORGE_RETENTION_CONVERSATIONS", time.ParseDuration)
	setTyped(&cfg.Retention.CostRecords, "CODEFORGE_RETENTION_COST_RECORDS", time.ParseDuration)
	setTyped(&cfg.Retention.AuditEntries, "CODEFORGE_RETENTION_AUDIT_ENTRIES", time.ParseDuration)

	// Env file override
	setString(&cfg.EnvFile, "CODEFORGE_ENV_FILE")
}

// validate checks that required fields are set and security constraints are met.
func validate(cfg *Config) error {
	if cfg.Server.Port == "" {
		return errors.New("server.port is required")
	}
	if cfg.Postgres.DSN == "" {
		return errors.New("postgres.dsn is required")
	}
	if cfg.NATS.URL == "" {
		return errors.New("nats.url is required")
	}
	if cfg.Postgres.MaxConns < 1 {
		return errors.New("postgres.max_conns must be >= 1")
	}
	if cfg.Breaker.MaxFailures < 1 {
		return errors.New("breaker.max_failures must be >= 1")
	}
	if cfg.Rate.Burst < 1 {
		return errors.New("rate.burst must be >= 1")
	}

	// Auth validation: reject empty JWT secret when auth is enabled.
	if cfg.Auth.Enabled && cfg.Auth.JWTSecret == "" {
		return errors.New("auth.jwt_secret is required when auth.enabled is true")
	}

	// Auth validation: reject well-known default/test secrets (blocklist).
	blockedSecrets := []string{
		"codeforge-dev-jwt-secret-change-in-production",
		"e2e-test-secret-key-minimum-32-bytes-long",
		"changeme",
		"secret",
		"password",
		"test-secret",
	}
	if cfg.Auth.Enabled {
		for _, blocked := range blockedSecrets {
			if cfg.Auth.JWTSecret == blocked {
				if cfg.AppEnv != "development" {
					return fmt.Errorf("blocked JWT secret %q is only allowed when APP_ENV=development -- set CODEFORGE_AUTH_JWT_SECRET to a unique secret (>= 32 chars)", blocked)
				}
				slog.Warn("using blocked JWT secret -- acceptable only for local development", "secret_prefix", blocked[:8]+"...")
				break
			}
		}
	}

	// Auth validation: enforce minimum JWT secret length when auth is enabled.
	if cfg.Auth.Enabled && len(cfg.Auth.JWTSecret) < 32 {
		return fmt.Errorf("auth.jwt_secret must be at least 32 characters when auth is enabled (got %d) -- set CODEFORGE_AUTH_JWT_SECRET", len(cfg.Auth.JWTSecret))
	}

	// Auth validation: entropy-based rejection for production (Shannon entropy >= 3.0 bits/char).
	if cfg.Auth.Enabled && cfg.AppEnv != "development" {
		if isLowEntropySecret(cfg.Auth.JWTSecret) {
			return errors.New("auth.jwt_secret has insufficient entropy -- use a cryptographically random value (>= 32 chars, >= 3.0 bits/char entropy)")
		}
	}

	// Auth validation: enforce minimum bcrypt cost for security.
	if cfg.Auth.BcryptCost < 12 {
		return errors.New("auth.bcrypt_cost must be >= 12")
	}
	if cfg.Auth.BcryptCost > 31 {
		return errors.New("auth.bcrypt_cost must be <= 31 (values above 20 cause multi-second login delays)")
	}

	// Auth validation: reject well-known default admin passwords in non-development environments.
	if cfg.Auth.Enabled {
		p := strings.ToLower(cfg.Auth.DefaultAdminPass)
		isDefaultPassword := p == "changeme123" || p == "admin" || p == "password" || p == "change_me_on_first_boot"
		if isDefaultPassword {
			if cfg.AppEnv != "development" {
				return fmt.Errorf("auth.default_admin_pass is a well-known default (%q) -- set CODEFORGE_AUTH_ADMIN_PASS to a strong password or enable auto_generate_initial_password", cfg.Auth.DefaultAdminPass)
			}
			slog.Warn("auth.default_admin_pass is set to a well-known default -- acceptable only for local development")
		}
	}

	// PostgreSQL validation: reject sslmode=disable in non-development environments.
	if cfg.AppEnv != "development" && cfg.AppEnv != "" && strings.Contains(cfg.Postgres.DSN, "sslmode=disable") {
		return fmt.Errorf("postgres.dsn must not use sslmode=disable when APP_ENV=%s -- use sslmode=require or sslmode=verify-full", cfg.AppEnv)
	}

	return nil
}

// ensureSecrets auto-generates missing secrets on first boot.
// Called AFTER env var loading but BEFORE validation so that
// auto-generated values pass the entropy check.
func ensureSecrets(cfg *Config) error {
	if cfg.Auth.Enabled && cfg.Auth.JWTSecret == "" {
		token, err := cfcrypto.GenerateRandomToken()
		if err != nil {
			return fmt.Errorf("generate JWT secret: %w", err)
		}
		cfg.Auth.JWTSecret = token
		slog.Info("auto-generated JWT secret -- persists only in memory; set CODEFORGE_AUTH_JWT_SECRET env var to stabilize across restarts")
	}
	return nil
}

// isLowEntropySecret checks if a string has insufficient entropy for cryptographic use.
// Returns true when the secret is shorter than 32 characters or has less than
// 3.0 bits of Shannon entropy per character.
func isLowEntropySecret(s string) bool {
	if len(s) < 32 {
		return true
	}
	freq := make(map[rune]float64)
	for _, c := range s {
		freq[c]++
	}
	length := float64(utf8.RuneCountInString(s))
	var entropy float64
	for _, count := range freq {
		p := count / length
		entropy -= p * math.Log2(p)
	}
	return entropy < 3.0
}

func setString(dst *string, key string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

// setTyped reads an environment variable and parses it with the given function.
// If the variable is empty or parsing fails, the destination is left unchanged.
func setTyped[T any](dst *T, key string, parse func(string) (T, error)) {
	v := os.Getenv(key)
	if v == "" {
		return
	}
	val, err := parse(v)
	if err != nil {
		slog.Warn("ignoring invalid config value", "key", key, "value", v, "error", err)
		return
	}
	*dst = val
}

func setStringSlice(dst *[]string, key string) {
	if v := os.Getenv(key); v != "" {
		parts := strings.Split(v, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				result = append(result, p)
			}
		}
		*dst = result
	}
}

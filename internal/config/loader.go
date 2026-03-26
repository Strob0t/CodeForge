package config

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
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
	setInt32(&cfg.Postgres.MaxConns, "CODEFORGE_PG_MAX_CONNS")
	setInt32(&cfg.Postgres.MinConns, "CODEFORGE_PG_MIN_CONNS")
	setDuration(&cfg.Postgres.MaxConnLifetime, "CODEFORGE_PG_MAX_CONN_LIFETIME")
	setDuration(&cfg.Postgres.MaxConnIdleTime, "CODEFORGE_PG_MAX_CONN_IDLE_TIME")
	setDuration(&cfg.Postgres.HealthCheck, "CODEFORGE_PG_HEALTH_CHECK")
	setString(&cfg.NATS.URL, "NATS_URL")
	setString(&cfg.LiteLLM.URL, "LITELLM_BASE_URL")
	setString(&cfg.LiteLLM.MasterKey, "LITELLM_MASTER_KEY")
	setString(&cfg.LiteLLM.ConversationModel, "CODEFORGE_CONVERSATION_MODEL")
	setString(&cfg.Logging.Level, "CODEFORGE_LOG_LEVEL")
	setString(&cfg.Logging.Service, "CODEFORGE_LOG_SERVICE")
	setBool(&cfg.Logging.Async, "CODEFORGE_LOG_ASYNC")
	setInt(&cfg.Breaker.MaxFailures, "CODEFORGE_BREAKER_MAX_FAILURES")
	setDuration(&cfg.Breaker.Timeout, "CODEFORGE_BREAKER_TIMEOUT")
	setFloat64(&cfg.Rate.RequestsPerSecond, "CODEFORGE_RATE_RPS")
	setInt(&cfg.Rate.Burst, "CODEFORGE_RATE_BURST")
	setDuration(&cfg.Rate.CleanupInterval, "CODEFORGE_RATE_CLEANUP_INTERVAL")
	setDuration(&cfg.Rate.MaxIdleTime, "CODEFORGE_RATE_MAX_IDLE_TIME")
	setFloat64(&cfg.Rate.AuthPerSecond, "CODEFORGE_RATE_AUTH_RPS")
	setInt(&cfg.Rate.AuthBurst, "CODEFORGE_RATE_AUTH_BURST")
	setInt(&cfg.Git.MaxConcurrent, "CODEFORGE_GIT_MAX_CONCURRENT")
	setString(&cfg.Policy.DefaultProfile, "CODEFORGE_POLICY_DEFAULT")
	setString(&cfg.Policy.CustomDir, "CODEFORGE_POLICY_DIR")
	setString(&cfg.Workspace.Root, "CODEFORGE_WORKSPACE_ROOT")
	setString(&cfg.Workspace.PipelineDir, "CODEFORGE_WORKSPACE_PIPELINE_DIR")
	setInt(&cfg.Runtime.StallThreshold, "CODEFORGE_STALL_THRESHOLD")
	setInt(&cfg.Runtime.StallMaxRetries, "CODEFORGE_STALL_MAX_RETRIES")
	setDuration(&cfg.Runtime.QualityGateTimeout, "CODEFORGE_QG_TIMEOUT")
	setString(&cfg.Runtime.DefaultDeliverMode, "CODEFORGE_DELIVER_MODE")
	setString(&cfg.Runtime.DefaultTestCommand, "CODEFORGE_TEST_COMMAND")
	setString(&cfg.Runtime.DefaultLintCommand, "CODEFORGE_LINT_COMMAND")
	setString(&cfg.Runtime.DeliveryCommitPrefix, "CODEFORGE_COMMIT_PREFIX")
	setDuration(&cfg.Runtime.HeartbeatInterval, "CODEFORGE_HEARTBEAT_INTERVAL")
	setDuration(&cfg.Runtime.HeartbeatTimeout, "CODEFORGE_HEARTBEAT_TIMEOUT")
	setInt(&cfg.Runtime.ApprovalTimeoutSeconds, "CODEFORGE_APPROVAL_TIMEOUT_SECONDS")

	// Idempotency
	setString(&cfg.Idempotency.Bucket, "CODEFORGE_IDEMPOTENCY_BUCKET")
	setDuration(&cfg.Idempotency.TTL, "CODEFORGE_IDEMPOTENCY_TTL")

	// Hybrid
	setString(&cfg.Runtime.Hybrid.CommandImage, "CODEFORGE_HYBRID_IMAGE")
	setString(&cfg.Runtime.Hybrid.MountMode, "CODEFORGE_HYBRID_MOUNT_MODE")

	// Sandbox
	setInt(&cfg.Runtime.Sandbox.MemoryMB, "CODEFORGE_SANDBOX_MEMORY_MB")
	setInt(&cfg.Runtime.Sandbox.CPUQuota, "CODEFORGE_SANDBOX_CPU_QUOTA")
	setInt(&cfg.Runtime.Sandbox.PidsLimit, "CODEFORGE_SANDBOX_PIDS_LIMIT")
	setInt(&cfg.Runtime.Sandbox.StorageGB, "CODEFORGE_SANDBOX_STORAGE_GB")
	setString(&cfg.Runtime.Sandbox.NetworkMode, "CODEFORGE_SANDBOX_NETWORK")
	setString(&cfg.Runtime.Sandbox.Image, "CODEFORGE_SANDBOX_IMAGE")

	// Cache
	setInt64(&cfg.Cache.L1MaxSizeMB, "CODEFORGE_CACHE_L1_SIZE_MB")
	setString(&cfg.Cache.L2Bucket, "CODEFORGE_CACHE_L2_BUCKET")
	setDuration(&cfg.Cache.L2TTL, "CODEFORGE_CACHE_L2_TTL")

	// Orchestrator
	setInt(&cfg.Orchestrator.MaxParallel, "CODEFORGE_ORCH_MAX_PARALLEL")
	setInt(&cfg.Orchestrator.PingPongMaxRounds, "CODEFORGE_ORCH_PINGPONG_MAX_ROUNDS")
	setInt(&cfg.Orchestrator.ConsensusQuorum, "CODEFORGE_ORCH_CONSENSUS_QUORUM")
	setString(&cfg.Orchestrator.Mode, "CODEFORGE_ORCH_MODE")
	setString(&cfg.Orchestrator.DecomposeModel, "CODEFORGE_ORCH_DECOMPOSE_MODEL")
	setInt(&cfg.Orchestrator.DecomposeMaxTokens, "CODEFORGE_ORCH_DECOMPOSE_MAX_TOKENS")
	setInt(&cfg.Orchestrator.MaxTeamSize, "CODEFORGE_ORCH_MAX_TEAM_SIZE")
	setInt(&cfg.Orchestrator.DefaultContextBudget, "CODEFORGE_ORCH_CONTEXT_BUDGET")
	setInt(&cfg.Orchestrator.PromptReserve, "CODEFORGE_ORCH_PROMPT_RESERVE")
	setBool(&cfg.Orchestrator.SubAgentEnabled, "CODEFORGE_ORCH_SUBAGENT_ENABLED")
	setString(&cfg.Orchestrator.SubAgentModel, "CODEFORGE_ORCH_SUBAGENT_MODEL")
	setInt(&cfg.Orchestrator.SubAgentMaxQueries, "CODEFORGE_ORCH_SUBAGENT_MAX_QUERIES")
	setBool(&cfg.Orchestrator.SubAgentRerank, "CODEFORGE_ORCH_SUBAGENT_RERANK")
	setDuration(&cfg.Orchestrator.SubAgentTimeout, "CODEFORGE_ORCH_SUBAGENT_TIMEOUT")
	setBool(&cfg.Orchestrator.ReviewRouterEnabled, "CODEFORGE_ORCH_REVIEW_ROUTER_ENABLED")
	setFloat64(&cfg.Orchestrator.ReviewConfidenceThreshold, "CODEFORGE_ORCH_REVIEW_CONFIDENCE_THRESHOLD")
	setString(&cfg.Orchestrator.ReviewRouterModel, "CODEFORGE_ORCH_REVIEW_ROUTER_MODEL")

	// GraphRAG
	setBool(&cfg.Orchestrator.GraphEnabled, "CODEFORGE_ORCH_GRAPH_ENABLED")
	setInt(&cfg.Orchestrator.GraphMaxHops, "CODEFORGE_ORCH_GRAPH_MAX_HOPS")
	setInt(&cfg.Orchestrator.GraphTopK, "CODEFORGE_ORCH_GRAPH_TOP_K")
	setFloat64(&cfg.Orchestrator.GraphHopDecay, "CODEFORGE_ORCH_GRAPH_HOP_DECAY")

	// Context re-ranking
	setBool(&cfg.Orchestrator.ContextRerankEnabled, "CODEFORGE_CONTEXT_RERANK_ENABLED")
	setString(&cfg.Orchestrator.ContextRerankModel, "CODEFORGE_CONTEXT_RERANK_MODEL")

	// Webhook
	setString(&cfg.Webhook.GitHubSecret, "CODEFORGE_WEBHOOK_GITHUB_SECRET")
	setString(&cfg.Webhook.GitLabToken, "CODEFORGE_WEBHOOK_GITLAB_TOKEN")
	setString(&cfg.Webhook.PlaneSecret, "CODEFORGE_WEBHOOK_PLANE_SECRET")

	// Notification
	setString(&cfg.Notification.SlackWebhookURL, "CODEFORGE_NOTIFICATION_SLACK_WEBHOOK_URL")
	setString(&cfg.Notification.DiscordWebhookURL, "CODEFORGE_NOTIFICATION_DISCORD_WEBHOOK_URL")

	// OpenTelemetry
	setBool(&cfg.OTEL.Enabled, "CODEFORGE_OTEL_ENABLED")
	setString(&cfg.OTEL.Endpoint, "CODEFORGE_OTEL_ENDPOINT")
	setString(&cfg.OTEL.ServiceName, "CODEFORGE_OTEL_SERVICE_NAME")
	setBool(&cfg.OTEL.Insecure, "CODEFORGE_OTEL_INSECURE")
	setFloat64(&cfg.OTEL.SampleRate, "CODEFORGE_OTEL_SAMPLE_RATE")

	// A2A
	setBool(&cfg.A2A.Enabled, "CODEFORGE_A2A_ENABLED")
	setString(&cfg.A2A.BaseURL, "CODEFORGE_A2A_BASE_URL")
	setStringSlice(&cfg.A2A.APIKeys, "CODEFORGE_A2A_API_KEYS")
	setString(&cfg.A2A.Transport, "CODEFORGE_A2A_TRANSPORT")
	setInt(&cfg.A2A.MaxTasks, "CODEFORGE_A2A_MAX_TASKS")
	setBool(&cfg.A2A.AllowOpen, "CODEFORGE_A2A_ALLOW_OPEN")
	setBool(&cfg.A2A.Streaming, "CODEFORGE_A2A_STREAMING")

	// AG-UI
	setBool(&cfg.AGUI.Enabled, "CODEFORGE_AGUI_ENABLED")

	// MCP
	setBool(&cfg.MCP.Enabled, "CODEFORGE_MCP_ENABLED")
	setString(&cfg.MCP.ServersDir, "CODEFORGE_MCP_SERVERS_DIR")
	setInt(&cfg.MCP.ServerPort, "CODEFORGE_MCP_SERVER_PORT")

	// Agent
	setString(&cfg.Agent.DefaultModel, "CODEFORGE_AGENT_DEFAULT_MODEL")
	setInt(&cfg.Agent.MaxContextTokens, "CODEFORGE_AGENT_MAX_CONTEXT_TOKENS")
	setInt(&cfg.Agent.MaxLoopIterations, "CODEFORGE_AGENT_MAX_LOOP_ITERATIONS")
	setBool(&cfg.Agent.AgenticByDefault, "CODEFORGE_AGENT_AGENTIC_BY_DEFAULT")
	setInt(&cfg.Agent.ToolOutputMaxChars, "CODEFORGE_AGENT_TOOL_OUTPUT_MAX_CHARS")
	setBool(&cfg.Agent.ContextEnabled, "CODEFORGE_AGENT_CONTEXT_ENABLED")
	setInt(&cfg.Agent.ContextBudget, "CODEFORGE_AGENT_CONTEXT_BUDGET")
	setInt(&cfg.Agent.ContextPromptReserve, "CODEFORGE_AGENT_CONTEXT_PROMPT_RESERVE")
	setInt(&cfg.Agent.ConversationRolloutCount, "CODEFORGE_AGENT_CONVERSATION_ROLLOUT_COUNT")
	setInt(&cfg.Agent.SummarizeThreshold, "CODEFORGE_SUMMARIZE_THRESHOLD")

	// Quarantine
	setBool(&cfg.Quarantine.Enabled, "CODEFORGE_QUARANTINE_ENABLED")
	setFloat64(&cfg.Quarantine.QuarantineThreshold, "CODEFORGE_QUARANTINE_THRESHOLD")
	setFloat64(&cfg.Quarantine.BlockThreshold, "CODEFORGE_QUARANTINE_BLOCK_THRESHOLD")
	setString(&cfg.Quarantine.MinTrustBypass, "CODEFORGE_QUARANTINE_MIN_TRUST_BYPASS")
	setInt(&cfg.Quarantine.ExpiryHours, "CODEFORGE_QUARANTINE_EXPIRY_HOURS")

	// LSP
	setBool(&cfg.LSP.Enabled, "CODEFORGE_LSP_ENABLED")

	// Auth
	setBool(&cfg.Auth.Enabled, "CODEFORGE_AUTH_ENABLED")
	setString(&cfg.Auth.JWTSecret, "CODEFORGE_AUTH_JWT_SECRET")
	setDuration(&cfg.Auth.AccessTokenExpiry, "CODEFORGE_AUTH_ACCESS_EXPIRY")
	setDuration(&cfg.Auth.RefreshTokenExpiry, "CODEFORGE_AUTH_REFRESH_EXPIRY")
	setInt(&cfg.Auth.BcryptCost, "CODEFORGE_AUTH_BCRYPT_COST")
	setString(&cfg.Auth.DefaultAdminEmail, "CODEFORGE_AUTH_ADMIN_EMAIL")
	setString(&cfg.Auth.DefaultAdminPass, "CODEFORGE_AUTH_ADMIN_PASS")
	setBool(&cfg.Auth.AutoGenerateInitialPassword, "CODEFORGE_AUTH_AUTO_GENERATE_PASSWORD")
	setString(&cfg.Auth.InitialPasswordFile, "CODEFORGE_AUTH_INITIAL_PASSWORD_FILE")
	setInt(&cfg.Auth.SetupTimeoutMinutes, "CODEFORGE_AUTH_SETUP_TIMEOUT_MINUTES")

	// LiteLLM health polling
	setDuration(&cfg.LiteLLM.HealthPollInterval, "CODEFORGE_LITELLM_HEALTH_POLL_INTERVAL")

	// Copilot
	setBool(&cfg.Copilot.Enabled, "CODEFORGE_COPILOT_ENABLED")
	setString(&cfg.Copilot.HostsFilePath, "CODEFORGE_COPILOT_HOSTS_FILE")

	// GitHub OAuth
	setString(&cfg.GitHub.ClientID, "GITHUB_CLIENT_ID")
	setString(&cfg.GitHub.ClientSecret, "GITHUB_CLIENT_SECRET")
	setString(&cfg.GitHub.CallbackURL, "GITHUB_CALLBACK_URL")

	// Routing
	setBool(&cfg.Routing.Enabled, "CODEFORGE_ROUTING_ENABLED")

	// Experience Pool
	setBool(&cfg.Experience.Enabled, "CODEFORGE_EXPERIENCE_ENABLED")
	setFloat64(&cfg.Experience.ConfidenceThreshold, "CODEFORGE_EXPERIENCE_CONFIDENCE_THRESHOLD")
	setInt(&cfg.Experience.MaxEntries, "CODEFORGE_EXPERIENCE_MAX_ENTRIES")

	// Email / SMTP (for feedback providers)
	setString(&cfg.Notification.SMTPHost, "CODEFORGE_SMTP_HOST")
	setInt(&cfg.Notification.SMTPPort, "CODEFORGE_SMTP_PORT")
	setString(&cfg.Notification.SMTPFrom, "CODEFORGE_SMTP_FROM")
	setString(&cfg.Notification.SMTPPassword, "CODEFORGE_SMTP_PASSWORD")

	// Benchmark
	setDuration(&cfg.Benchmark.WatchdogTimeout, "CODEFORGE_BENCHMARK_WATCHDOG_TIMEOUT")

	// Ollama
	setString(&cfg.Ollama.BaseURL, "OLLAMA_BASE_URL")

	// Plane
	setString(&cfg.Plane.APIToken, "CODEFORGE_PLANE_API_TOKEN")

	// Retention
	setDuration(&cfg.Retention.Sessions, "CODEFORGE_RETENTION_SESSIONS")
	setDuration(&cfg.Retention.Conversations, "CODEFORGE_RETENTION_CONVERSATIONS")
	setDuration(&cfg.Retention.CostRecords, "CODEFORGE_RETENTION_COST_RECORDS")
	setDuration(&cfg.Retention.AuditEntries, "CODEFORGE_RETENTION_AUDIT_ENTRIES")

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

	// Auth validation: reject default JWT secret in production.
	if cfg.Auth.JWTSecret == "codeforge-dev-jwt-secret-change-in-production" && cfg.AppEnv == "production" {
		return errors.New("FATAL: default JWT secret detected in production -- set CODEFORGE_AUTH_JWT_SECRET environment variable")
	}

	// Auth validation: enforce minimum bcrypt cost for security.
	if cfg.Auth.BcryptCost < 10 {
		return errors.New("auth.bcrypt_cost must be >= 10")
	}

	// Auth validation: warn about default admin password in production.
	if cfg.Auth.Enabled {
		p := cfg.Auth.DefaultAdminPass
		if p == "changeme123" || p == "Changeme123" || p == "CHANGE_ME_ON_FIRST_BOOT" {
			slog.Warn("auth.default_admin_pass is set to a well-known default; change it before production use")
		}
	}

	return nil
}

func setString(dst *string, key string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

func setInt(dst *int, key string) {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			slog.Warn("ignoring invalid config value", "key", key, "value", v, "error", err)
			return
		}
		*dst = n
	}
}

func setInt32(dst *int32, key string) {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			slog.Warn("ignoring invalid config value", "key", key, "value", v, "error", err)
			return
		}
		*dst = int32(n)
	}
}

func setFloat64(dst *float64, key string) {
	if v := os.Getenv(key); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			slog.Warn("ignoring invalid config value", "key", key, "value", v, "error", err)
			return
		}
		*dst = f
	}
}

func setInt64(dst *int64, key string) {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			slog.Warn("ignoring invalid config value", "key", key, "value", v, "error", err)
			return
		}
		*dst = n
	}
}

func setBool(dst *bool, key string) {
	if v := os.Getenv(key); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			slog.Warn("ignoring invalid config value", "key", key, "value", v, "error", err)
			return
		}
		*dst = b
	}
}

func setDuration(dst *time.Duration, key string) {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			slog.Warn("ignoring invalid config value", "key", key, "value", v, "error", err)
			return
		}
		*dst = d
	}
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

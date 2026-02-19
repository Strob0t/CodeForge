package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultConfigFile is the path checked for YAML configuration.
const DefaultConfigFile = "codeforge.yaml"

// Load returns a Config using the hierarchy: defaults < YAML < ENV.
// YAML file is optional; missing file is not an error.
func Load() (*Config, error) {
	return LoadFrom(DefaultConfigFile)
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
	setString(&cfg.Server.Port, "CODEFORGE_PORT")
	setString(&cfg.Server.CORSOrigin, "CODEFORGE_CORS_ORIGIN")
	setString(&cfg.Postgres.DSN, "DATABASE_URL")
	setInt32(&cfg.Postgres.MaxConns, "CODEFORGE_PG_MAX_CONNS")
	setInt32(&cfg.Postgres.MinConns, "CODEFORGE_PG_MIN_CONNS")
	setDuration(&cfg.Postgres.MaxConnLifetime, "CODEFORGE_PG_MAX_CONN_LIFETIME")
	setDuration(&cfg.Postgres.MaxConnIdleTime, "CODEFORGE_PG_MAX_CONN_IDLE_TIME")
	setDuration(&cfg.Postgres.HealthCheck, "CODEFORGE_PG_HEALTH_CHECK")
	setString(&cfg.NATS.URL, "NATS_URL")
	setString(&cfg.LiteLLM.URL, "LITELLM_URL")
	setString(&cfg.LiteLLM.MasterKey, "LITELLM_MASTER_KEY")
	setString(&cfg.Logging.Level, "CODEFORGE_LOG_LEVEL")
	setString(&cfg.Logging.Service, "CODEFORGE_LOG_SERVICE")
	setBool(&cfg.Logging.Async, "CODEFORGE_LOG_ASYNC")
	setInt(&cfg.Breaker.MaxFailures, "CODEFORGE_BREAKER_MAX_FAILURES")
	setDuration(&cfg.Breaker.Timeout, "CODEFORGE_BREAKER_TIMEOUT")
	setFloat64(&cfg.Rate.RequestsPerSecond, "CODEFORGE_RATE_RPS")
	setInt(&cfg.Rate.Burst, "CODEFORGE_RATE_BURST")
	setDuration(&cfg.Rate.CleanupInterval, "CODEFORGE_RATE_CLEANUP_INTERVAL")
	setDuration(&cfg.Rate.MaxIdleTime, "CODEFORGE_RATE_MAX_IDLE_TIME")
	setInt(&cfg.Git.MaxConcurrent, "CODEFORGE_GIT_MAX_CONCURRENT")
	setString(&cfg.Policy.DefaultProfile, "CODEFORGE_POLICY_DEFAULT")
	setString(&cfg.Policy.CustomDir, "CODEFORGE_POLICY_DIR")
	setInt(&cfg.Runtime.StallThreshold, "CODEFORGE_STALL_THRESHOLD")
	setDuration(&cfg.Runtime.QualityGateTimeout, "CODEFORGE_QG_TIMEOUT")
	setString(&cfg.Runtime.DefaultDeliverMode, "CODEFORGE_DELIVER_MODE")
	setString(&cfg.Runtime.DefaultTestCommand, "CODEFORGE_TEST_COMMAND")
	setString(&cfg.Runtime.DefaultLintCommand, "CODEFORGE_LINT_COMMAND")
	setString(&cfg.Runtime.DeliveryCommitPrefix, "CODEFORGE_COMMIT_PREFIX")
	setDuration(&cfg.Runtime.HeartbeatInterval, "CODEFORGE_HEARTBEAT_INTERVAL")
	setDuration(&cfg.Runtime.HeartbeatTimeout, "CODEFORGE_HEARTBEAT_TIMEOUT")

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

	// GraphRAG
	setBool(&cfg.Orchestrator.GraphEnabled, "CODEFORGE_ORCH_GRAPH_ENABLED")
	setInt(&cfg.Orchestrator.GraphMaxHops, "CODEFORGE_ORCH_GRAPH_MAX_HOPS")
	setInt(&cfg.Orchestrator.GraphTopK, "CODEFORGE_ORCH_GRAPH_TOP_K")
	setFloat64(&cfg.Orchestrator.GraphHopDecay, "CODEFORGE_ORCH_GRAPH_HOP_DECAY")

	// Webhook
	setString(&cfg.Webhook.GitHubSecret, "CODEFORGE_WEBHOOK_GITHUB_SECRET")
	setString(&cfg.Webhook.GitLabToken, "CODEFORGE_WEBHOOK_GITLAB_TOKEN")
}

// validate checks that required fields are set.
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
	return nil
}

func setString(dst *string, key string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

func setInt(dst *int, key string) {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			*dst = n
		}
	}
}

func setInt32(dst *int32, key string) {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil {
			*dst = int32(n)
		}
	}
}

func setFloat64(dst *float64, key string) {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			*dst = f
		}
	}
}

func setInt64(dst *int64, key string) {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			*dst = n
		}
	}
}

func setBool(dst *bool, key string) {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			*dst = b
		}
	}
}

func setDuration(dst *time.Duration, key string) {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			*dst = d
		}
	}
}

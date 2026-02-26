// Package config provides hierarchical configuration loading for CodeForge.
// Precedence: defaults < YAML file < environment variables < CLI flags.
package config

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ConfigHolder provides thread-safe access to a Config with hot-reload support.
// Services that hold pointers into the Config (e.g., &cfg.Runtime) will see
// updated values after a reload because fields are swapped in-place.
type ConfigHolder struct {
	mu       sync.RWMutex
	cfg      Config
	yamlPath string
}

// NewHolder creates a ConfigHolder from an initial Config and the YAML path
// used for reloading.
func NewHolder(cfg *Config, yamlPath string) *ConfigHolder {
	return &ConfigHolder{cfg: *cfg, yamlPath: yamlPath}
}

// Get returns a pointer to the Config. Callers must not store the pointer
// long-term; read values immediately and release.
func (h *ConfigHolder) Get() *Config {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return &h.cfg
}

// Reload re-reads the YAML file and environment variables, validates, and
// swaps the config in-place. If validation fails, the old config is preserved.
// Fields that cannot be hot-reloaded (Server.Port, Postgres.DSN, NATS.URL) are
// logged as warnings if they differ.
func (h *ConfigHolder) Reload() error {
	newCfg, err := LoadFrom(h.yamlPath)
	if err != nil {
		return fmt.Errorf("reload config: %w", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Warn about non-hot-reloadable fields.
	if newCfg.Server.Port != h.cfg.Server.Port {
		slog.Warn("config reload: server.port changed but requires restart",
			"old", h.cfg.Server.Port, "new", newCfg.Server.Port)
	}
	if newCfg.Postgres.DSN != h.cfg.Postgres.DSN {
		slog.Warn("config reload: postgres.dsn changed but requires restart",
			"old", "***", "new", "***")
	}
	if newCfg.NATS.URL != h.cfg.NATS.URL {
		slog.Warn("config reload: nats.url changed but requires restart",
			"old", h.cfg.NATS.URL, "new", newCfg.NATS.URL)
	}

	// Log level change notification.
	if newCfg.Logging.Level != h.cfg.Logging.Level {
		slog.Info("config reload: logging level changed",
			"old", h.cfg.Logging.Level, "new", newCfg.Logging.Level)
	}

	h.cfg = *newCfg
	return nil
}

// Config holds all runtime configuration for the CodeForge core service.
type Config struct {
	Server       Server       `yaml:"server"`
	Postgres     Postgres     `yaml:"postgres"`
	NATS         NATS         `yaml:"nats"`
	LiteLLM      LiteLLM      `yaml:"litellm"`
	Logging      Logging      `yaml:"logging"`
	Breaker      Breaker      `yaml:"breaker"`
	Rate         Rate         `yaml:"rate"`
	Git          Git          `yaml:"git"`
	Policy       Policy       `yaml:"policy"`
	Runtime      Runtime      `yaml:"runtime"`
	Orchestrator Orchestrator `yaml:"orchestrator"`
	Cache        Cache        `yaml:"cache"`
	Idempotency  Idempotency  `yaml:"idempotency"`
	Webhook      Webhook      `yaml:"webhook"`
	Notification Notification `yaml:"notification"`
	OTEL         OTEL         `yaml:"otel"`
	A2A          A2A          `yaml:"a2a"`
	AGUI         AGUI         `yaml:"agui"`
	MCP          MCP          `yaml:"mcp"`
	LSP          LSP          `yaml:"lsp"`
	Auth         Auth         `yaml:"auth"`
	Workspace    Workspace    `yaml:"workspace"`
	Agent        Agent        `yaml:"agent"`
	Benchmark    Benchmark    `yaml:"benchmark"`
}

// Benchmark holds benchmark evaluation mode configuration.
type Benchmark struct {
	Enabled        bool   `yaml:"enabled"`         // Enable benchmark endpoints (requires APP_ENV=development)
	DatasetsDir    string `yaml:"datasets_dir"`    // Directory with benchmark dataset YAML files (default: configs/benchmarks)
	TimeoutSeconds int    `yaml:"timeout_seconds"` // Timeout per evaluation task in seconds (default: 300)
	DashboardPort  int    `yaml:"dashboard_port"`  // AgentNeo tracing dashboard port (default: 3100)
}

// Agent holds agentic conversation loop configuration.
type Agent struct {
	BuiltinTools       []string `yaml:"builtin_tools"`         // Built-in tools to enable (default: all)
	DefaultModel       string   `yaml:"default_model"`         // Default LLM model for agentic loops
	MaxContextTokens   int      `yaml:"max_context_tokens"`    // Max tokens for context window (default: 128000)
	MaxLoopIterations  int      `yaml:"max_loop_iterations"`   // Max tool-use loop iterations (default: 50)
	AgenticByDefault   bool     `yaml:"agentic_by_default"`    // Enable agentic mode by default for conversations
	ToolOutputMaxChars int      `yaml:"tool_output_max_chars"` // Max chars for tool output before truncation (default: 10000)
}

// Auth holds authentication and authorization configuration.
type Auth struct {
	Enabled            bool          `yaml:"enabled"`              // Enable auth (default: false)
	JWTSecret          string        `yaml:"jwt_secret" json:"-"`  // HMAC-SHA256 signing key
	AccessTokenExpiry  time.Duration `yaml:"access_token_expiry"`  // Access token lifetime (default: 15m)
	RefreshTokenExpiry time.Duration `yaml:"refresh_token_expiry"` // Refresh token lifetime (default: 168h / 7d)
	BcryptCost         int           `yaml:"bcrypt_cost"`          // Bcrypt work factor (default: 12)
	DefaultAdminEmail  string        `yaml:"default_admin_email"`  // Seed admin email (default: admin@localhost)
	DefaultAdminPass   string        `yaml:"default_admin_pass"`   // Seed admin password (default: changeme123)
}

// Webhook holds VCS/PM webhook verification configuration.
type Webhook struct {
	GitHubSecret string `yaml:"github_secret"` // HMAC-SHA256 secret for GitHub webhooks
	GitLabToken  string `yaml:"gitlab_token"`  // Static token for GitLab webhooks
	PlaneSecret  string `yaml:"plane_secret"`  // HMAC secret for Plane.so webhooks
}

// Notification holds notification provider configuration.
type Notification struct {
	SlackWebhookURL   string   `yaml:"slack_webhook_url"`   // Slack incoming webhook URL
	DiscordWebhookURL string   `yaml:"discord_webhook_url"` // Discord webhook URL
	EnabledEvents     []string `yaml:"enabled_events"`      // Event filter (empty = all events)
}

// Git holds git operation configuration.
type Git struct {
	MaxConcurrent int `yaml:"max_concurrent"` // Max concurrent git CLI operations (default: 5)
}

// Orchestrator holds multi-agent execution plan configuration.
type Orchestrator struct {
	MaxParallel               int           `yaml:"max_parallel"`                // Max concurrent steps (default: 4)
	PingPongMaxRounds         int           `yaml:"ping_pong_max_rounds"`        // Max rounds per step in ping_pong (default: 3)
	ConsensusQuorum           int           `yaml:"consensus_quorum"`            // Required successes; 0 = majority (default: 0)
	Mode                      string        `yaml:"mode"`                        // "manual" | "semi_auto" | "full_auto" (default: "semi_auto")
	DecomposeModel            string        `yaml:"decompose_model"`             // LLM model for decomposition (default: "openai/gpt-4o-mini")
	DecomposeMaxTokens        int           `yaml:"decompose_max_tokens"`        // Max tokens for decomposition response (default: 4096)
	ReviewRouterEnabled       bool          `yaml:"review_router_enabled"`       // Enable confidence-based review routing (default: false)
	ReviewConfidenceThreshold float64       `yaml:"review_confidence_threshold"` // Steps below this confidence get routed to review (default: 0.7)
	ReviewRouterModel         string        `yaml:"review_router_model"`         // LLM model for review evaluation (default: scenario "review")
	DebateRounds              int           `yaml:"debate_rounds"`               // Max rounds for moderator debate (default: 1, max: 3)
	MaxTeamSize               int           `yaml:"max_team_size"`               // Max agents per team (default: 5)
	DefaultContextBudget      int           `yaml:"default_context_budget"`      // Default token budget per task context (default: 4096)
	PromptReserve             int           `yaml:"prompt_reserve"`              // Tokens reserved for prompt+output (default: 1024)
	RepoMapTokenBudget        int           `yaml:"repomap_token_budget"`        // Default token budget for repo map generation (default: 1024)
	DefaultEmbeddingModel     string        `yaml:"default_embedding_model"`     // Embedding model for retrieval (default: "text-embedding-3-small")
	RetrievalTopK             int           `yaml:"retrieval_top_k"`             // Number of retrieval results (default: 20)
	RetrievalBM25Weight       float64       `yaml:"retrieval_bm25_weight"`       // BM25 weight for hybrid search (default: 0.5)
	RetrievalSemanticWeight   float64       `yaml:"retrieval_semantic_weight"`   // Semantic weight for hybrid search (default: 0.5)
	SubAgentEnabled           bool          `yaml:"subagent_enabled"`            // Enable sub-agent retrieval (default: true)
	SubAgentModel             string        `yaml:"subagent_model"`              // LLM for sub-agent query expansion/rerank (default: "openai/gpt-4o-mini")
	SubAgentMaxQueries        int           `yaml:"subagent_max_queries"`        // Max expanded queries (default: 5)
	SubAgentRerank            bool          `yaml:"subagent_rerank"`             // Enable LLM reranking (default: true)
	SubAgentTimeout           time.Duration `yaml:"subagent_timeout"`            // Timeout for sub-agent search (default: 60s)
	GraphEnabled              bool          `yaml:"graph_enabled"`               // Enable GraphRAG (default: false)
	GraphMaxHops              int           `yaml:"graph_max_hops"`              // Max hops for graph traversal (default: 2)
	GraphTopK                 int           `yaml:"graph_top_k"`                 // Top-K results for graph search (default: 10)
	GraphHopDecay             float64       `yaml:"graph_hop_decay"`             // Score decay per hop (default: 0.7)
}

// Runtime holds agent execution engine configuration.
type Runtime struct {
	StallThreshold         int           `yaml:"stall_threshold"`
	StallMaxRetries        int           `yaml:"stall_max_retries"` // Max re-plan attempts on stall (default: 2)
	QualityGateTimeout     time.Duration `yaml:"quality_gate_timeout"`
	DefaultDeliverMode     string        `yaml:"default_deliver_mode"`
	DefaultTestCommand     string        `yaml:"default_test_command"`
	DefaultLintCommand     string        `yaml:"default_lint_command"`
	DeliveryCommitPrefix   string        `yaml:"delivery_commit_prefix"`
	HeartbeatInterval      time.Duration `yaml:"heartbeat_interval"`       // Worker heartbeat send interval (default: 30s)
	HeartbeatTimeout       time.Duration `yaml:"heartbeat_timeout"`        // Max time without heartbeat before kill (default: 120s)
	ApprovalTimeoutSeconds int           `yaml:"approval_timeout_seconds"` // HITL approval timeout in seconds (default: 60)
	Sandbox                SandboxConfig `yaml:"sandbox"`
	Hybrid                 HybridConfig  `yaml:"hybrid"`
}

// HybridConfig holds settings for the hybrid execution mode.
// Hybrid mode mounts the workspace read-write while running commands
// inside a Docker container for isolation.
type HybridConfig struct {
	CommandImage string `yaml:"command_image"` // Docker image for command execution (default: same as sandbox)
	MountMode    string `yaml:"mount_mode"`    // Mount type: "rw" (default) or "ro"
}

// SandboxConfig holds Docker sandbox resource defaults.
type SandboxConfig struct {
	MemoryMB    int    `yaml:"memory_mb"`
	CPUQuota    int    `yaml:"cpu_quota"`
	PidsLimit   int    `yaml:"pids_limit"`
	StorageGB   int    `yaml:"storage_gb"`
	NetworkMode string `yaml:"network_mode"`
	Image       string `yaml:"image"`
}

// Cache holds tiered cache configuration.
type Cache struct {
	L1MaxSizeMB int64         `yaml:"l1_max_size_mb"`
	L2Bucket    string        `yaml:"l2_bucket"`
	L2TTL       time.Duration `yaml:"l2_ttl"`
}

// Policy holds policy engine configuration.
type Policy struct {
	DefaultProfile string `yaml:"default_profile"`
	CustomDir      string `yaml:"custom_dir"`
}

// Workspace holds workspace directory configuration.
type Workspace struct {
	Root        string `yaml:"root"`         // Base directory for cloned repos (default: data/workspaces)
	PipelineDir string `yaml:"pipeline_dir"` // Custom pipeline YAML directory
}

// Server holds HTTP server configuration.
type Server struct {
	Port       string `yaml:"port"`
	CORSOrigin string `yaml:"cors_origin"`
}

// Postgres holds PostgreSQL connection configuration.
type Postgres struct {
	DSN             string        `yaml:"dsn"`
	MaxConns        int32         `yaml:"max_conns"`
	MinConns        int32         `yaml:"min_conns"`
	MaxConnLifetime time.Duration `yaml:"max_conn_lifetime"`
	MaxConnIdleTime time.Duration `yaml:"max_conn_idle_time"`
	HealthCheck     time.Duration `yaml:"health_check"`
}

// NATS holds NATS JetStream configuration.
type NATS struct {
	URL string `yaml:"url"`
}

// LiteLLM holds LiteLLM proxy configuration.
type LiteLLM struct {
	URL               string `yaml:"url"`
	MasterKey         string `yaml:"master_key"`
	ConversationModel string `yaml:"conversation_model"` // Model for chat conversations (default: resolved at init)
}

// Logging holds structured logging configuration.
type Logging struct {
	Level   string `yaml:"level"`
	Service string `yaml:"service"`
	Async   bool   `yaml:"async"`
}

// Breaker holds circuit breaker configuration.
type Breaker struct {
	MaxFailures int           `yaml:"max_failures"`
	Timeout     time.Duration `yaml:"timeout"`
}

// Rate holds rate limiter configuration.
type Rate struct {
	RequestsPerSecond float64       `yaml:"requests_per_second"`
	Burst             int           `yaml:"burst"`
	CleanupInterval   time.Duration `yaml:"cleanup_interval"` // Stale bucket cleanup interval (default: 5m)
	MaxIdleTime       time.Duration `yaml:"max_idle_time"`    // Remove buckets idle longer than this (default: 10m)
}

// Idempotency holds idempotency key middleware configuration.
type Idempotency struct {
	Bucket string        `yaml:"bucket"`
	TTL    time.Duration `yaml:"ttl"`
}

// OTEL holds OpenTelemetry configuration.
type OTEL struct {
	Enabled     bool    `yaml:"enabled"`      // Enable OTEL tracing + metrics (default: false)
	Endpoint    string  `yaml:"endpoint"`     // OTLP gRPC endpoint (default: "localhost:4317")
	ServiceName string  `yaml:"service_name"` // Service name for traces (default: "codeforge-core")
	Insecure    bool    `yaml:"insecure"`     // Use insecure gRPC connection (default: true)
	SampleRate  float64 `yaml:"sample_rate"`  // Trace sampling rate 0.0-1.0 (default: 1.0)
}

// A2A holds Agent-to-Agent protocol configuration.
type A2A struct {
	Enabled bool `yaml:"enabled"` // Enable A2A endpoints (default: false)
}

// AGUI holds AG-UI (Agent-User Interaction) protocol configuration.
type AGUI struct {
	Enabled bool `yaml:"enabled"` // Enable AG-UI event emission (default: false)
}

// MCP holds Model Context Protocol integration configuration.
type MCP struct {
	Enabled    bool   `yaml:"enabled"`     // Enable MCP integration (default: false)
	ServersDir string `yaml:"servers_dir"` // Directory with MCP server YAML definitions
	ServerPort int    `yaml:"server_port"` // Port for the built-in MCP server (default: 3001)
}

// LSP holds Language Server Protocol integration configuration.
type LSP struct {
	Enabled         bool          `yaml:"enabled"`          // Enable LSP integration (default: false)
	StartTimeout    time.Duration `yaml:"start_timeout"`    // Max time to wait for server init (default: 30s)
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"` // Max time for graceful shutdown (default: 10s)
	DiagnosticDelay time.Duration `yaml:"diagnostic_delay"` // Debounce delay for diagnostic broadcasts (default: 500ms)
	MaxDiagnostics  int           `yaml:"max_diagnostics"`  // Max diagnostics to cache per file (default: 100)
	AutoStart       bool          `yaml:"auto_start"`       // Auto-start servers on project setup (default: true)
}

// Defaults returns a Config with sensible default values for local development.
func Defaults() Config {
	return Config{
		Server: Server{
			Port:       "8080",
			CORSOrigin: "http://localhost:3000",
		},
		Postgres: Postgres{
			DSN:             "postgres://codeforge:codeforge_dev@localhost:5432/codeforge?sslmode=disable",
			MaxConns:        15,
			MinConns:        2,
			MaxConnLifetime: time.Hour,
			MaxConnIdleTime: 10 * time.Minute,
			HealthCheck:     time.Minute,
		},
		NATS: NATS{
			URL: "nats://localhost:4222",
		},
		LiteLLM: LiteLLM{
			URL: "http://localhost:4000",
		},
		Logging: Logging{
			Level:   "info",
			Service: "codeforge-core",
			Async:   true,
		},
		Breaker: Breaker{
			MaxFailures: 5,
			Timeout:     30 * time.Second,
		},
		Rate: Rate{
			RequestsPerSecond: 10,
			Burst:             100,
			CleanupInterval:   5 * time.Minute,
			MaxIdleTime:       10 * time.Minute,
		},
		Git: Git{
			MaxConcurrent: 5,
		},
		Policy: Policy{
			DefaultProfile: "headless-safe-sandbox",
		},
		Workspace: Workspace{
			Root: "data/workspaces",
		},
		Runtime: Runtime{
			StallThreshold:         5,
			StallMaxRetries:        2,
			QualityGateTimeout:     60 * time.Second,
			DefaultDeliverMode:     "",
			DefaultTestCommand:     "go test ./...",
			DefaultLintCommand:     "golangci-lint run ./...",
			DeliveryCommitPrefix:   "codeforge:",
			HeartbeatInterval:      30 * time.Second,
			HeartbeatTimeout:       120 * time.Second,
			ApprovalTimeoutSeconds: 60,
			Sandbox: SandboxConfig{
				MemoryMB:    512,
				CPUQuota:    1000,
				PidsLimit:   100,
				StorageGB:   10,
				NetworkMode: "none",
				Image:       "ubuntu:22.04",
			},
			Hybrid: HybridConfig{
				CommandImage: "",
				MountMode:    "rw",
			},
		},
		Cache: Cache{
			L1MaxSizeMB: 100,
			L2Bucket:    "CACHE",
			L2TTL:       10 * time.Minute,
		},
		Idempotency: Idempotency{
			Bucket: "IDEMPOTENCY",
			TTL:    24 * time.Hour,
		},
		Orchestrator: Orchestrator{
			MaxParallel:               4,
			PingPongMaxRounds:         3,
			ConsensusQuorum:           0,
			Mode:                      "semi_auto",
			DecomposeModel:            "openai/gpt-4o-mini",
			DecomposeMaxTokens:        4096,
			ReviewRouterEnabled:       false,
			ReviewConfidenceThreshold: 0.7,
			ReviewRouterModel:         "",
			DebateRounds:              1,
			MaxTeamSize:               5,
			DefaultContextBudget:      4096,
			PromptReserve:             1024,
			RepoMapTokenBudget:        1024,
			DefaultEmbeddingModel:     "text-embedding-3-small",
			RetrievalTopK:             20,
			RetrievalBM25Weight:       0.5,
			RetrievalSemanticWeight:   0.5,
			SubAgentEnabled:           true,
			SubAgentModel:             "openai/gpt-4o-mini",
			SubAgentMaxQueries:        5,
			SubAgentRerank:            true,
			SubAgentTimeout:           60 * time.Second,
			GraphEnabled:              false,
			GraphMaxHops:              2,
			GraphTopK:                 10,
			GraphHopDecay:             0.7,
		},
		Webhook:      Webhook{},
		Notification: Notification{},
		OTEL: OTEL{
			Enabled:     false,
			Endpoint:    "localhost:4317",
			ServiceName: "codeforge-core",
			Insecure:    true,
			SampleRate:  1.0,
		},
		A2A:  A2A{Enabled: false},
		AGUI: AGUI{Enabled: false},
		MCP: MCP{
			Enabled:    false,
			ServersDir: "",
			ServerPort: 3001,
		},
		LSP: LSP{
			Enabled:         false,
			StartTimeout:    30 * time.Second,
			ShutdownTimeout: 10 * time.Second,
			DiagnosticDelay: 500 * time.Millisecond,
			MaxDiagnostics:  100,
			AutoStart:       true,
		},
		Auth: Auth{
			Enabled:            false,
			JWTSecret:          "",
			AccessTokenExpiry:  15 * time.Minute,
			RefreshTokenExpiry: 7 * 24 * time.Hour,
			BcryptCost:         12,
			DefaultAdminEmail:  "admin@localhost",
			DefaultAdminPass:   "Changeme123",
		},
		Agent: Agent{
			DefaultModel:       "",
			MaxContextTokens:   128_000,
			MaxLoopIterations:  50,
			AgenticByDefault:   true,
			ToolOutputMaxChars: 10_000,
		},
		Benchmark: Benchmark{
			Enabled:        false,
			DatasetsDir:    "configs/benchmarks",
			TimeoutSeconds: 300,
			DashboardPort:  3100,
		},
	}
}

// Package config provides hierarchical configuration loading for CodeForge.
// Precedence: defaults < YAML file < environment variables < CLI flags.
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// Config holds all runtime configuration for the CodeForge core service.
type Config struct {
	AppEnv       string       `yaml:"app_env"`               // Application environment: "development" | "production" (default: "")
	InternalKey  string       `yaml:"internal_key" json:"-"` // Shared secret for Python worker <-> Go Core API calls
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
	Knowledge    Knowledge    `yaml:"knowledge"`
	Agent        Agent        `yaml:"agent"`
	Benchmark    Benchmark    `yaml:"benchmark"`
	Copilot      Copilot      `yaml:"copilot"`
	GitHub       GitHub       `yaml:"github"`
	Experience   Experience   `yaml:"experience"`
	Limits       Limits       `yaml:"limits"`
	Quarantine   Quarantine   `yaml:"quarantine"`
	Routing      Routing      `yaml:"routing"`
	Ollama       Ollama       `yaml:"ollama"`
	Plane        Plane        `yaml:"plane"`
	PM           PM           `yaml:"pm"`
	Retention    Retention    `yaml:"retention"`
	EnvFile      string       `yaml:"env_file"` // Path to .env file for OAuth device flow
}

// Retention holds the data retention policy (GDPR Article 5(1)(e),
// docs/data-retention.md) that the retention job applies to all tenants.
// A zero period keeps that category forever; a zero interval disables the job.
type Retention struct {
	Interval           time.Duration `yaml:"interval"`             // How often the retention job runs (default: 24h; 0 disables it)
	Sessions           time.Duration `yaml:"sessions"`             // Max idle age of agent sessions (default: 30 days)
	Conversations      time.Duration `yaml:"conversations"`        // Max idle age of conversations + messages (default: 8760h, 1 year)
	CostRecords        time.Duration `yaml:"cost_records"`         // Max idle age of runs with their cost records (default: 8760h, 1 year)
	AuditEntries       time.Duration `yaml:"audit_entries"`        // Max age of audit log entries (default: 61320h, 7 years)
	AuditIPAddresses   time.Duration `yaml:"audit_ip_addresses"`   // Max age of IP addresses in audit entries (default: 180 days)
	ConsentIPAddresses time.Duration `yaml:"consent_ip_addresses"` // Max age of IP addresses + user agents in consent records (default: 180 days)
	// HandoffClaims is how long a handoff stage's claim is kept once the
	// stage was done (KI-90). A redelivered handoff message finds its claim
	// within this period; the default outlasts the NATS stream's 30-day
	// max age, after which no message of the stage can come back.
	HandoffClaims time.Duration `yaml:"handoff_claims"` // Max age of done handoff claims (default: 720h, 30 days; 0 or at least 720h)
}

// Routing holds intelligent model routing configuration (Phase 29).
type Routing struct {
	Enabled bool `yaml:"enabled"` // Enable intelligent three-layer routing cascade (default: true)
}

// Quarantine holds message quarantine system configuration (Phase 23B).
type Quarantine struct {
	Enabled             bool    `yaml:"enabled"`              // Enable quarantine (default: false)
	QuarantineThreshold float64 `yaml:"quarantine_threshold"` // Risk score threshold for quarantine (default: 0.7)
	BlockThreshold      float64 `yaml:"block_threshold"`      // Risk score threshold for immediate block (default: 0.95)
	MinTrustBypass      string  `yaml:"min_trust_bypass"`     // Minimum trust level to bypass quarantine (default: "verified"; full, verified, partial or untrusted, lower case, anything else fails at startup)
	ExpiryHours         int     `yaml:"expiry_hours"`         // Hours until unreviewed messages expire (default: 72; 1 to 2562047)
}

// Expiry is how long a held message waits for a review before it expires.
func (q *Quarantine) Expiry() time.Duration {
	return time.Duration(q.ExpiryHours) * time.Hour
}

// Limits holds configurable caps and timeouts that were previously hardcoded.
type Limits struct {
	MaxQueryLength     int           `yaml:"max_query_length"`      // Max length for search queries (default: 2000)
	MaxRequestBodySize int64         `yaml:"max_request_body_size"` // Max HTTP request body in bytes (default: 1MB)
	MaxFiles           int           `yaml:"max_files"`             // Max files for workspace scan (default: 50)
	SearchTimeout      time.Duration `yaml:"search_timeout"`        // BM25/semantic search timeout (default: 30s)
	GraphSearchTimeout time.Duration `yaml:"graph_search_timeout"`  // Graph traversal timeout (default: 30s)
	MCPTestTimeout     time.Duration `yaml:"mcp_test_timeout"`      // MCP handshake test timeout (default: 10s)
	MaxInputLen        int           `yaml:"max_input_len"`         // Max input length for LLM sanitization (default: 10000)
	MaxEntries         int           `yaml:"max_entries"`           // Max dir entries for task planner (default: 100)
	MaxFileSize        int           `yaml:"max_file_size"`         // Max single file size for context scan (default: 32KB)
}

// Benchmark holds benchmark evaluation mode configuration.
type Benchmark struct {
	Enabled         bool          `yaml:"enabled"`          // Enable benchmark endpoints (requires APP_ENV=development)
	DatasetsDir     string        `yaml:"datasets_dir"`     // Directory with benchmark dataset YAML files (default: configs/benchmarks)
	TimeoutSeconds  int           `yaml:"timeout_seconds"`  // Timeout per evaluation task in seconds (default: 300)
	DashboardPort   int           `yaml:"dashboard_port"`   // AgentNeo tracing dashboard port (default: 3100)
	WatchdogTimeout time.Duration `yaml:"watchdog_timeout"` // Max time before watchdog kills stale benchmark runs (default: 2h)
}

// Ollama holds Ollama local model provider configuration.
type Ollama struct {
	BaseURL string `yaml:"base_url"` // Ollama API base URL (e.g. http://localhost:11434)
}

// Plane holds Plane.so project management integration configuration.
type Plane struct {
	APIToken string `yaml:"api_token" json:"-"` // Plane.so API token for PM sync
	// BaseURL is the Plane API the token belongs to; the token is never sent
	// to another host (a project's plane_base_url must match it).
	BaseURL string `yaml:"base_url"`
}

// PM holds settings of the PM syncs (roadmap import, manual and webhook
// syncs).
type PM struct {
	// AllowedPrivateHosts are the host names, IP addresses and CIDR prefixes
	// whose private addresses (RFC 1918, ULA, CGNAT) the GitLab PM provider
	// may connect to: its base URL is the host of a project's repo_url or a
	// manual sync's base_url, both chosen by tenants (KI-85 review; default
	// none, so a self-hosted GitLab on a private network must be listed).
	// Loopback needs an explicit entry (localhost, 127.0.0.1, ::1, a loopback
	// CIDR); link-local, metadata, unspecified, multicast and reserved
	// addresses stay refused. Separate from mcp.allowed_private_hosts: each
	// list opens private hosts for its own feature only.
	AllowedPrivateHosts []string `yaml:"allowed_private_hosts"`
}

// maxToolOutputMaxChars bounds agent.tool_output_max_chars. A quality gate
// result carries two outputs of up to this many characters, and a
// character takes at most 6 bytes in JSON (a \u escape): 2 * 6 * 80,000 =
// 960,000 bytes stay below the NATS default max payload of 1 MiB, which
// configs/nats/nats-server.conf keeps. The worker clamps to the same bound
// (MAX_TOOL_OUTPUT_MAX_CHARS in workers/codeforge/constants.py).
const maxToolOutputMaxChars = 80_000

// maxAutoAgentFixAttempts bounds agent.auto_agent_fix_attempts: each attempt
// is a whole agent run of up to autoagent.FeatureTimeoutMinutes.
const maxAutoAgentFixAttempts = 10

// Agent holds agentic conversation loop configuration.
type Agent struct {
	DefaultModel             string         `yaml:"default_model"`              // Default LLM model for agentic loops
	MaxContextTokens         int            `yaml:"max_context_tokens"`         // Max tokens for context window (default: 128000)
	MaxLoopIterations        int            `yaml:"max_loop_iterations"`        // Max tool-use loop iterations (default: 50)
	AgenticByDefault         bool           `yaml:"agentic_by_default"`         // Enable agentic mode by default for conversations
	ToolOutputMaxChars       int            `yaml:"tool_output_max_chars"`      // Max chars for tool output before truncation (default: 10000; 0 = the worker default; at most maxToolOutputMaxChars)
	ContextEnabled           bool           `yaml:"context_enabled"`            // Enable context optimizer for conversations (default: true)
	ContextBudget            int            `yaml:"context_budget"`             // Token budget for conversation context (default: 2048)
	ContextPromptReserve     int            `yaml:"context_prompt_reserve"`     // Tokens reserved for prompt in conversation context (default: 512)
	ConversationRolloutCount int            `yaml:"conversation_rollout_count"` // Multi-rollout count for inference-time scaling (default: 1, max: 8)
	SummarizeThreshold       int            `yaml:"summarize_threshold"`        // Message count threshold for auto-summarization (0 = disabled)
	AutoAgentFixAttempts     int            `yaml:"auto_agent_fix_attempts"`    // Runs the auto-agent gets to fix a feature whose verification failed (KI-152; default: 2, 0 to 10)
	PhaseScaling             map[string]int `yaml:"phase_scaling"`              // Phase-aware context budget scaling (mode_id -> percentage, default: boundary_analyzer=100, contract_reviewer=60, reviewer=50, refactorer=70)
}

// Auth holds authentication and authorization configuration.
type Auth struct {
	Enabled                     bool          `yaml:"enabled"`                            // Enable auth (default: true)
	JWTSecret                   string        `yaml:"jwt_secret" json:"-"`                // HMAC-SHA256 signing key
	AccessTokenExpiry           time.Duration `yaml:"access_token_expiry"`                // Access token lifetime (default: 15m)
	RefreshTokenExpiry          time.Duration `yaml:"refresh_token_expiry"`               // Refresh token lifetime (default: 168h / 7d)
	BcryptCost                  int           `yaml:"bcrypt_cost"`                        // Bcrypt work factor (default: 12)
	DefaultAdminEmail           string        `yaml:"default_admin_email"`                // Seed admin email (default: admin@localhost)
	DefaultAdminPass            string        `yaml:"default_admin_pass"`                 // Seed admin password (default: empty)
	AutoGenerateInitialPassword bool          `yaml:"auto_generate_initial_password"`     // Generate random password to file (GitLab-style)
	InitialPasswordFile         string        `yaml:"initial_password_file"`              // Path for generated password (default: data/initial_admin_password)
	SetupTimeoutMinutes         int           `yaml:"setup_timeout_minutes"`              // Setup wizard timeout in minutes (default: 5)
	SetupTokenFile              string        `yaml:"setup_token_file"`                   // One-time setup token written on a first start without users (KI-119; default: data/setup_token, empty: log only)
	LLMKeyEncryptionSecret      string        `yaml:"llm_key_encryption_secret" json:"-"` // Separate encryption key for LLM user keys (falls back to JWTSecret)

	jwtSecretGenerated bool // JWTSecret was generated at load time, not configured
}

// Webhook configures the inbound VCS and PM webhooks. They are registered
// per project (POST /api/v1/projects/{id}/webhooks) with their own secrets
// (KI-85).
type Webhook struct {
	// DeliveryRetention is how long a webhook remembers a delivery (its body
	// and delivery ID): within it, a provider's redelivery and a replay of a
	// signed delivery under any delivery ID are handled once (default 168h).
	DeliveryRetention time.Duration `yaml:"delivery_retention"`

	// Removed with KI-85 (the global webhook routes they verified are
	// gone). They still load, so older files and environments keep
	// working, and a set one is reported at startup.
	GitHubSecret string `yaml:"github_secret" json:"-"`
	GitLabToken  string `yaml:"gitlab_token" json:"-"`
	PlaneSecret  string `yaml:"plane_secret" json:"-"`
}

// RemovedGlobalSecrets names the removed global webhook secrets that are
// still set.
func (w *Webhook) RemovedGlobalSecrets() []string {
	var set []string
	for _, s := range []struct{ key, value string }{
		{"webhook.github_secret", w.GitHubSecret},
		{"webhook.gitlab_token", w.GitLabToken},
		{"webhook.plane_secret", w.PlaneSecret},
	} {
		if s.value != "" {
			set = append(set, s.key)
		}
	}
	return set
}

// Notification holds notification provider configuration.
type Notification struct {
	SlackWebhookURL   string   `yaml:"slack_webhook_url"`      // Slack incoming webhook URL
	DiscordWebhookURL string   `yaml:"discord_webhook_url"`    // Discord webhook URL
	EnabledEvents     []string `yaml:"enabled_events"`         // Event filter (empty = all events)
	SMTPHost          string   `yaml:"smtp_host"`              // SMTP server host for email feedback
	SMTPPort          int      `yaml:"smtp_port"`              // SMTP server port (default: 587)
	SMTPFrom          string   `yaml:"smtp_from"`              // Sender email address
	SMTPPassword      string   `yaml:"smtp_password" json:"-"` // SMTP authentication password
	// ApprovalRecipients receive an email for each tool call awaiting
	// approval (with smtp_host and web_ui_url set; default: none).
	ApprovalRecipients []string `yaml:"approval_recipients"`
	// WebUIURL is the base URL of the web UI; approval emails and Slack
	// approval messages link to its approval page
	// (<web_ui_url>/approvals/<run>/<call>).
	WebUIURL string `yaml:"web_ui_url"`
	// ApprovalTenants are the tenants (IDs) whose approval requests reach the
	// operator's approval channels, the Slack channel and the approval email
	// recipients (KI-84; default: the default tenant; empty: none).
	ApprovalTenants []string `yaml:"approval_tenants"`
}

// Copilot holds GitHub Copilot token exchange configuration.
type Copilot struct {
	Enabled       bool   `yaml:"enabled"`         // Enable Copilot integration (default: false)
	HostsFilePath string `yaml:"hosts_file_path"` // Path to hosts.json (default: ~/.config/github-copilot/hosts.json)
}

// GitHub holds GitHub OAuth integration configuration. ClientID alone
// enables the device flow; ClientID, ClientSecret and CallbackURL together
// enable the web flow that connects a GitHub account as a VCS account.
type GitHub struct {
	ClientID     string `yaml:"client_id"`              // OAuth app client ID
	ClientSecret string `yaml:"client_secret" json:"-"` // OAuth app client secret (web flow)
	// CallbackURL is the web flow's redirect URI - the only one sent to
	// GitHub: https (http only on loopback), path /api/v1/auth/github/callback
	// on the origin the web UI uses for the API.
	CallbackURL string `yaml:"callback_url"`
	// Token is the operator's GitHub token for the REST API of github.com
	// (KI-117): the github-issues PM provider uses it when an integration
	// has no token of its own, PR delivery when a project's github-api
	// provider has none. It serves only the default tenant (KI-85).
	Token string `yaml:"token" json:"-"`
}

// WebFlowConfigured reports whether the GitHub OAuth web flow is configured.
func (g *GitHub) WebFlowConfigured() bool {
	return g.ClientID != "" && g.ClientSecret != "" && g.CallbackURL != ""
}

// Experience holds experience pool configuration.
type Experience struct {
	Enabled             bool    `yaml:"enabled"`              // Enable experience pool (default: false)
	ConfidenceThreshold float64 `yaml:"confidence_threshold"` // Min similarity for cache hit (default: 0.85)
	MaxEntries          int     `yaml:"max_entries"`          // Max entries per project (default: 1000)
}

// Git holds git operation configuration.
type Git struct {
	MaxConcurrent int `yaml:"max_concurrent"` // Max concurrent git CLI operations (default: 5)
	// OperationTimeout bounds the synchronous clone, setup and pull API calls
	// instead of the default request timeout (default: 30m, KI-213).
	OperationTimeout time.Duration `yaml:"operation_timeout"`
}

// Orchestrator holds multi-agent execution plan configuration.
type Orchestrator struct {
	MaxParallel               int           `yaml:"max_parallel"`                // Max concurrent steps (default: 4)
	PingPongMaxRounds         int           `yaml:"ping_pong_max_rounds"`        // Max rounds per step in ping_pong (default: 3)
	ConsensusQuorum           int           `yaml:"consensus_quorum"`            // Required successes; 0 = majority (default: 0)
	Mode                      string        `yaml:"mode"`                        // "manual" | "semi_auto" | "full_auto" (default: "semi_auto")
	DecomposeModel            string        `yaml:"decompose_model"`             // LLM model for decomposition (empty = auto-discover from LiteLLM)
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
	SubAgentModel             string        `yaml:"subagent_model"`              // LLM for sub-agent query expansion/rerank (empty = auto-discover from LiteLLM)
	SubAgentMaxQueries        int           `yaml:"subagent_max_queries"`        // Max expanded queries (default: 5)
	SubAgentRerank            bool          `yaml:"subagent_rerank"`             // Enable LLM reranking (default: true)
	SubAgentTimeout           time.Duration `yaml:"subagent_timeout"`            // Timeout for sub-agent search (default: 60s)
	GraphEnabled              bool          `yaml:"graph_enabled"`               // Enable GraphRAG (default: false)
	GraphMaxHops              int           `yaml:"graph_max_hops"`              // Max hops for graph traversal (default: 2)
	GraphTopK                 int           `yaml:"graph_top_k"`                 // Top-K results for graph search (default: 10)
	GraphHopDecay             float64       `yaml:"graph_hop_decay"`             // Score decay per hop (default: 0.7)
	ContextRerankEnabled      bool          `yaml:"context_rerank_enabled"`      // Enable LLM re-ranking of context entries (default: false)
	ContextRerankModel        string        `yaml:"context_rerank_model"`        // Model to use for re-ranking (empty = default)
}

// Runtime holds agent execution engine configuration.
type Runtime struct {
	StallThreshold         int           `yaml:"stall_threshold"`
	StallMaxRetries        int           `yaml:"stall_max_retries"` // New runs a plan step gets after stalled runs; 0 = none (default: 2)
	QualityGateTimeout     time.Duration `yaml:"quality_gate_timeout"`
	DefaultDeliverMode     string        `yaml:"default_deliver_mode"`
	DefaultTestCommand     string        `yaml:"default_test_command"` // Gate test command for projects without test_command whose language has no default ("" = none)
	DefaultLintCommand     string        `yaml:"default_lint_command"` // Gate lint command for projects without lint_command whose language has no default ("" = none)
	DeliveryCommitPrefix   string        `yaml:"delivery_commit_prefix"`
	HeartbeatInterval      time.Duration `yaml:"heartbeat_interval"`       // Worker heartbeat send interval (default: 30s)
	HeartbeatTimeout       time.Duration `yaml:"heartbeat_timeout"`        // Max time without heartbeat before kill (default: 120s)
	ApprovalTimeoutSeconds int           `yaml:"approval_timeout_seconds"` // HITL approval timeout in seconds (default: 60)
	StaleCheckInterval     time.Duration `yaml:"stale_check_interval"`     // How often to check for stale work (default: 60s)
	TaskAcceptTimeout      time.Duration `yaml:"task_accept_timeout"`      // How long a dispatched backend task may wait for a worker before it fails (default: 1h, 0 = never)
	Sandbox                SandboxConfig `yaml:"sandbox"`
	Hybrid                 HybridConfig  `yaml:"hybrid"`
}

// defaultApprovalTimeoutSeconds is the HITL approval timeout when none is configured.
const defaultApprovalTimeoutSeconds = 60

// DefaultWorkerHeartbeatInterval is the worker heartbeat interval when
// runtime.heartbeat_interval is not set, and the interval of a worker that
// does not read heartbeat_seconds from its start message.
const DefaultWorkerHeartbeatInterval = 30 * time.Second

// WorkerHeartbeatInterval is how often a worker reports the work it executes
// as alive. It is sent to the worker with every start (heartbeat_seconds).
// A nil config or a value <= 0 means the default.
func (r *Runtime) WorkerHeartbeatInterval() time.Duration {
	if r == nil || r.HeartbeatInterval <= 0 {
		return DefaultWorkerHeartbeatInterval
	}
	return r.HeartbeatInterval
}

// ApprovalTimeout is how long a tool call waits for a HITL decision. It is
// the single source for the Go approval wait and for the worker, which gets
// it with every run start and waits for the policy response at least this
// long. A nil config or a value <= 0 means the default.
func (r *Runtime) ApprovalTimeout() time.Duration {
	if r == nil || r.ApprovalTimeoutSeconds <= 0 {
		return defaultApprovalTimeoutSeconds * time.Second
	}
	return time.Duration(r.ApprovalTimeoutSeconds) * time.Second
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

// Policy holds policy engine configuration.
type Policy struct {
	DefaultProfile string `yaml:"default_profile"`
	CustomDir      string `yaml:"custom_dir"` // Custom profiles, API and Allow-Always writes (default: data/policies; "" = memory only)
}

// Knowledge holds the knowledge-base content configuration (KI-105).
type Knowledge struct {
	// ContentRoot is the operator directory knowledge-base content lives in,
	// one area per tenant (<content_root>/<tenant_id>/); content_path values
	// are stored relative to the tenant's area and resolved inside it (no
	// symlink out of it). The worker indexes below its own root with the same
	// setting (default: data/knowledge).
	ContentRoot string `yaml:"content_root"`
}

// Workspace holds workspace directory configuration.
type Workspace struct {
	Root        string `yaml:"root"`         // Base directory for cloned repos (default: data/workspaces)
	PipelineDir string `yaml:"pipeline_dir"` // Custom pipeline YAML directory
	// AdoptRoots are absolute directories whose subdirectories admins may
	// adopt as workspaces (local_path, POST /projects/{id}/adopt) or clone
	// local repositories from; everyone else only adopts inside their
	// tenant's directory of Root. Default: none.
	AdoptRoots []string `yaml:"adopt_roots"`
	// ToolACLs selects per-tenant tool identities (KI-96, ADR-018): with
	// "required" (the Core image, docker-compose.prod.yml) the Core gives
	// every tenant directory POSIX ACLs for the tenant's tool UID and sends
	// the UID (tool_uid) on every payload that starts tool processes; "off"
	// (the default: development, macOS, WSL2 drvfs) keeps plain directories
	// and sends none. Any other value counts as "required".
	ToolACLs string `yaml:"tool_acls"`
}

// Values of workspace.tool_acls.
const (
	ToolACLsOff      = "off"
	ToolACLsRequired = "required"
)

// ToolACLsRequired reports whether workspace.tool_acls selects per-tenant
// tool identities, and whether the value was one of the known ones (an
// unknown value fails closed: required).
func (w *Workspace) ToolACLsRequired() (required, known bool) {
	switch strings.ToLower(strings.TrimSpace(w.ToolACLs)) {
	case "", ToolACLsOff:
		return false, true
	case ToolACLsRequired:
		return true, true
	default:
		return true, false
	}
}

// Server holds HTTP server configuration.
type Server struct {
	// Host is the address the HTTP server listens on: "" (default) listens
	// on all interfaces (containers), 127.0.0.1 keeps a development Core off
	// the network. An IP address or "localhost".
	Host               string        `yaml:"host"`
	Port               string        `yaml:"port"`
	CORSOrigin         string        `yaml:"cors_origin"`
	ReadHeaderTimeout  time.Duration `yaml:"read_header_timeout"`
	ReadTimeout        time.Duration `yaml:"read_timeout"`
	WriteTimeout       time.Duration `yaml:"write_timeout"`
	IdleTimeout        time.Duration `yaml:"idle_timeout"`
	ForceSecureCookies bool          `yaml:"force_secure_cookies"` // Unconditionally set Secure=true on cookies (default: false)
	TLSCertFile        string        `yaml:"tls_cert_file"`        // Path to TLS certificate file (PEM). Empty = plain HTTP.
	TLSKeyFile         string        `yaml:"tls_key_file"`         // Path to TLS private key file (PEM). Empty = plain HTTP.
	// TrustedProxies lists reverse proxies (IPs or CIDR prefixes) whose X-Forwarded-For /
	// X-Real-IP headers identify the client. Empty = forwarding headers are ignored.
	TrustedProxies []string `yaml:"trusted_proxies"`
}

// ListenAddr is the HTTP server's listen address (host:port).
func (s *Server) ListenAddr() string {
	return net.JoinHostPort(s.Host, s.Port)
}

// TrustedProxyPrefixes parses TrustedProxies; a bare IP becomes a single-address prefix.
func (s *Server) TrustedProxyPrefixes() ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(s.TrustedProxies))
	for _, entry := range s.TrustedProxies {
		if prefix, err := netip.ParsePrefix(entry); err == nil {
			prefixes = append(prefixes, prefix.Masked())
			continue
		}
		addr, err := netip.ParseAddr(entry)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy %q: want an IP or CIDR prefix", entry)
		}
		addr = addr.Unmap()
		prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return prefixes, nil
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
	URL            string `yaml:"url"`
	StreamMaxBytes int64  `yaml:"stream_max_bytes"` // Storage cap of the CODEFORGE JetStream stream (default: 10 GiB)
}

// LiteLLM holds LiteLLM proxy configuration.
type LiteLLM struct {
	URL                string        `yaml:"url"`
	MasterKey          string        `yaml:"master_key" json:"-"`
	ConversationModel  string        `yaml:"conversation_model"`   // Model for chat conversations (default: resolved at init)
	HealthPollInterval time.Duration `yaml:"health_poll_interval"` // Model health poll interval (default: 60s)
	// KeyedProviders names the providers whose API key LiteLLM holds (names
	// only, never a key): their wildcard routes list their models and the
	// default model can be one of them (KI-125). docker-compose.prod.yml
	// derives it from the key variables; a provider whose key variable is set
	// in the Core's own environment counts as well.
	KeyedProviders []string `yaml:"keyed_providers"`
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
	AuthPerSecond     float64       `yaml:"auth_per_second"`  // Stricter rate for auth endpoints (default: 0.167 = 10/min)
	AuthBurst         int           `yaml:"auth_burst"`       // Burst for auth endpoints (default: 5)
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
	Insecure    bool    `yaml:"insecure"`     // Plaintext gRPC instead of TLS, e.g. for the dev Jaeger (default: false)
	SampleRate  float64 `yaml:"sample_rate"`  // Trace sampling rate 0.0-1.0 (default: 1.0)
}

// A2A holds Agent-to-Agent protocol configuration.
type A2A struct {
	Enabled   bool     `yaml:"enabled"`           // Enable A2A endpoints (default: false)
	BaseURL   string   `yaml:"base_url"`          // Public URL for AgentCard (default: auto-detect from Server.Port)
	APIKeys   []string `yaml:"api_keys" json:"-"` // API keys of incoming A2A requests: "<key>" (default tenant) or "<tenant-uuid>:<key>"; none = every A2A request is refused
	Transport string   `yaml:"transport"`         // "jsonrpc" (default) | "rest"
	MaxTasks  int      `yaml:"max_tasks"`         // Max concurrent A2A tasks (default: 100)
	AllowOpen bool     `yaml:"allow_open"`        // Allow unauthenticated AgentCard discovery (default: true)
	Streaming bool     `yaml:"streaming"`         // FIX-109: Advertise streaming capability in AgentCard (default: false)
}

// A2AAPIKey is an A2A API key and the tenant its callers act in.
type A2AAPIKey struct {
	Key      string
	TenantID string
	// ID identifies the key without revealing it (a SHA-256 prefix of the
	// key): the inbound A2A tasks a key creates record it, and the A2A
	// protocol handler shows a caller only its own tasks.
	ID string
}

// a2aKeyID is the stable ID of an A2A API key.
func a2aKeyID(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "key-" + hex.EncodeToString(sum[:8])
}

// tenantIDPattern matches a tenant ID (a UUID, lower case).
var tenantIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// looksLikeUUID reports whether s has the shape of a UUID (36 characters,
// dashes after 8, 13, 18 and 23), whatever its characters.
func looksLikeUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for _, i := range []int{8, 13, 18, 23} {
		if s[i] != '-' {
			return false
		}
	}
	return true
}

// ParsedAPIKeys returns the A2A API keys with their tenants (KI-15). An entry
// "<tenant-uuid>:<key>" maps its key to that tenant (the UUID in any case,
// stored lowercase); any other entry is a key of the default tenant. An empty
// key, a key listed twice (its tenant would be ambiguous) and a tenant shaped
// like a UUID that is none are errors.
func (a *A2A) ParsedAPIKeys() ([]A2AAPIKey, error) {
	keys := make([]A2AAPIKey, 0, len(a.APIKeys))
	seen := make(map[string]bool, len(a.APIKeys))
	for i, entry := range a.APIKeys {
		k := A2AAPIKey{Key: strings.TrimSpace(entry), TenantID: tenantctx.DefaultTenantID}
		if tenant, key, ok := strings.Cut(k.Key, ":"); ok && looksLikeUUID(tenant) {
			tenant = strings.ToLower(tenant)
			if !tenantIDPattern.MatchString(tenant) {
				return nil, fmt.Errorf("entry %d: tenant %q is not a UUID", i+1, tenant)
			}
			k = A2AAPIKey{Key: key, TenantID: tenant}
		}
		if k.Key == "" {
			return nil, fmt.Errorf("entry %d has an empty key", i+1)
		}
		if seen[k.Key] {
			return nil, fmt.Errorf("entry %d repeats a key", i+1)
		}
		seen[k.Key] = true
		k.ID = a2aKeyID(k.Key)
		keys = append(keys, k)
	}
	return keys, nil
}

// AGUI holds AG-UI (Agent-User Interaction) protocol configuration.
type AGUI struct {
	Enabled bool `yaml:"enabled"` // Enable AG-UI event emission (default: false)
}

// MCP holds Model Context Protocol integration configuration.
type MCP struct {
	Enabled    bool   `yaml:"enabled"`          // Enable MCP integration (default: false)
	ServersDir string `yaml:"servers_dir"`      // Directory with MCP server YAML definitions
	ServerPort int    `yaml:"server_port"`      // Port for the built-in MCP server (default: 3001)
	APIKey     string `yaml:"api_key" json:"-"` // API key for MCP server authentication (empty = unauthenticated)
	// AllowedPrivateHosts are the host names, IP addresses and CIDR prefixes
	// whose private addresses (RFC 1918, ULA, CGNAT) sse and streamable_http
	// MCP servers may use (KI-100; default none). Loopback needs an explicit
	// entry (localhost, 127.0.0.1, ::1, a loopback CIDR); link-local, metadata,
	// unspecified, multicast and reserved addresses stay refused. Only the
	// platform operator sets it.
	AllowedPrivateHosts []string `yaml:"allowed_private_hosts"`
	// UseProxy sends sse and streamable_http MCP connections (the core's
	// connection test, the worker's runs) through the proxy of the
	// environment (HTTPS_PROXY, HTTP_PROXY, NO_PROXY; default false). The
	// url's host is still checked before connecting, but the address cannot
	// be pinned, so DNS rebinding is left to the proxy's egress policy.
	UseProxy bool `yaml:"use_proxy"`
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
			Port:              "8080",
			CORSOrigin:        "http://localhost:3000",
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      60 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
		Postgres: Postgres{
			DSN:             "postgres://codeforge:codeforge_dev@localhost:5432/codeforge?sslmode=prefer",
			MaxConns:        50,
			MinConns:        10,
			MaxConnLifetime: 30 * time.Minute,
			MaxConnIdleTime: 5 * time.Minute,
			HealthCheck:     30 * time.Second,
		},
		NATS: NATS{
			URL:            "nats://localhost:4222",
			StreamMaxBytes: 10 << 30,
		},
		LiteLLM: LiteLLM{
			URL:                "http://localhost:4000",
			HealthPollInterval: 60 * time.Second,
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
			AuthPerSecond:     10.0 / 60.0, // 10 req/min
			AuthBurst:         5,
		},
		Git: Git{
			MaxConcurrent:    5,
			OperationTimeout: 30 * time.Minute,
		},
		Policy: Policy{
			DefaultProfile: "headless-safe-sandbox",
			CustomDir:      "data/policies",
		},
		Workspace: Workspace{
			Root: "data/workspaces",
		},
		Knowledge: Knowledge{
			ContentRoot: "data/knowledge",
		},
		Runtime: Runtime{
			StallThreshold:         5,
			StallMaxRetries:        2,
			QualityGateTimeout:     60 * time.Second,
			DefaultDeliverMode:     "",
			DefaultTestCommand:     "",
			DefaultLintCommand:     "",
			DeliveryCommitPrefix:   "codeforge:",
			HeartbeatInterval:      30 * time.Second,
			HeartbeatTimeout:       120 * time.Second,
			ApprovalTimeoutSeconds: defaultApprovalTimeoutSeconds,
			StaleCheckInterval:     60 * time.Second,
			TaskAcceptTimeout:      time.Hour,
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
		Idempotency: Idempotency{
			Bucket: "IDEMPOTENCY",
			TTL:    24 * time.Hour,
		},
		Orchestrator: Orchestrator{
			MaxParallel:               4,
			PingPongMaxRounds:         3,
			ConsensusQuorum:           0,
			Mode:                      "semi_auto",
			DecomposeModel:            "",
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
			SubAgentModel:             "",
			SubAgentMaxQueries:        5,
			SubAgentRerank:            true,
			SubAgentTimeout:           60 * time.Second,
			GraphEnabled:              false,
			GraphMaxHops:              2,
			GraphTopK:                 10,
			GraphHopDecay:             0.7,
		},
		Webhook:      Webhook{DeliveryRetention: 7 * 24 * time.Hour},
		Notification: Notification{SMTPPort: 587, ApprovalTenants: []string{tenantctx.DefaultTenantID}},
		Plane:        Plane{BaseURL: "https://api.plane.so"},
		OTEL: OTEL{
			Enabled:     false,
			Endpoint:    "localhost:4317",
			ServiceName: "codeforge-core",
			Insecure:    false,
			SampleRate:  1.0,
		},
		A2A: A2A{
			Enabled:   false,
			Transport: "jsonrpc",
			MaxTasks:  100,
			AllowOpen: false,
		},
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
			Enabled:             true,
			JWTSecret:           "", // auto-generated on first boot if not set via CODEFORGE_AUTH_JWT_SECRET
			AccessTokenExpiry:   15 * time.Minute,
			RefreshTokenExpiry:  7 * 24 * time.Hour,
			BcryptCost:          12,
			DefaultAdminEmail:   "admin@localhost",
			DefaultAdminPass:    "",
			InitialPasswordFile: "data/initial_admin_password",
			SetupTimeoutMinutes: 5,
			SetupTokenFile:      "data/setup_token",
		},
		Agent: Agent{
			DefaultModel:             "",
			MaxContextTokens:         128_000,
			MaxLoopIterations:        50,
			AgenticByDefault:         true,
			ToolOutputMaxChars:       10_000,
			ContextEnabled:           true,
			ContextBudget:            2048,
			ContextPromptReserve:     512,
			ConversationRolloutCount: 1,
			AutoAgentFixAttempts:     2,
		},
		Benchmark: Benchmark{
			Enabled:         false,
			DatasetsDir:     "configs/benchmarks",
			TimeoutSeconds:  300,
			DashboardPort:   3100,
			WatchdogTimeout: 2 * time.Hour,
		},
		Copilot: Copilot{
			Enabled: false,
		},
		GitHub: GitHub{},
		Experience: Experience{
			Enabled:             false,
			ConfidenceThreshold: 0.85,
			MaxEntries:          1000,
		},
		Quarantine: Quarantine{
			Enabled:             false,
			QuarantineThreshold: 0.7,
			BlockThreshold:      0.95,
			MinTrustBypass:      "verified",
			ExpiryHours:         72,
		},
		Routing: Routing{
			Enabled: true,
		},
		// Whole 365-day years count as calendar years (RetentionService).
		Retention: Retention{
			Interval:           24 * time.Hour,
			Sessions:           30 * 24 * time.Hour,       // 30 days
			Conversations:      365 * 24 * time.Hour,      // 1 year
			CostRecords:        365 * 24 * time.Hour,      // 1 year
			AuditEntries:       7 * 365 * 24 * time.Hour,  // 7 years (SOC 2)
			AuditIPAddresses:   180 * 24 * time.Hour,      // 180 days (CNIL)
			ConsentIPAddresses: 180 * 24 * time.Hour,      // 180 days, like audit IP addresses
			HandoffClaims:      messagequeue.StreamMaxAge, // 30 days: no message of a done stage can come back
		},
		Limits: Limits{
			MaxQueryLength:     2000,
			MaxRequestBodySize: 1 << 20, // 1 MB
			MaxFiles:           50,
			SearchTimeout:      30 * time.Second,
			GraphSearchTimeout: 30 * time.Second,
			MCPTestTimeout:     10 * time.Second,
			MaxInputLen:        10_000,
			MaxEntries:         100,
			MaxFileSize:        32 * 1024, // 32 KB
		},
	}
}

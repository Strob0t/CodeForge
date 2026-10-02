package messagequeue

import "github.com/Strob0t/CodeForge/internal/domain/trust"

// --- MCP payloads (Phase 15A) ---

// MCPServerDefPayload carries an MCP server definition in NATS messages.
type MCPServerDefPayload struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Transport   string            `json:"transport"` // "stdio" or "sse"
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	URL         string            `json:"url,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Enabled     bool              `json:"enabled"`
	// AllowedPrivateHosts is mcp.allowed_private_hosts for sse and
	// streamable_http servers: the worker refuses other private addresses (KI-100).
	AllowedPrivateHosts []string `json:"allowed_private_hosts,omitempty"`
	// Trusted marks an operator server (servers_dir): the worker lets it use
	// private and loopback addresses. Servers stored by tenants never are.
	Trusted bool `json:"trusted,omitempty"`
	// UseProxy is mcp.use_proxy for sse and streamable_http servers: the
	// worker connects through the proxy of its environment, unpinned.
	UseProxy bool `json:"use_proxy,omitempty"`
}

// --- Conversation run payloads (Phase 17C) ---

// ConversationToolCallFunction describes the function within a tool call.
type ConversationToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ConversationToolCall represents a single tool call in an assistant message.
type ConversationToolCall struct {
	ID       string                       `json:"id"`
	Type     string                       `json:"type"`
	Function ConversationToolCallFunction `json:"function"`
}

// MessageImagePayload is the NATS representation of an image in a conversation message.
type MessageImagePayload struct {
	Data      string `json:"data"`
	MediaType string `json:"media_type"`
	AltText   string `json:"alt_text,omitempty"`
}

// ConversationMessagePayload represents a chat message in the conversation protocol.
type ConversationMessagePayload struct {
	Role       string                 `json:"role"`
	Content    string                 `json:"content,omitempty"`
	ToolCalls  []ConversationToolCall `json:"tool_calls,omitempty"`
	ToolCallID string                 `json:"tool_call_id,omitempty"`
	Name       string                 `json:"name,omitempty"`
	Images     []MessageImagePayload  `json:"images,omitempty"`
}

// ConversationRunStartPayload is the schema for conversation.run.start messages.
type ConversationRunStartPayload struct {
	RunID              string                       `json:"run_id"`
	ConversationID     string                       `json:"conversation_id"`
	SessionID          string                       `json:"session_id,omitempty"`
	ProjectID          string                       `json:"project_id"`
	Messages           []ConversationMessagePayload `json:"messages"`
	SystemPrompt       string                       `json:"system_prompt"`
	Model              string                       `json:"model"`
	PolicyProfile      string                       `json:"policy_profile"`
	WorkspacePath      string                       `json:"workspace_path"`
	Mode               *ModePayload                 `json:"mode,omitempty"`
	Termination        TerminationPayload           `json:"termination"`
	Context            []ContextEntryPayload        `json:"context,omitempty"`
	MCPServers         []MCPServerDefPayload        `json:"mcp_servers,omitempty"`
	Tools              []string                     `json:"tools,omitempty"`
	MicroagentPrompts  []string                     `json:"microagent_prompts,omitempty"`  // Matched microagent prompts (Phase 22C)
	Trust              *trust.Annotation            `json:"trust,omitempty"`               // Message trust annotation (Phase 23A)
	RoutingEnabled     bool                         `json:"routing_enabled,omitempty"`     // Intelligent routing enabled (Phase 29)
	Agentic            bool                         `json:"agentic"`                       // true = multi-turn tool loop, false = single-turn chat
	ProviderAPIKey     string                       `json:"provider_api_key,omitempty"`    // Per-user provider API key (overrides global)
	TenantID           string                       `json:"tenant_id,omitempty"`           // Tenant isolation for background jobs
	SessionMeta        *SessionMetaPayload          `json:"session_meta,omitempty"`        // Session operation context (Phase B2/B3)
	Reminders          []string                     `json:"reminders,omitempty"`           // Pre-evaluated reminder texts (Phase E)
	PlanActEnabled     bool                         `json:"plan_act_enabled,omitempty"`    // Plan/Act mode toggle (A3)
	RolloutCount       int                          `json:"rollout_count,omitempty"`       // Multi-rollout count for inference-time scaling (Phase 4 A4)
	SummarizeThreshold int                          `json:"summarize_threshold,omitempty"` // Message count threshold for auto-summarization (Phase 3)
	// ToolOutputMaxChars is agent.tool_output_max_chars: the worker truncates
	// longer tool results in the history (0 = the worker's default).
	ToolOutputMaxChars int `json:"tool_output_max_chars,omitempty"`
	// TurnID identifies this run of the conversation (runs reuse the
	// conversation ID as run ID); the worker echoes it on every tool call so
	// that calls of a stopped run are rejected after the next run started.
	TurnID string `json:"turn_id,omitempty"`
	// ToolUID is the tenant's tool UID: the worker runs the turn's tool
	// processes as it (KI-96; 0/omitted with workspace.tool_acls off).
	ToolUID int `json:"tool_uid,omitempty"`
	// ApprovalTimeoutSeconds is how long Go waits for a HITL decision on a
	// tool call of an agentic run; the worker waits for policy responses
	// longer than that (0 = the worker's default).
	ApprovalTimeoutSeconds int `json:"approval_timeout_seconds,omitempty"`
	// HeartbeatSeconds is how often the worker reports the run alive
	// (config runtime.heartbeat_interval; 0 = the worker's default, 30 s).
	HeartbeatSeconds int `json:"heartbeat_seconds,omitempty"`
}

// SessionMetaPayload carries session operation context for resumed/forked/rewound sessions.
type SessionMetaPayload struct {
	ParentSessionID string `json:"parent_session_id,omitempty"`
	ParentRunID     string `json:"parent_run_id,omitempty"`
	ForkEventID     string `json:"fork_event_id,omitempty"`
	RewindEventID   string `json:"rewind_event_id,omitempty"`
	Operation       string `json:"operation,omitempty"` // "resume" | "fork" | "rewind" | ""
}

// ConversationRunCompletePayload is the schema for conversation.run.complete messages.
type ConversationRunCompletePayload struct {
	RunID            string                       `json:"run_id"`
	ConversationID   string                       `json:"conversation_id"`
	SessionID        string                       `json:"session_id,omitempty"`
	AssistantContent string                       `json:"assistant_content"`
	ToolMessages     []ConversationMessagePayload `json:"tool_messages,omitempty"`
	Status           string                       `json:"status"`
	Error            string                       `json:"error,omitempty"`
	CostUSD          float64                      `json:"cost_usd"`
	TokensIn         int64                        `json:"tokens_in"`
	TokensOut        int64                        `json:"tokens_out"`
	StepCount        int                          `json:"step_count"`
	Model            string                       `json:"model"`
	TenantID         string                       `json:"tenant_id,omitempty"`
	// TurnID is the turn of the run start this completion ends; Go ends the
	// conversation's run only when it is the conversation's current turn.
	TurnID string `json:"turn_id,omitempty"`
}

// ConversationCompactCompletePayload is the schema for conversation.compact.complete messages.
// Python publishes this after summarising a conversation's history.
type ConversationCompactCompletePayload struct {
	ConversationID string `json:"conversation_id"`
	TenantID       string `json:"tenant_id"`
	Summary        string `json:"summary"`
	OriginalCount  int    `json:"original_count"`
	Status         string `json:"status"`
}

// WorkspaceTestRequestPayload is the schema for conversation.test.request:
// the auto-agent's post-verification asks the worker to run one pytest file
// of a workspace (KI-81: the Go Core does not execute workspace code).
type WorkspaceTestRequestPayload struct {
	RequestID      string `json:"request_id"`
	TenantID       string `json:"tenant_id"`
	ProjectID      string `json:"project_id"`
	ConversationID string `json:"conversation_id"`
	WorkspacePath  string `json:"workspace_path"`
	// TestFile is a file name matching test_<word>.py in the workspace root.
	TestFile       string `json:"test_file"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	// ToolUID is the tenant's tool UID: the test runs as it (KI-96).
	ToolUID int `json:"tool_uid,omitempty"`
}

// WorkspaceTestResultPayload is the schema for conversation.test.result.
// Passed is the test run's verdict (pytest's exit status); nil when the
// tests could not run or did not finish, with the reason in Error.
type WorkspaceTestResultPayload struct {
	RequestID      string `json:"request_id"`
	TenantID       string `json:"tenant_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	Passed         *bool  `json:"passed,omitempty"`
	Output         string `json:"output"`
	Error          string `json:"error,omitempty"`
}

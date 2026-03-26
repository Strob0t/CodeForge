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

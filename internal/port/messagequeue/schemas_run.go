package messagequeue

import (
	"encoding/json"

	"github.com/Strob0t/CodeForge/internal/domain/trust"
)

// TaskResultPayload is the schema for tasks.result messages.
type TaskResultPayload struct {
	TaskID    string   `json:"task_id"`
	ProjectID string   `json:"project_id"`
	Status    string   `json:"status"`
	Output    string   `json:"output"`
	Files     []string `json:"files"`
	Error     string   `json:"error"`
	TokensIn  int64    `json:"tokens_in"`
	TokensOut int64    `json:"tokens_out"`
	CostUSD   float64  `json:"cost_usd"`
}

// TaskCancelPayload is the schema for tasks.cancel messages.
type TaskCancelPayload struct {
	TaskID   string `json:"task_id"`
	TenantID string `json:"tenant_id,omitempty"`
}

// --- Run protocol payloads (Phase 4B) ---

// ModePayload carries agent mode metadata to the Python worker.
type ModePayload struct {
	ID               string            `json:"id"`
	PromptPrefix     string            `json:"prompt_prefix"`
	Tools            []string          `json:"tools"`
	DeniedTools      []string          `json:"denied_tools,omitempty"`
	DeniedActions    []string          `json:"denied_actions,omitempty"`
	RequiredArtifact string            `json:"required_artifact,omitempty"`
	LLMScenario      string            `json:"llm_scenario,omitempty"`
	OutputSchema     string            `json:"output_schema,omitempty"`
	ModelAdaptations map[string]string `json:"model_adaptations,omitempty"`
}

// RunStartPayload is the schema for runs.start messages.
type RunStartPayload struct {
	RunID             string                `json:"run_id"`
	TaskID            string                `json:"task_id"`
	ProjectID         string                `json:"project_id"`
	AgentID           string                `json:"agent_id"`
	TenantID          string                `json:"tenant_id,omitempty"` // Tenant isolation for background jobs
	Prompt            string                `json:"prompt"`
	PolicyProfile     string                `json:"policy_profile"`
	ExecMode          string                `json:"exec_mode"`
	DeliverMode       string                `json:"deliver_mode,omitempty"`
	Mode              *ModePayload          `json:"mode,omitempty"`
	Config            map[string]string     `json:"config,omitempty"`
	Termination       TerminationPayload    `json:"termination"`
	Context           []ContextEntryPayload `json:"context,omitempty"`            // Pre-packed context entries (Phase 5D)
	MCPServers        []MCPServerDefPayload `json:"mcp_servers,omitempty"`        // MCP server definitions (Phase 15A)
	MicroagentPrompts []string              `json:"microagent_prompts,omitempty"` // Matched microagent prompts (Phase 22C)
	Trust             *trust.Annotation     `json:"trust,omitempty"`              // Message trust annotation (Phase 23A)
}

// TerminationPayload carries the termination limits for a run.
type TerminationPayload struct {
	MaxSteps       int     `json:"max_steps"`
	TimeoutSeconds int     `json:"timeout_seconds"`
	MaxCost        float64 `json:"max_cost"`
}

// ToolCallRequestPayload is the schema for runs.toolcall.request messages.
type ToolCallRequestPayload struct {
	RunID   string            `json:"run_id"`
	CallID  string            `json:"call_id"`
	Tool    string            `json:"tool"`
	Command string            `json:"command"`
	Path    string            `json:"path"`
	Trust   *trust.Annotation `json:"trust,omitempty"` // Message trust annotation (Phase 23A)
}

// ToolCallResponsePayload is the schema for runs.toolcall.response messages.
type ToolCallResponsePayload struct {
	RunID       string `json:"run_id"`
	CallID      string `json:"call_id"`
	Decision    string `json:"decision"`
	Reason      string `json:"reason"`
	ExecMode    string `json:"exec_mode,omitempty"`
	ContainerID string `json:"container_id,omitempty"`
}

// ToolCallResultPayload is the schema for runs.toolcall.result messages.
type ToolCallResultPayload struct {
	RunID     string          `json:"run_id"`
	CallID    string          `json:"call_id"`
	Tool      string          `json:"tool"`
	Success   bool            `json:"success"`
	Output    string          `json:"output"`
	Error     string          `json:"error"`
	CostUSD   float64         `json:"cost_usd"`
	TokensIn  int64           `json:"tokens_in"`
	TokensOut int64           `json:"tokens_out"`
	Model     string          `json:"model,omitempty"`
	Diff      json.RawMessage `json:"diff,omitempty"`
}

// RunCompletePayload is the schema for runs.complete messages.
type RunCompletePayload struct {
	RunID     string  `json:"run_id"`
	TaskID    string  `json:"task_id"`
	ProjectID string  `json:"project_id"`
	Status    string  `json:"status"`
	Output    string  `json:"output"`
	Error     string  `json:"error"`
	CostUSD   float64 `json:"cost_usd"`
	StepCount int     `json:"step_count"`
	TokensIn  int64   `json:"tokens_in"`
	TokensOut int64   `json:"tokens_out"`
	Model     string  `json:"model,omitempty"`
}

// RunOutputPayload is the schema for runs.output messages.
type RunOutputPayload struct {
	RunID    string `json:"run_id"`
	TaskID   string `json:"task_id"`
	TenantID string `json:"tenant_id,omitempty"`
	Line     string `json:"line"`
	Stream   string `json:"stream"`
}

// --- Heartbeat payload (Phase 3C) ---

// RunHeartbeatPayload is the schema for runs.heartbeat messages.
type RunHeartbeatPayload struct {
	RunID     string `json:"run_id"`
	Timestamp string `json:"timestamp"`
}

// --- Quality Gate payloads (Phase 4C) ---

// QualityGateRequestPayload is published to request test/lint execution.
type QualityGateRequestPayload struct {
	RunID         string `json:"run_id"`
	ProjectID     string `json:"project_id"`
	WorkspacePath string `json:"workspace_path"`
	RunTests      bool   `json:"run_tests"`
	RunLint       bool   `json:"run_lint"`
	TestCommand   string `json:"test_command,omitempty"`
	LintCommand   string `json:"lint_command,omitempty"`
}

// QualityGateResultPayload is published with the outcome of a quality gate execution.
type QualityGateResultPayload struct {
	RunID       string `json:"run_id"`
	TestsPassed *bool  `json:"tests_passed,omitempty"`
	LintPassed  *bool  `json:"lint_passed,omitempty"`
	TestOutput  string `json:"test_output,omitempty"`
	LintOutput  string `json:"lint_output,omitempty"`
	Error       string `json:"error,omitempty"`
}

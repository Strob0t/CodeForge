package messagequeue

import (
	"encoding/json"

	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/domain/trust"
)

// TaskAgentPayload is the schema for tasks.agent.{backend} messages: a task
// for an agent backend (Aider, OpenHands, ...) that the worker runs in the
// project workspace.
type TaskAgentPayload struct {
	TaskID        string `json:"task_id"`
	ProjectID     string `json:"project_id"`
	TenantID      string `json:"tenant_id,omitempty"` // owning tenant: Go sets it on requests, the worker echoes it back
	AgentID       string `json:"agent_id,omitempty"`
	Title         string `json:"title"`
	Prompt        string `json:"prompt"`
	Backend       string `json:"backend"`
	WorkspacePath string `json:"workspace_path"`
}

// NewTaskAgentPayload builds the tasks.agent.{backend} payload for t.
func NewTaskAgentPayload(t *task.Task, backend, workspacePath string) TaskAgentPayload {
	return TaskAgentPayload{
		TaskID:        t.ID,
		ProjectID:     t.ProjectID,
		TenantID:      t.TenantID,
		AgentID:       t.AgentID,
		Title:         t.Title,
		Prompt:        t.Prompt,
		Backend:       backend,
		WorkspacePath: workspacePath,
	}
}

// TaskResultPayload is the schema for tasks.result messages.
type TaskResultPayload struct {
	TaskID    string   `json:"task_id"`
	ProjectID string   `json:"project_id"`
	TenantID  string   `json:"tenant_id,omitempty"` // owning tenant: Go sets it on requests, the worker echoes it back
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
	WorkspacePath     string                `json:"workspace_path"`               // project workspace the run's tools work in
	Backend           string                `json:"backend"`                      // the agent's backend (informational: the worker runs its own agent loop)
}

// TerminationPayload carries the termination limits for a run.
type TerminationPayload struct {
	MaxSteps       int     `json:"max_steps"`
	TimeoutSeconds int     `json:"timeout_seconds"`
	MaxCost        float64 `json:"max_cost"`
}

// ToolCallRequestPayload is the schema for runs.toolcall.request messages.
//
// Tool is the worker's tool name (e.g. "bash", "edit_file" or a Claude Code
// tool name); the policy layer maps it to its canonical name. Command is the
// shell command of a bash call and empty otherwise. Path is the file or
// directory argument of a file tool. ModeID is the agent mode the worker was
// started with (conversation runs); runs use the mode stored on the run.
type ToolCallRequestPayload struct {
	RunID    string            `json:"run_id"`
	CallID   string            `json:"call_id"`
	TenantID string            `json:"tenant_id,omitempty"` // owning tenant: Go sets it on requests, the worker echoes it back
	Tool     string            `json:"tool"`
	Command  string            `json:"command"`
	Path     string            `json:"path"`
	ModeID   string            `json:"mode_id,omitempty"`
	TurnID   string            `json:"turn_id,omitempty"` // conversation runs: the turn of conversation.run.start
	Trust    *trust.Annotation `json:"trust,omitempty"`   // Message trust annotation (Phase 23A)
	// ArgumentsPreview is truncated JSON of the tool arguments, shown to a
	// human approver. Display only: the policy never evaluates it.
	ArgumentsPreview string `json:"arguments_preview,omitempty"`
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
	TenantID  string          `json:"tenant_id,omitempty"` // owning tenant: Go sets it on requests, the worker echoes it back
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
	TenantID  string  `json:"tenant_id,omitempty"` // owning tenant: Go sets it on requests, the worker echoes it back
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
	TenantID      string `json:"tenant_id,omitempty"` // owning tenant: Go sets it on requests, the worker echoes it back
	WorkspacePath string `json:"workspace_path"`
	RunTests      bool   `json:"run_tests"`
	RunLint       bool   `json:"run_lint"`
	TestCommand   string `json:"test_command,omitempty"`
	LintCommand   string `json:"lint_command,omitempty"`
}

// QualityGateResultPayload is published with the outcome of a quality gate execution.
type QualityGateResultPayload struct {
	RunID       string `json:"run_id"`
	TenantID    string `json:"tenant_id,omitempty"` // owning tenant: Go sets it on requests, the worker echoes it back
	TestsPassed *bool  `json:"tests_passed,omitempty"`
	LintPassed  *bool  `json:"lint_passed,omitempty"`
	TestOutput  string `json:"test_output,omitempty"`
	LintOutput  string `json:"lint_output,omitempty"`
	Error       string `json:"error,omitempty"`
}

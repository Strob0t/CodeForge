package messagequeue

// TaskCreatedPayload is the schema for tasks.created messages.
type TaskCreatedPayload struct {
	TaskID    string `json:"task_id"`
	ProjectID string `json:"project_id"`
	Title     string `json:"title"`
	Prompt    string `json:"prompt"`
}

// TaskResultPayload is the schema for tasks.result messages.
type TaskResultPayload struct {
	TaskID    string   `json:"task_id"`
	ProjectID string   `json:"project_id"`
	Status    string   `json:"status"`
	Output    string   `json:"output"`
	Files     []string `json:"files"`
	Error     string   `json:"error"`
	TokensIn  int      `json:"tokens_in"`
	TokensOut int      `json:"tokens_out"`
	CostUSD   float64  `json:"cost_usd"`
}

// TaskOutputPayload is the schema for tasks.output messages.
type TaskOutputPayload struct {
	TaskID    string `json:"task_id"`
	ProjectID string `json:"project_id"`
	AgentID   string `json:"agent_id"`
	Line      string `json:"line"`
}

// TaskCancelPayload is the schema for tasks.cancel messages.
type TaskCancelPayload struct {
	TaskID string `json:"task_id"`
}

// AgentStatusPayload is the schema for agents.status messages.
type AgentStatusPayload struct {
	AgentID   string `json:"agent_id"`
	ProjectID string `json:"project_id"`
	Status    string `json:"status"`
}

// --- Run protocol payloads (Phase 4B) ---

// RunStartPayload is the schema for runs.start messages.
type RunStartPayload struct {
	RunID         string                `json:"run_id"`
	TaskID        string                `json:"task_id"`
	ProjectID     string                `json:"project_id"`
	AgentID       string                `json:"agent_id"`
	Prompt        string                `json:"prompt"`
	PolicyProfile string                `json:"policy_profile"`
	ExecMode      string                `json:"exec_mode"`
	DeliverMode   string                `json:"deliver_mode,omitempty"`
	Config        map[string]string     `json:"config"`
	Termination   TerminationPayload    `json:"termination"`
	Context       []ContextEntryPayload `json:"context,omitempty"` // Pre-packed context entries (Phase 5D)
}

// TerminationPayload carries the termination limits for a run.
type TerminationPayload struct {
	MaxSteps       int     `json:"max_steps"`
	TimeoutSeconds int     `json:"timeout_seconds"`
	MaxCost        float64 `json:"max_cost"`
}

// ToolCallRequestPayload is the schema for runs.toolcall.request messages.
type ToolCallRequestPayload struct {
	RunID   string `json:"run_id"`
	CallID  string `json:"call_id"`
	Tool    string `json:"tool"`
	Command string `json:"command"`
	Path    string `json:"path"`
}

// ToolCallResponsePayload is the schema for runs.toolcall.response messages.
type ToolCallResponsePayload struct {
	RunID    string `json:"run_id"`
	CallID   string `json:"call_id"`
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// ToolCallResultPayload is the schema for runs.toolcall.result messages.
type ToolCallResultPayload struct {
	RunID   string  `json:"run_id"`
	CallID  string  `json:"call_id"`
	Tool    string  `json:"tool"`
	Success bool    `json:"success"`
	Output  string  `json:"output"`
	Error   string  `json:"error"`
	CostUSD float64 `json:"cost_usd"`
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
}

// RunOutputPayload is the schema for runs.output messages.
type RunOutputPayload struct {
	RunID  string `json:"run_id"`
	TaskID string `json:"task_id"`
	Line   string `json:"line"`
	Stream string `json:"stream"`
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

// --- Context payloads (Phase 5D) ---

// ContextEntryPayload represents a single context entry in a NATS message.
type ContextEntryPayload struct {
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	Content  string `json:"content"`
	Tokens   int    `json:"tokens"`
	Priority int    `json:"priority"`
}

// ContextPackedPayload notifies the worker that a context pack is available for a run.
type ContextPackedPayload struct {
	RunID     string                `json:"run_id"`
	TaskID    string                `json:"task_id"`
	ProjectID string                `json:"project_id"`
	Entries   []ContextEntryPayload `json:"entries"`
}

// SharedContextUpdatedPayload notifies that a team's shared context has changed.
type SharedContextUpdatedPayload struct {
	TeamID    string `json:"team_id"`
	ProjectID string `json:"project_id,omitempty"`
	Key       string `json:"key"`
	Author    string `json:"author"`
	Version   int    `json:"version"`
}

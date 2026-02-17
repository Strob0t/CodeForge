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
	RunID         string             `json:"run_id"`
	TaskID        string             `json:"task_id"`
	ProjectID     string             `json:"project_id"`
	AgentID       string             `json:"agent_id"`
	Prompt        string             `json:"prompt"`
	PolicyProfile string             `json:"policy_profile"`
	ExecMode      string             `json:"exec_mode"`
	Config        map[string]string  `json:"config"`
	Termination   TerminationPayload `json:"termination"`
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

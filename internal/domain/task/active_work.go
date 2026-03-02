package task

import "time"

// ActiveWorkItem is a read-only projection combining a running/queued task
// with its assigned agent and current run metadata. Used for the active work
// visibility API that prevents redundant parallel task execution.
type ActiveWorkItem struct {
	TaskID     string    `json:"task_id"`
	TaskTitle  string    `json:"task_title"`
	TaskStatus Status    `json:"task_status"`
	ProjectID  string    `json:"project_id"`
	AgentID    string    `json:"agent_id"`
	AgentName  string    `json:"agent_name"`
	AgentMode  string    `json:"agent_mode,omitempty"`
	RunID      string    `json:"run_id,omitempty"`
	StepCount  int       `json:"step_count,omitempty"`
	CostUSD    float64   `json:"cost_usd,omitempty"`
	StartedAt  time.Time `json:"started_at"`
}

// ClaimResult is returned by atomic task claiming. If Claimed is false,
// Reason explains why (e.g. "already claimed by agent X", "not pending").
type ClaimResult struct {
	Task    *Task  `json:"task,omitempty"`
	Claimed bool   `json:"claimed"`
	Reason  string `json:"reason,omitempty"`
}

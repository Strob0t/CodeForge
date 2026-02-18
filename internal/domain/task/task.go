// Package task defines the Task domain entity.
package task

import "time"

// Status represents the current state of a task.
type Status string

const (
	StatusPending   Status = "pending"
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// Task represents a unit of work assigned to an agent.
type Task struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id,omitempty"`
	ProjectID string    `json:"project_id"`
	AgentID   string    `json:"agent_id,omitempty"`
	Title     string    `json:"title"`
	Prompt    string    `json:"prompt"`
	Status    Status    `json:"status"`
	Result    *Result   `json:"result,omitempty"`
	CostUSD   float64   `json:"cost_usd"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Result holds the output of a completed task.
type Result struct {
	Output    string   `json:"output"`
	Files     []string `json:"files,omitempty"`
	Error     string   `json:"error,omitempty"`
	TokensIn  int      `json:"tokens_in"`
	TokensOut int      `json:"tokens_out"`
}

// CreateRequest holds the fields needed to create a new task.
type CreateRequest struct {
	ProjectID string `json:"project_id"`
	Title     string `json:"title"`
	Prompt    string `json:"prompt"`
}

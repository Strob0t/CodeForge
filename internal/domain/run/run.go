// Package run defines the Run domain entity for agent execution attempts.
package run

import "time"

// Status represents the current state of a run.
type Status string

const (
	StatusPending     Status = "pending"
	StatusRunning     Status = "running"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
	StatusCancelled   Status = "cancelled"
	StatusTimeout     Status = "timeout"
	StatusQualityGate Status = "quality_gate" // Quality gate check in progress
)

// ExecMode defines how the agent accesses the project filesystem.
type ExecMode string

const (
	ExecModeMount   ExecMode = "mount"   // Direct host filesystem access
	ExecModeSandbox ExecMode = "sandbox" // Isolated container
	ExecModeHybrid  ExecMode = "hybrid"  // Container with host mount (future)
)

// DeliverMode defines how the output of a successful run is delivered.
type DeliverMode string

const (
	DeliverModeNone        DeliverMode = ""             // No delivery action
	DeliverModePatch       DeliverMode = "patch"        // Generate diff/patch file
	DeliverModeCommitLocal DeliverMode = "commit-local" // Git commit locally (no push)
	DeliverModeBranch      DeliverMode = "branch"       // Push to feature branch
	DeliverModePR          DeliverMode = "pr"           // Create pull request
)

// Run represents a single execution attempt of a task by an agent under a specific policy.
// One task can have multiple runs (retries, different agents, different policies).
type Run struct {
	ID            string      `json:"id"`
	TenantID      string      `json:"tenant_id,omitempty"`
	TaskID        string      `json:"task_id"`
	AgentID       string      `json:"agent_id"`
	ProjectID     string      `json:"project_id"`
	TeamID        string      `json:"team_id,omitempty"`
	PolicyProfile string      `json:"policy_profile"`
	ExecMode      ExecMode    `json:"exec_mode"`
	DeliverMode   DeliverMode `json:"deliver_mode,omitempty"`
	Status        Status      `json:"status"`
	StepCount     int         `json:"step_count"`
	CostUSD       float64     `json:"cost_usd"`
	TokensIn      int64       `json:"tokens_in"`
	TokensOut     int64       `json:"tokens_out"`
	Model         string      `json:"model,omitempty"`
	Output        string      `json:"output,omitempty"`
	Error         string      `json:"error,omitempty"`
	Version       int         `json:"version"`
	StartedAt     time.Time   `json:"started_at"`
	CompletedAt   *time.Time  `json:"completed_at,omitempty"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

// StartRequest holds the fields needed to start a new run.
type StartRequest struct {
	TaskID        string      `json:"task_id"`
	AgentID       string      `json:"agent_id"`
	ProjectID     string      `json:"project_id"`
	TeamID        string      `json:"team_id,omitempty"`
	PolicyProfile string      `json:"policy_profile,omitempty"`
	ExecMode      ExecMode    `json:"exec_mode,omitempty"`
	DeliverMode   DeliverMode `json:"deliver_mode,omitempty"`
}

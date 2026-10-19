// Package run defines the Run domain entity for agent execution attempts.
package run

import (
	"slices"
	"time"
)

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

// TerminalStatuses returns the states a run never leaves: the store refuses
// status updates of a run in one of them.
func TerminalStatuses() []Status {
	return []Status{StatusCompleted, StatusFailed, StatusCancelled, StatusTimeout}
}

// IsTerminal reports whether s is a state the run never leaves.
func (s Status) IsTerminal() bool {
	return slices.Contains(TerminalStatuses(), s)
}

// SourceStatuses returns the statuses a run may be moved to status from: a
// run runs only while pending or running, waits for its quality gate only
// after running, and ends from any active status. Nothing leads back to
// pending or out of a terminal status. The store refuses any other status
// write with domain.ErrConflict.
func SourceStatuses(status Status) []Status {
	switch {
	case status == StatusRunning:
		return []Status{StatusPending, StatusRunning}
	case status == StatusQualityGate:
		return []Status{StatusRunning}
	case status.IsTerminal():
		return []Status{StatusPending, StatusRunning, StatusQualityGate}
	default:
		return nil
	}
}

// CanTransition reports whether a run in status from may be moved to status to.
func CanTransition(from, to Status) bool {
	return slices.Contains(SourceStatuses(to), from)
}

// Usage is the LLM usage a run accumulates: steps (tool calls), cost and tokens.
type Usage struct {
	Steps     int
	CostUSD   float64
	TokensIn  int64
	TokensOut int64
}

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
	ID             string      `json:"id"`
	TenantID       string      `json:"tenant_id,omitempty"`
	TaskID         string      `json:"task_id"`
	AgentID        string      `json:"agent_id"`
	ProjectID      string      `json:"project_id"`
	TeamID         string      `json:"team_id,omitempty"`
	ModeID         string      `json:"mode_id,omitempty"`
	PolicyProfile  string      `json:"policy_profile"`
	ExecMode       ExecMode    `json:"exec_mode"`
	DeliverMode    DeliverMode `json:"deliver_mode,omitempty"`
	Status         Status      `json:"status"`
	StepCount      int         `json:"step_count"`
	CostUSD        float64     `json:"cost_usd"`
	TokensIn       int64       `json:"tokens_in"`
	TokensOut      int64       `json:"tokens_out"`
	Model          string      `json:"model,omitempty"`
	ArtifactType   string      `json:"artifact_type,omitempty"`
	ArtifactValid  *bool       `json:"artifact_valid,omitempty"`
	ArtifactErrors []string    `json:"artifact_errors,omitempty"`
	Output         string      `json:"output,omitempty"`
	Error          string      `json:"error,omitempty"`
	Version        int         `json:"version"`
	StartedAt      time.Time   `json:"started_at"`
	CompletedAt    *time.Time  `json:"completed_at,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// CompletionRequest carries the data needed to finalize a run in the store.
type CompletionRequest struct {
	ID        string
	Status    Status
	Output    string
	Error     string
	CostUSD   float64
	StepCount int
	TokensIn  int64
	TokensOut int64
	Model     string
}

// StartRequest holds the fields needed to start a new run.
type StartRequest struct {
	TaskID        string      `json:"task_id"`
	AgentID       string      `json:"agent_id"`
	ProjectID     string      `json:"project_id"`
	TeamID        string      `json:"team_id,omitempty"`
	ModeID        string      `json:"mode_id,omitempty"`
	PolicyProfile string      `json:"policy_profile,omitempty"`
	ExecMode      ExecMode    `json:"exec_mode,omitempty"`
	DeliverMode   DeliverMode `json:"deliver_mode,omitempty"`
	// PromptNote is appended to the task prompt of this run only: the
	// orchestrator tells a re-planned run why the earlier one stalled
	// (KI-94). Never read from a request body.
	PromptNote string `json:"-"`
}

// Package plan defines the ExecutionPlan domain entity for multi-agent orchestration.
package plan

import "time"

// Protocol defines the scheduling strategy for an execution plan.
type Protocol string

const (
	ProtocolSequential Protocol = "sequential"
	ProtocolParallel   Protocol = "parallel"
	ProtocolPingPong   Protocol = "ping_pong"
	ProtocolConsensus  Protocol = "consensus"
)

// Status represents the lifecycle state of an execution plan.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// StepStatus represents the lifecycle state of an individual step.
type StepStatus string

const (
	StepStatusPending   StepStatus = "pending"
	StepStatusRunning   StepStatus = "running"
	StepStatusCompleted StepStatus = "completed"
	StepStatusFailed    StepStatus = "failed"
	StepStatusSkipped   StepStatus = "skipped"
	StepStatusCancelled StepStatus = "cancelled"
)

// IsTerminal returns true if the step is in a final state.
func (s StepStatus) IsTerminal() bool {
	switch s {
	case StepStatusCompleted, StepStatusFailed, StepStatusSkipped, StepStatusCancelled:
		return true
	}
	return false
}

// ExecutionPlan organizes multiple Runs as a DAG with a scheduling protocol.
type ExecutionPlan struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	TeamID      string    `json:"team_id,omitempty"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Protocol    Protocol  `json:"protocol"`
	Status      Status    `json:"status"`
	MaxParallel int       `json:"max_parallel"`
	Steps       []Step    `json:"steps"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Step represents one unit of work in an execution plan, mapping to a single Run.
type Step struct {
	ID            string     `json:"id"`
	PlanID        string     `json:"plan_id"`
	TaskID        string     `json:"task_id"`
	AgentID       string     `json:"agent_id"`
	PolicyProfile string     `json:"policy_profile"`
	DeliverMode   string     `json:"deliver_mode"`
	DependsOn     []string   `json:"depends_on"`
	Status        StepStatus `json:"status"`
	RunID         string     `json:"run_id,omitempty"`
	Round         int        `json:"round"`
	Error         string     `json:"error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// CreatePlanRequest holds the fields for creating a new execution plan.
type CreatePlanRequest struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	ProjectID   string              `json:"project_id"`
	TeamID      string              `json:"team_id,omitempty"`
	Protocol    Protocol            `json:"protocol"`
	MaxParallel int                 `json:"max_parallel"`
	Steps       []CreateStepRequest `json:"steps"`
}

// CreateStepRequest holds the fields for creating a step within a plan.
type CreateStepRequest struct {
	TaskID        string   `json:"task_id"`
	AgentID       string   `json:"agent_id"`
	PolicyProfile string   `json:"policy_profile,omitempty"`
	DeliverMode   string   `json:"deliver_mode,omitempty"`
	DependsOn     []string `json:"depends_on,omitempty"` // step indices ("0", "1") at creation time
}

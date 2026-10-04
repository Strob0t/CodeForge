// Package plan defines the ExecutionPlan domain entity for multi-agent orchestration.
package plan

import (
	"slices"
	"time"
)

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

// TerminalStatuses returns the states an execution plan never leaves: the
// store refuses status updates of a plan in one of them.
func TerminalStatuses() []Status {
	return []Status{StatusCompleted, StatusFailed, StatusCancelled}
}

// IsTerminal reports whether s is a state the plan never leaves.
func (s Status) IsTerminal() bool {
	return slices.Contains(TerminalStatuses(), s)
}

// StepStatus represents the lifecycle state of an individual step.
type StepStatus string

const (
	StepStatusPending         StepStatus = "pending"
	StepStatusRunning         StepStatus = "running"
	StepStatusCompleted       StepStatus = "completed"
	StepStatusFailed          StepStatus = "failed"
	StepStatusSkipped         StepStatus = "skipped"
	StepStatusCancelled       StepStatus = "cancelled"
	StepStatusWaitingApproval StepStatus = "waiting_approval"
)

// IsTerminal returns true if the step is in a final state.
func (s StepStatus) IsTerminal() bool {
	switch s {
	case StepStatusCompleted, StepStatusFailed, StepStatusSkipped, StepStatusCancelled:
		return true
	}
	return false
}

// ReplanOutcome is what re-planning a step after its run stalled did
// (KI-94). The completion of the stalled run may reach two Go Core
// replicas: only the one that still finds the step running that run acts.
type ReplanOutcome int

const (
	// Replanned: the step ran the stalled run and is pending again.
	Replanned ReplanOutcome = iota + 1
	// ReplanBudgetUsedUp: the step still runs the stalled run, but it used
	// its re-plans; the stalled run fails it.
	ReplanBudgetUsedUp
	// ReplanStepMoved: the step no longer runs the stalled run (another
	// replica re-planned or ended it); nothing is left to do.
	ReplanStepMoved
)

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
	ModeID        string     `json:"mode_id,omitempty"`
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
	ModeID        string   `json:"mode_id,omitempty"`
	DeliverMode   string   `json:"deliver_mode,omitempty"`
	DependsOn     []string `json:"depends_on,omitempty"` // step indices ("0", "1") at creation time
}

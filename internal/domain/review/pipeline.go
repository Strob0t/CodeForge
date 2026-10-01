package review

import (
	"errors"
	"time"
)

// ErrPipelineActive: the project already has a review pipeline that runs or
// whose refactoring waits for a keep or undo decision (S6-F 7).
var ErrPipelineActive = errors.New("a review pipeline is already active on this project " +
	"(running, or its refactoring waits for keep or undo): try again when it has ended")

// ErrAgentInUse: the review pipeline's agent belongs to another plan that has
// not ended.
var ErrAgentInUse = errors.New("the review pipeline's agent belongs to another plan that has not ended")

// PipelineState is where a review pipeline's refactoring stands.
type PipelineState string

const (
	// PipelinePending: the refactoring has not started (or the pipeline does
	// not refactor); no baseline is recorded.
	PipelinePending PipelineState = "pending"
	// PipelineRefactoring: the refactorer step started; BaselineSHA is the
	// workspace at that moment and the change is not decided yet.
	PipelineRefactoring PipelineState = "refactoring"
	// PipelineAwaitingDecision: the change is measured (ResultSHA, Impact)
	// and waits for the user's keep or undo.
	PipelineAwaitingDecision PipelineState = "awaiting_decision"
	// PipelineDone: nothing is left to decide.
	PipelineDone PipelineState = "done"
)

// Pipeline is the Go Core's record of a contract-first review pipeline
// (KI-17): the plan it runs as and the commits its refactoring is measured
// and undone against. The threshold HITL trusts this record, not the
// workspace: the refs there are agent-writable and only keep the commits
// from git's garbage collection.
type Pipeline struct {
	PlanID    string        `json:"plan_id"`
	TenantID  string        `json:"tenant_id"`
	ProjectID string        `json:"project_id"`
	State     PipelineState `json:"state"`
	// BaselineSHA is the workspace when the refactorer step started (S6-F 2);
	// "" until then.
	BaselineSHA string `json:"baseline_sha"`
	// ResultSHA is the workspace when the refactoring was measured; "" until
	// then.
	ResultSHA string `json:"result_sha"`
	// StepID and RunID are the refactoring step and the run that was measured.
	StepID string  `json:"step_id"`
	RunID  string  `json:"run_id"`
	Impact *Impact `json:"impact,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Impact is the measured change of a refactoring.
type Impact struct {
	Level        string `json:"impact_level"`
	FilesChanged int    `json:"files_changed"`
	LinesAdded   int    `json:"lines_added"`
	LinesRemoved int    `json:"lines_removed"`
	CrossLayer   bool   `json:"cross_layer"`
	Structural   bool   `json:"structural"`
	// Reason says why the change needs a decision although it could not be
	// measured (or why a failed refactoring asks), "" otherwise.
	Reason string `json:"reason,omitempty"`
}

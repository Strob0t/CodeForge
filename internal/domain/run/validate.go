package run

import (
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain"
)

// validStatuses enumerates all valid run statuses.
var validStatuses = map[Status]bool{
	StatusPending:     true,
	StatusRunning:     true,
	StatusCompleted:   true,
	StatusFailed:      true,
	StatusCancelled:   true,
	StatusTimeout:     true,
	StatusQualityGate: true,
}

// validDeliverModes enumerates all valid delivery modes.
var validDeliverModes = map[DeliverMode]bool{
	DeliverModeNone:        true,
	DeliverModePatch:       true,
	DeliverModeCommitLocal: true,
	DeliverModeBranch:      true,
	DeliverModePR:          true,
}

// validExecModes enumerates all valid execution modes.
var validExecModes = map[ExecMode]bool{
	ExecModeMount:   true,
	ExecModeSandbox: true,
	ExecModeHybrid:  true,
}

// Validate checks that a Run has all required fields and valid values.
func (r *Run) Validate() error {
	if r.TaskID == "" {
		return fmt.Errorf("task_id is required")
	}
	if r.AgentID == "" {
		return fmt.Errorf("agent_id is required")
	}
	if r.ProjectID == "" {
		return fmt.Errorf("project_id is required")
	}
	if r.Status != "" && !validStatuses[r.Status] {
		return fmt.Errorf("invalid status %q", r.Status)
	}
	if r.ExecMode != "" && !validExecModes[r.ExecMode] {
		return fmt.Errorf("invalid exec_mode %q", r.ExecMode)
	}
	if r.StepCount < 0 {
		return fmt.Errorf("step_count must be non-negative")
	}
	if r.CostUSD < 0 {
		return fmt.Errorf("cost_usd must be non-negative")
	}
	if r.DeliverMode != "" && !validDeliverModes[r.DeliverMode] {
		return fmt.Errorf("invalid deliver_mode %q", r.DeliverMode)
	}
	return nil
}

// Validate checks that a StartRequest has all required fields.
func (r *StartRequest) Validate() error {
	if r.TaskID == "" {
		return fmt.Errorf("task_id is required: %w", domain.ErrValidation)
	}
	if r.AgentID == "" {
		return fmt.Errorf("agent_id is required: %w", domain.ErrValidation)
	}
	if r.ProjectID == "" {
		return fmt.Errorf("project_id is required: %w", domain.ErrValidation)
	}
	if r.ExecMode != "" && !validExecModes[r.ExecMode] {
		return fmt.Errorf("invalid exec_mode %q: %w", r.ExecMode, domain.ErrValidation)
	}
	if r.DeliverMode != "" && !validDeliverModes[r.DeliverMode] {
		return fmt.Errorf("invalid deliver_mode %q: %w", r.DeliverMode, domain.ErrValidation)
	}
	return nil
}

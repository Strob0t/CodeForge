package run

import "fmt"

// validStatuses enumerates all valid run statuses.
var validStatuses = map[Status]bool{
	StatusPending:   true,
	StatusRunning:   true,
	StatusCompleted: true,
	StatusFailed:    true,
	StatusCancelled: true,
	StatusTimeout:   true,
}

// validExecModes enumerates all valid execution modes.
var validExecModes = map[ExecMode]bool{
	ExecModeMount:   true,
	ExecModeSandbox: true,
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
	return nil
}

// Validate checks that a StartRequest has all required fields.
func (r *StartRequest) Validate() error {
	if r.TaskID == "" {
		return fmt.Errorf("task_id is required")
	}
	if r.AgentID == "" {
		return fmt.Errorf("agent_id is required")
	}
	if r.ProjectID == "" {
		return fmt.Errorf("project_id is required")
	}
	if r.ExecMode != "" && !validExecModes[r.ExecMode] {
		return fmt.Errorf("invalid exec_mode %q", r.ExecMode)
	}
	return nil
}

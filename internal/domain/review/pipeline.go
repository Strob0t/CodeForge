package review

import "time"

// Pipeline is the Go Core's record of a contract-first review pipeline
// (KI-17): the plan it runs as and the baseline commit its refactoring is
// measured and undone against. The threshold HITL trusts this record, not
// the workspace: the baseline ref there is agent-writable and only keeps the
// baseline commit from git's garbage collection.
type Pipeline struct {
	PlanID      string    `json:"plan_id"`
	TenantID    string    `json:"tenant_id"`
	ProjectID   string    `json:"project_id"`
	BaselineSHA string    `json:"baseline_sha"` // "" until a baseline is recorded
	CreatedAt   time.Time `json:"created_at"`
}

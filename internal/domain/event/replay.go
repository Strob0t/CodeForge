package event

import "time"

// ReplayRequest holds the parameters for replaying a run's events.
type ReplayRequest struct {
	RunID     string `json:"run_id"`
	FromEvent string `json:"from_event,omitempty"` // Event ID to start from (empty = beginning)
	ToEvent   string `json:"to_event,omitempty"`   // Event ID to stop at (empty = end)
	DryRun    bool   `json:"dry_run"`              // If true, return events without executing
}

// ReplayResult contains the outcome of a replay request.
type ReplayResult struct {
	RunID      string       `json:"run_id"`
	Events     []AgentEvent `json:"events"`
	EventCount int          `json:"event_count"`
	DryRun     bool         `json:"dry_run"`
}

// AuditEntry represents a single entry in the audit trail.
type AuditEntry struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id,omitempty"`
	ProjectID string    `json:"project_id"`
	RunID     string    `json:"run_id,omitempty"`
	AgentID   string    `json:"agent_id,omitempty"`
	Action    string    `json:"action"`
	Details   string    `json:"details,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// AuditFilter controls which audit entries are returned.
type AuditFilter struct {
	ProjectID string     `json:"project_id,omitempty"`
	RunID     string     `json:"run_id,omitempty"`
	AgentID   string     `json:"agent_id,omitempty"`
	Action    string     `json:"action,omitempty"`
	After     *time.Time `json:"after,omitempty"`
	Before    *time.Time `json:"before,omitempty"`
}

// AuditPage is a cursor-paginated page of audit entries.
type AuditPage struct {
	Entries []AuditEntry `json:"entries"`
	Cursor  string       `json:"cursor"`
	HasMore bool         `json:"has_more"`
	Total   int          `json:"total"`
}

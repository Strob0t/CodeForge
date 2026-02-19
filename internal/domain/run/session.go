package run

import "time"

// SessionStatus represents the current state of a session.
type SessionStatus string

const (
	SessionStatusActive    SessionStatus = "active"
	SessionStatusPaused    SessionStatus = "paused"
	SessionStatusCompleted SessionStatus = "completed"
	SessionStatusForked    SessionStatus = "forked"
)

// Session represents a resumable execution context that can span multiple runs.
// Sessions enable resume, fork, and rewind operations.
type Session struct {
	ID              string        `json:"id"`
	TenantID        string        `json:"tenant_id,omitempty"`
	ProjectID       string        `json:"project_id"`
	TaskID          string        `json:"task_id"`
	ParentSessionID string        `json:"parent_session_id,omitempty"`
	ParentRunID     string        `json:"parent_run_id,omitempty"`
	CurrentRunID    string        `json:"current_run_id,omitempty"`
	Status          SessionStatus `json:"status"`
	Metadata        string        `json:"metadata,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

// ResumeRequest holds the fields needed to resume a run.
type ResumeRequest struct {
	RunID    string `json:"run_id"`
	Prompt   string `json:"prompt,omitempty"`
	AgentID  string `json:"agent_id,omitempty"`
	ExecMode string `json:"exec_mode,omitempty"`
}

// ForkRequest holds the fields needed to fork a run into a new session.
type ForkRequest struct {
	RunID       string `json:"run_id"`
	FromEventID string `json:"from_event_id,omitempty"`
	Prompt      string `json:"prompt,omitempty"`
	AgentID     string `json:"agent_id,omitempty"`
}

// RewindRequest holds the fields needed to rewind a run.
type RewindRequest struct {
	RunID     string `json:"run_id"`
	ToEventID string `json:"to_event_id,omitempty"`
}

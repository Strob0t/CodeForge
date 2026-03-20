package a2a

import (
	"fmt"
	"time"
)

// TaskState represents the state of an A2A task (matches A2A spec v0.3.0).
//
// FIX-099: Spelling convention: The A2A protocol and Go stdlib use "canceled"
// (American). All CodeForge-internal code uses "cancelled" (British) for
// consistency. Both spellings are intentional — do not "fix" either.
type TaskState string

const (
	TaskStateSubmitted     TaskState = "submitted"
	TaskStateWorking       TaskState = "working"
	TaskStateCompleted     TaskState = "completed"
	TaskStateFailed        TaskState = "failed"
	TaskStateCanceled      TaskState = "canceled"
	TaskStateRejected      TaskState = "rejected"
	TaskStateInputRequired TaskState = "input-required"
	TaskStateAuthRequired  TaskState = "auth-required"
)

var validStates = map[TaskState]bool{
	TaskStateSubmitted:     true,
	TaskStateWorking:       true,
	TaskStateCompleted:     true,
	TaskStateFailed:        true,
	TaskStateCanceled:      true,
	TaskStateRejected:      true,
	TaskStateInputRequired: true,
	TaskStateAuthRequired:  true,
}

// IsValidState reports whether s is a recognised A2A task state.
func IsValidState(s TaskState) bool { return validStates[s] }

// Direction describes whether a task is inbound (received) or outbound (sent).
type Direction string

const (
	DirectionInbound  Direction = "inbound"
	DirectionOutbound Direction = "outbound"
)

// A2ATask is the domain model for an A2A protocol task.
type A2ATask struct {
	ID            string            `json:"id"`
	ContextID     string            `json:"context_id"`
	State         TaskState         `json:"state"`
	Direction     Direction         `json:"direction"`
	SkillID       string            `json:"skill_id"`
	TrustOrigin   string            `json:"trust_origin"`
	TrustLevel    string            `json:"trust_level"`
	SourceAddr    string            `json:"source_addr"`
	ProjectID     string            `json:"project_id"`
	RemoteAgentID string            `json:"remote_agent_id"`
	TenantID      string            `json:"tenant_id"`
	Metadata      map[string]string `json:"metadata"`
	History       []byte            `json:"history"`
	Artifacts     []byte            `json:"artifacts"`
	ErrorMessage  string            `json:"error_message"`
	Version       int               `json:"version"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// Validate checks required fields on an A2ATask.
func (t *A2ATask) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("a2a task: id is required")
	}
	if !IsValidState(t.State) {
		return fmt.Errorf("a2a task: invalid state %q", t.State)
	}
	if t.Direction != DirectionInbound && t.Direction != DirectionOutbound {
		return fmt.Errorf("a2a task: invalid direction %q", t.Direction)
	}
	return nil
}

// NewA2ATask returns a task with sensible defaults.
func NewA2ATask(id string) *A2ATask {
	now := time.Now().UTC()
	return &A2ATask{
		ID:          id,
		State:       TaskStateSubmitted,
		Direction:   DirectionInbound,
		TrustOrigin: "a2a",
		TrustLevel:  "untrusted",
		Metadata:    make(map[string]string),
		History:     []byte("[]"),
		Artifacts:   []byte("[]"),
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

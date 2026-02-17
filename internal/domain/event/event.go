// Package event defines the AgentEvent domain entity for event sourcing.
package event

import (
	"encoding/json"
	"time"
)

// Type identifies the kind of agent event.
type Type string

const (
	TypeAgentStarted  Type = "agent.started"
	TypeAgentStepDone Type = "agent.step_done"
	TypeToolCalled    Type = "agent.tool_called"
	TypeToolResult    Type = "agent.tool_result"
	TypeAgentFinished Type = "agent.finished"
	TypeAgentError    Type = "agent.error"

	// Run protocol events (Phase 4B)
	TypeRunStarted        Type = "run.started"
	TypeRunCompleted      Type = "run.completed"
	TypeToolCallRequested Type = "run.toolcall.requested"
	TypeToolCallApproved  Type = "run.toolcall.approved"
	TypeToolCallDenied    Type = "run.toolcall.denied"
	TypeToolCallResultEv  Type = "run.toolcall.result"
)

// AgentEvent represents a single immutable event in an agent's execution trajectory.
type AgentEvent struct {
	ID        string          `json:"id"`
	AgentID   string          `json:"agent_id"`
	TaskID    string          `json:"task_id"`
	ProjectID string          `json:"project_id"`
	Type      Type            `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	RequestID string          `json:"request_id,omitempty"`
	Version   int             `json:"version"`
	CreatedAt time.Time       `json:"created_at"`
}

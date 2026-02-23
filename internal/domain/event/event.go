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

	// Phase 4C: quality gate + delivery events
	TypeQualityGateStarted Type = "run.qualitygate.started"
	TypeQualityGatePassed  Type = "run.qualitygate.passed"
	TypeQualityGateFailed  Type = "run.qualitygate.failed"
	TypeDeliveryStarted    Type = "run.delivery.started"
	TypeDeliveryCompleted  Type = "run.delivery.completed"
	TypeDeliveryFailed     Type = "run.delivery.failed"
	TypeStallDetected      Type = "run.stall_detected"
	TypeArtifactValidated  Type = "run.artifact.validated"
	TypeArtifactFailed     Type = "run.artifact.failed"

	// Phase 5A: orchestration plan events
	TypePlanCreated   Type = "plan.created"
	TypePlanStarted   Type = "plan.started"
	TypePlanCompleted Type = "plan.completed"
	TypePlanFailed    Type = "plan.failed"
	TypePlanCancelled Type = "plan.cancelled"

	// Session events (resume/fork/rewind)
	TypeSessionCreated Type = "session.created"
	TypeSessionResumed Type = "session.resumed"
	TypeSessionForked  Type = "session.forked"
	TypeSessionRewound Type = "session.rewound"
	TypeSessionPaused  Type = "session.paused"
	TypeSessionEnded   Type = "session.ended"
)

// AgentEvent represents a single immutable event in an agent's execution trajectory.
type AgentEvent struct {
	ID        string          `json:"id"`
	AgentID   string          `json:"agent_id"`
	TaskID    string          `json:"task_id"`
	ProjectID string          `json:"project_id"`
	RunID     string          `json:"run_id,omitempty"`
	Type      Type            `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	RequestID string          `json:"request_id,omitempty"`
	Version   int             `json:"version"`
	CreatedAt time.Time       `json:"created_at"`
	// Per-tool token tracking (populated for run.toolcall.result events).
	ToolName  string  `json:"tool_name,omitempty"`
	Model     string  `json:"model,omitempty"`
	TokensIn  int64   `json:"tokens_in,omitempty"`
	TokensOut int64   `json:"tokens_out,omitempty"`
	CostUSD   float64 `json:"cost_usd,omitempty"`
}

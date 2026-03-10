// Package ws -- AG-UI (Agent-User Interaction) protocol event types.
// These follow the CopilotKit AG-UI specification for agent <-> frontend streaming.
// When enabled, these events are emitted alongside native CodeForge WS events.
package ws

import "encoding/json"

// AG-UI event type constants.
const (
	AGUIRunStarted   = "agui.run_started"
	AGUIRunFinished  = "agui.run_finished"
	AGUITextMessage  = "agui.text_message"
	AGUIToolCall     = "agui.tool_call"
	AGUIToolResult   = "agui.tool_result"
	AGUIStateDelta   = "agui.state_delta"
	AGUIStepStarted  = "agui.step_started"
	AGUIStepFinished = "agui.step_finished"
)

// AGUIRunStartedEvent signals that an agent run has begun.
type AGUIRunStartedEvent struct {
	RunID     string `json:"run_id"`
	ThreadID  string `json:"thread_id,omitempty"`
	AgentName string `json:"agent_name,omitempty"`
}

// AGUIRunFinishedEvent signals that an agent run has completed.
type AGUIRunFinishedEvent struct {
	RunID  string `json:"run_id"`
	Status string `json:"status"`          // "completed", "failed", "cancelled"
	Error  string `json:"error,omitempty"` // error message when status is "failed"
}

// AGUITextMessageEvent carries a text chunk from the agent.
type AGUITextMessageEvent struct {
	RunID   string `json:"run_id"`
	Role    string `json:"role"` // "assistant"
	Content string `json:"content"`
}

// AGUIToolCallEvent signals a tool invocation by the agent.
type AGUIToolCallEvent struct {
	RunID  string `json:"run_id"`
	CallID string `json:"call_id"`
	Name   string `json:"name"`
	Args   string `json:"args"` // JSON-encoded arguments
}

// AGUIToolResultEvent carries the result of a tool invocation.
type AGUIToolResultEvent struct {
	RunID  string          `json:"run_id"`
	CallID string          `json:"call_id"`
	Result string          `json:"result"` // JSON-encoded result
	Error  string          `json:"error,omitempty"`
	Diff   json.RawMessage `json:"diff,omitempty"`
}

// AGUIStateDeltaEvent carries a partial state update.
type AGUIStateDeltaEvent struct {
	RunID string `json:"run_id"`
	Delta string `json:"delta"` // JSON Patch (RFC 6902) or JSON Merge Patch (RFC 7396)
}

// AGUIStepStartedEvent signals the start of a named step within a run.
type AGUIStepStartedEvent struct {
	RunID  string `json:"run_id"`
	StepID string `json:"step_id"`
	Name   string `json:"name"`
}

// AGUIStepFinishedEvent signals the completion of a named step.
type AGUIStepFinishedEvent struct {
	RunID  string `json:"run_id"`
	StepID string `json:"step_id"`
	Status string `json:"status"` // "completed", "failed"
}

// AGUIPermissionRequest event type constant.
const AGUIPermissionRequest = "agui.permission_request"

// AGUIGoalProposal event type constant.
const AGUIGoalProposal = "agui.goal_proposal"

// AGUIPermissionRequestEvent signals that a tool call requires user approval.
type AGUIPermissionRequestEvent struct {
	RunID   string `json:"run_id"`
	CallID  string `json:"call_id"`
	Tool    string `json:"tool"`
	Command string `json:"command,omitempty"`
	Path    string `json:"path,omitempty"`
}

// AGUIGoalProposalEvent is sent when the agent proposes a goal for user review.
type AGUIGoalProposalEvent struct {
	RunID      string `json:"run_id"`
	ProposalID string `json:"proposal_id"`
	Action     string `json:"action"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	Priority   int    `json:"priority"`
	GoalID     string `json:"goal_id,omitempty"`
}

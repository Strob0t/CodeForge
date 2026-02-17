// Package messagequeue defines the message queue port (interface).
package messagequeue

import "context"

// Handler processes a message received from the queue.
// The context carries request-scoped values such as the request ID.
type Handler func(ctx context.Context, subject string, data []byte) error

// Queue is the port interface for publishing and subscribing to messages.
type Queue interface {
	// Publish sends a message to the given subject.
	Publish(ctx context.Context, subject string, data []byte) error

	// Subscribe registers a handler for messages on the given subject.
	// The returned function cancels the subscription.
	Subscribe(ctx context.Context, subject string, handler Handler) (cancel func(), err error)

	// Drain gracefully drains all subscriptions before closing.
	// Pending messages are processed; no new messages are accepted.
	Drain() error

	// Close shuts down the queue connection immediately.
	Close() error

	// IsConnected reports whether the queue is currently connected.
	IsConnected() bool
}

// Subject constants for NATS subjects used by CodeForge.
const (
	SubjectTaskCreated = "tasks.created"
	SubjectTaskAgent   = "tasks.agent"  // tasks.agent.{backend} — dispatched to specific backend
	SubjectTaskResult  = "tasks.result" // results from workers
	SubjectTaskOutput  = "tasks.output" // streaming output lines from workers
	SubjectTaskCancel  = "tasks.cancel" // cancel a running task
	SubjectAgentStatus = "agents.status"

	// Run protocol subjects (Phase 4B step-by-step execution)
	SubjectRunStart            = "runs.start"             // Go → Python: start a new run
	SubjectRunToolCallRequest  = "runs.toolcall.request"  // Python → Go: request permission for tool call
	SubjectRunToolCallResponse = "runs.toolcall.response" // Go → Python: permission decision
	SubjectRunToolCallResult   = "runs.toolcall.result"   // Python → Go: tool execution result
	SubjectRunComplete         = "runs.complete"          // Python → Go: run finished
	SubjectRunCancel           = "runs.cancel"            // Go → Python: cancel a run
	SubjectRunOutput           = "runs.output"            // Python → Go: streaming output
)

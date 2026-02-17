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
}

// Subject constants for NATS subjects used by CodeForge.
const (
	SubjectTaskCreated = "tasks.created"
	SubjectTaskAgent   = "tasks.agent"  // tasks.agent.{backend} — dispatched to specific backend
	SubjectTaskResult  = "tasks.result" // results from workers
	SubjectTaskOutput  = "tasks.output" // streaming output lines from workers
	SubjectTaskCancel  = "tasks.cancel" // cancel a running task
	SubjectAgentStatus = "agents.status"
)

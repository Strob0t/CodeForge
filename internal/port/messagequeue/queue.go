// Package messagequeue defines the message queue port (interface).
package messagequeue

import "context"

// Handler processes a message received from the queue.
type Handler func(subject string, data []byte) error

// Queue is the port interface for publishing and subscribing to messages.
type Queue interface {
	// Publish sends a message to the given subject.
	Publish(ctx context.Context, subject string, data []byte) error

	// Subscribe registers a handler for messages on the given subject.
	// The returned function cancels the subscription.
	Subscribe(ctx context.Context, subject string, handler Handler) (cancel func(), err error)

	// Close shuts down the queue connection.
	Close() error
}

// Subject constants for NATS subjects used by CodeForge.
const (
	SubjectTaskCreated  = "tasks.created"
	SubjectTaskAssigned = "tasks.agent.assigned"
	SubjectTaskResult   = "tasks.result"
	SubjectAgentStatus  = "agents.status"
)

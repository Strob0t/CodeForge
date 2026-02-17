// Package eventstore defines the port interface for the append-only event store.
package eventstore

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/event"
)

// Store is the port interface for appending and loading agent events.
type Store interface {
	// Append persists a new event to the store.
	Append(ctx context.Context, ev *event.AgentEvent) error

	// LoadByTask returns all events for the given task, ordered by version.
	LoadByTask(ctx context.Context, taskID string) ([]event.AgentEvent, error)

	// LoadByAgent returns all events for the given agent, ordered by version.
	LoadByAgent(ctx context.Context, agentID string) ([]event.AgentEvent, error)
}

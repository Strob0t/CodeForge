// Package eventstore defines the port interface for the append-only event store.
package eventstore

import (
	"context"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/event"
)

// TrajectoryFilter controls which events are returned by LoadTrajectory.
type TrajectoryFilter struct {
	Types  []event.Type `json:"types,omitempty"`
	After  *time.Time   `json:"after,omitempty"`
	Before *time.Time   `json:"before,omitempty"`
}

// TrajectoryPage is a cursor-paginated page of events.
type TrajectoryPage struct {
	Events  []event.AgentEvent `json:"events"`
	Cursor  string             `json:"cursor"`
	HasMore bool               `json:"has_more"`
	Total   int                `json:"total"`
}

// TrajectorySummary contains aggregate stats for a run's trajectory.
type TrajectorySummary struct {
	TotalEvents   int            `json:"total_events"`
	EventCounts   map[string]int `json:"event_counts"`
	DurationMS    int64          `json:"duration_ms"`
	ToolCallCount int            `json:"tool_call_count"`
	ErrorCount    int            `json:"error_count"`
}

// Store is the port interface for appending and loading agent events.
type Store interface {
	// Append persists a new event to the store.
	Append(ctx context.Context, ev *event.AgentEvent) error

	// LoadByTask returns all events for the given task, ordered by version.
	LoadByTask(ctx context.Context, taskID string) ([]event.AgentEvent, error)

	// LoadByAgent returns all events for the given agent, ordered by version.
	LoadByAgent(ctx context.Context, agentID string) ([]event.AgentEvent, error)

	// LoadByRun returns all events for the given run, ordered by version.
	LoadByRun(ctx context.Context, runID string) ([]event.AgentEvent, error)

	// LoadTrajectory returns a cursor-paginated page of events for a run with optional filtering.
	LoadTrajectory(ctx context.Context, runID string, filter TrajectoryFilter, cursor string, limit int) (*TrajectoryPage, error)

	// TrajectoryStats returns aggregate statistics for a run's event trajectory.
	TrajectoryStats(ctx context.Context, runID string) (*TrajectorySummary, error)
}

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Strob0t/CodeForge/internal/domain/event"
)

// EventStore implements eventstore.Store using PostgreSQL (append-only).
type EventStore struct {
	pool *pgxpool.Pool
}

// NewEventStore creates a new EventStore backed by the given connection pool.
func NewEventStore(pool *pgxpool.Pool) *EventStore {
	return &EventStore{pool: pool}
}

// Append inserts a new event into the agent_events table.
func (s *EventStore) Append(ctx context.Context, ev *event.AgentEvent) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO agent_events (agent_id, task_id, project_id, run_id, event_type, payload, request_id, version)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		ev.AgentID, ev.TaskID, ev.ProjectID, nullIfEmpty(ev.RunID), string(ev.Type), ev.Payload, ev.RequestID, ev.Version)
	if err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	return nil
}

// LoadByTask returns all events for the given task, ordered by version ascending.
func (s *EventStore) LoadByTask(ctx context.Context, taskID string) ([]event.AgentEvent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, agent_id, task_id, project_id, COALESCE(run_id::text, ''), event_type, payload, request_id, version, created_at
		 FROM agent_events WHERE task_id = $1 ORDER BY version ASC`, taskID)
	if err != nil {
		return nil, fmt.Errorf("load events by task %s: %w", taskID, err)
	}
	defer rows.Close()

	var events []event.AgentEvent
	for rows.Next() {
		var ev event.AgentEvent
		if err := rows.Scan(&ev.ID, &ev.AgentID, &ev.TaskID, &ev.ProjectID, &ev.RunID, &ev.Type, &ev.Payload, &ev.RequestID, &ev.Version, &ev.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

// LoadByAgent returns all events for the given agent, ordered by version ascending.
func (s *EventStore) LoadByAgent(ctx context.Context, agentID string) ([]event.AgentEvent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, agent_id, task_id, project_id, COALESCE(run_id::text, ''), event_type, payload, request_id, version, created_at
		 FROM agent_events WHERE agent_id = $1 ORDER BY version ASC`, agentID)
	if err != nil {
		return nil, fmt.Errorf("load events by agent %s: %w", agentID, err)
	}
	defer rows.Close()

	var events []event.AgentEvent
	for rows.Next() {
		var ev event.AgentEvent
		if err := rows.Scan(&ev.ID, &ev.AgentID, &ev.TaskID, &ev.ProjectID, &ev.RunID, &ev.Type, &ev.Payload, &ev.RequestID, &ev.Version, &ev.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

// LoadByRun returns all events for the given run, ordered by version ascending.
func (s *EventStore) LoadByRun(ctx context.Context, runID string) ([]event.AgentEvent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, agent_id, task_id, project_id, COALESCE(run_id::text, ''), event_type, payload, request_id, version, created_at
		 FROM agent_events WHERE run_id = $1 ORDER BY version ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("load events by run %s: %w", runID, err)
	}
	defer rows.Close()

	var events []event.AgentEvent
	for rows.Next() {
		var ev event.AgentEvent
		if err := rows.Scan(&ev.ID, &ev.AgentID, &ev.TaskID, &ev.ProjectID, &ev.RunID, &ev.Type, &ev.Payload, &ev.RequestID, &ev.Version, &ev.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

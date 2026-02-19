package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
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
	tid := middleware.TenantIDFromContext(ctx)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO agent_events (tenant_id, agent_id, task_id, project_id, run_id, event_type, payload, request_id, version)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		tid, ev.AgentID, ev.TaskID, ev.ProjectID, nullIfEmpty(ev.RunID), string(ev.Type), ev.Payload, ev.RequestID, ev.Version)
	if err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	return nil
}

// LoadByTask returns all events for the given task, ordered by version ascending.
func (s *EventStore) LoadByTask(ctx context.Context, taskID string) ([]event.AgentEvent, error) {
	tid := middleware.TenantIDFromContext(ctx)
	rows, err := s.pool.Query(ctx,
		`SELECT id, agent_id, task_id, project_id, COALESCE(run_id::text, ''), event_type, payload, request_id, version, created_at
		 FROM agent_events WHERE task_id = $1 AND tenant_id = $2 ORDER BY version ASC`, taskID, tid)
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
	tid := middleware.TenantIDFromContext(ctx)
	rows, err := s.pool.Query(ctx,
		`SELECT id, agent_id, task_id, project_id, COALESCE(run_id::text, ''), event_type, payload, request_id, version, created_at
		 FROM agent_events WHERE agent_id = $1 AND tenant_id = $2 ORDER BY version ASC`, agentID, tid)
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
	tid := middleware.TenantIDFromContext(ctx)
	rows, err := s.pool.Query(ctx,
		`SELECT id, agent_id, task_id, project_id, COALESCE(run_id::text, ''), event_type, payload, request_id, version, created_at
		 FROM agent_events WHERE run_id = $1 AND tenant_id = $2 ORDER BY version ASC`, runID, tid)
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

// LoadTrajectory returns a cursor-paginated page of events for a run with optional filtering.
func (s *EventStore) LoadTrajectory(ctx context.Context, runID string, filter eventstore.TrajectoryFilter, cursor string, limit int) (*eventstore.TrajectoryPage, error) {
	if limit <= 0 {
		limit = 50
	}

	tid := middleware.TenantIDFromContext(ctx)

	// Build dynamic WHERE clause.
	args := []any{runID, tid}
	conditions := []string{"run_id = $1", "tenant_id = $2"}
	argIdx := 3

	if cursor != "" {
		conditions = append(conditions, fmt.Sprintf("id > $%d", argIdx))
		args = append(args, cursor)
		argIdx++
	}
	if len(filter.Types) > 0 {
		types := make([]string, len(filter.Types))
		for i, t := range filter.Types {
			types[i] = string(t)
		}
		conditions = append(conditions, fmt.Sprintf("event_type = ANY($%d)", argIdx))
		args = append(args, types)
		argIdx++
	}
	if filter.After != nil {
		conditions = append(conditions, fmt.Sprintf("created_at > $%d", argIdx))
		args = append(args, *filter.After)
		argIdx++
	}
	if filter.Before != nil {
		conditions = append(conditions, fmt.Sprintf("created_at < $%d", argIdx))
		args = append(args, *filter.Before)
		argIdx++
	}

	where := strings.Join(conditions, " AND ")

	// Count total matching events.
	var total int
	countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM agent_events WHERE %s`, where)
	if err := s.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count trajectory events: %w", err)
	}

	// Fetch limit+1 to detect hasMore.
	fetchSQL := fmt.Sprintf(
		`SELECT id, agent_id, task_id, project_id, COALESCE(run_id::text, ''), event_type, payload, request_id, version, created_at
		 FROM agent_events WHERE %s ORDER BY version ASC LIMIT $%d`,
		where, argIdx)
	args = append(args, limit+1)

	rows, err := s.pool.Query(ctx, fetchSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("load trajectory: %w", err)
	}
	defer rows.Close()

	var events []event.AgentEvent
	for rows.Next() {
		var ev event.AgentEvent
		if err := rows.Scan(&ev.ID, &ev.AgentID, &ev.TaskID, &ev.ProjectID, &ev.RunID, &ev.Type, &ev.Payload, &ev.RequestID, &ev.Version, &ev.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan trajectory event: %w", err)
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}

	var nextCursor string
	if hasMore && len(events) > 0 {
		nextCursor = events[len(events)-1].ID
	}

	return &eventstore.TrajectoryPage{
		Events:  events,
		Cursor:  nextCursor,
		HasMore: hasMore,
		Total:   total,
	}, nil
}

// TrajectoryStats returns aggregate statistics for a run's event trajectory.
func (s *EventStore) TrajectoryStats(ctx context.Context, runID string) (*eventstore.TrajectorySummary, error) {
	tid := middleware.TenantIDFromContext(ctx)

	// Aggregate counts per event type in a single query.
	rows, err := s.pool.Query(ctx,
		`SELECT event_type, COUNT(*) FROM agent_events WHERE run_id = $1 AND tenant_id = $2 GROUP BY event_type`, runID, tid)
	if err != nil {
		return nil, fmt.Errorf("trajectory stats counts: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	var total int
	var toolCalls int
	var errors int
	for rows.Next() {
		var eventType string
		var count int
		if err := rows.Scan(&eventType, &count); err != nil {
			return nil, fmt.Errorf("scan trajectory stat: %w", err)
		}
		counts[eventType] = count
		total += count
		if eventType == string(event.TypeToolCalled) {
			toolCalls = count
		}
		if eventType == string(event.TypeAgentError) {
			errors = count
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Duration: time between first and last event.
	var durationMS int64
	err = s.pool.QueryRow(ctx,
		`SELECT COALESCE(EXTRACT(EPOCH FROM (MAX(created_at) - MIN(created_at))) * 1000, 0)::bigint
		 FROM agent_events WHERE run_id = $1 AND tenant_id = $2`, runID, tid).Scan(&durationMS)
	if err != nil {
		return nil, fmt.Errorf("trajectory duration: %w", err)
	}

	return &eventstore.TrajectorySummary{
		TotalEvents:   total,
		EventCounts:   counts,
		DurationMS:    durationMS,
		ToolCallCount: toolCalls,
		ErrorCount:    errors,
	}, nil
}

// Ensure int-to-string conversion for cursor is available.
var _ = strconv.Itoa

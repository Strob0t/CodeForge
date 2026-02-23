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
		`INSERT INTO agent_events (tenant_id, agent_id, task_id, project_id, run_id, event_type, payload, request_id, version, tool_name, model, tokens_in, tokens_out, cost_usd)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		tid, ev.AgentID, ev.TaskID, ev.ProjectID, nullIfEmpty(ev.RunID), string(ev.Type), ev.Payload, ev.RequestID, ev.Version,
		ev.ToolName, ev.Model, ev.TokensIn, ev.TokensOut, ev.CostUSD)
	if err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	return nil
}

// eventColumns is the SELECT column list for agent_events queries.
const eventColumns = `id, agent_id, task_id, project_id, COALESCE(run_id::text, ''), event_type, payload, request_id, version, created_at, tool_name, model, tokens_in, tokens_out, cost_usd`

// scanEvent scans a row into an AgentEvent including per-tool token columns.
func scanEvent(scanner interface{ Scan(dest ...any) error }, ev *event.AgentEvent) error {
	return scanner.Scan(
		&ev.ID, &ev.AgentID, &ev.TaskID, &ev.ProjectID, &ev.RunID,
		&ev.Type, &ev.Payload, &ev.RequestID, &ev.Version, &ev.CreatedAt,
		&ev.ToolName, &ev.Model, &ev.TokensIn, &ev.TokensOut, &ev.CostUSD,
	)
}

// LoadByTask returns all events for the given task, ordered by version ascending.
func (s *EventStore) LoadByTask(ctx context.Context, taskID string) ([]event.AgentEvent, error) {
	tid := middleware.TenantIDFromContext(ctx)
	rows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM agent_events WHERE task_id = $1 AND tenant_id = $2 ORDER BY version ASC`, eventColumns), taskID, tid)
	if err != nil {
		return nil, fmt.Errorf("load events by task %s: %w", taskID, err)
	}
	defer rows.Close()

	var events []event.AgentEvent
	for rows.Next() {
		var ev event.AgentEvent
		if err := scanEvent(rows, &ev); err != nil {
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
		fmt.Sprintf(`SELECT %s FROM agent_events WHERE agent_id = $1 AND tenant_id = $2 ORDER BY version ASC`, eventColumns), agentID, tid)
	if err != nil {
		return nil, fmt.Errorf("load events by agent %s: %w", agentID, err)
	}
	defer rows.Close()

	var events []event.AgentEvent
	for rows.Next() {
		var ev event.AgentEvent
		if err := scanEvent(rows, &ev); err != nil {
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
		fmt.Sprintf(`SELECT %s FROM agent_events WHERE run_id = $1 AND tenant_id = $2 ORDER BY version ASC`, eventColumns), runID, tid)
	if err != nil {
		return nil, fmt.Errorf("load events by run %s: %w", runID, err)
	}
	defer rows.Close()

	var events []event.AgentEvent
	for rows.Next() {
		var ev event.AgentEvent
		if err := scanEvent(rows, &ev); err != nil {
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
		`SELECT %s FROM agent_events WHERE %s ORDER BY version ASC LIMIT $%d`,
		eventColumns, where, argIdx)
	args = append(args, limit+1)

	rows, err := s.pool.Query(ctx, fetchSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("load trajectory: %w", err)
	}
	defer rows.Close()

	var events []event.AgentEvent
	for rows.Next() {
		var ev event.AgentEvent
		if err := scanEvent(rows, &ev); err != nil {
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

	// Per-tool token totals from tool call result events.
	var totalTokensIn, totalTokensOut int64
	var totalCostUSD float64
	err = s.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(tokens_in), 0), COALESCE(SUM(tokens_out), 0), COALESCE(SUM(cost_usd), 0)
		 FROM agent_events WHERE run_id = $1 AND tenant_id = $2 AND event_type = 'run.toolcall.result'`, runID, tid).
		Scan(&totalTokensIn, &totalTokensOut, &totalCostUSD)
	if err != nil {
		return nil, fmt.Errorf("trajectory token totals: %w", err)
	}

	return &eventstore.TrajectorySummary{
		TotalEvents:    total,
		EventCounts:    counts,
		DurationMS:     durationMS,
		ToolCallCount:  toolCalls,
		ErrorCount:     errors,
		TotalTokensIn:  totalTokensIn,
		TotalTokensOut: totalTokensOut,
		TotalCostUSD:   totalCostUSD,
	}, nil
}

// LoadEventsRange returns events for a run between two event IDs (inclusive).
// If fromEventID is empty, starts from the beginning. If toEventID is empty, goes to the end.
func (s *EventStore) LoadEventsRange(ctx context.Context, runID, fromEventID, toEventID string) ([]event.AgentEvent, error) {
	tid := middleware.TenantIDFromContext(ctx)

	args := []any{runID, tid}
	conditions := []string{"run_id = $1", "tenant_id = $2"}
	argIdx := 3

	if fromEventID != "" {
		conditions = append(conditions, fmt.Sprintf("version >= (SELECT version FROM agent_events WHERE id = $%d)", argIdx))
		args = append(args, fromEventID)
		argIdx++
	}
	if toEventID != "" {
		conditions = append(conditions, fmt.Sprintf("version <= (SELECT version FROM agent_events WHERE id = $%d)", argIdx))
		args = append(args, toEventID)
	}

	where := strings.Join(conditions, " AND ")
	query := fmt.Sprintf(
		`SELECT %s FROM agent_events WHERE %s ORDER BY version ASC`, eventColumns, where)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("load events range: %w", err)
	}
	defer rows.Close()

	var events []event.AgentEvent
	for rows.Next() {
		var ev event.AgentEvent
		if err := scanEvent(rows, &ev); err != nil {
			return nil, fmt.Errorf("scan event range: %w", err)
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

// ListCheckpoints returns events of type tool_result for a run, which serve as checkpoints.
func (s *EventStore) ListCheckpoints(ctx context.Context, runID string) ([]event.AgentEvent, error) {
	tid := middleware.TenantIDFromContext(ctx)
	rows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM agent_events WHERE run_id = $1 AND tenant_id = $2 AND event_type = $3 ORDER BY version ASC`, eventColumns),
		runID, tid, string(event.TypeToolResult))
	if err != nil {
		return nil, fmt.Errorf("list checkpoints: %w", err)
	}
	defer rows.Close()

	var events []event.AgentEvent
	for rows.Next() {
		var ev event.AgentEvent
		if err := scanEvent(rows, &ev); err != nil {
			return nil, fmt.Errorf("scan checkpoint: %w", err)
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

// AppendAudit inserts an audit trail entry.
func (s *EventStore) AppendAudit(ctx context.Context, entry *event.AuditEntry) error {
	tid := middleware.TenantIDFromContext(ctx)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO audit_trail (tenant_id, project_id, run_id, agent_id, action, details)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		tid, entry.ProjectID, nullIfEmpty(entry.RunID), nullIfEmpty(entry.AgentID), entry.Action, entry.Details)
	if err != nil {
		return fmt.Errorf("append audit: %w", err)
	}
	return nil
}

// LoadAudit returns a cursor-paginated page of audit entries.
func (s *EventStore) LoadAudit(ctx context.Context, filter *event.AuditFilter, cursor string, limit int) (*event.AuditPage, error) {
	if limit <= 0 {
		limit = 50
	}

	tid := middleware.TenantIDFromContext(ctx)

	args := []any{tid}
	conditions := []string{"tenant_id = $1"}
	argIdx := 2

	if filter.ProjectID != "" {
		conditions = append(conditions, fmt.Sprintf("project_id = $%d", argIdx))
		args = append(args, filter.ProjectID)
		argIdx++
	}
	if filter.RunID != "" {
		conditions = append(conditions, fmt.Sprintf("run_id = $%d", argIdx))
		args = append(args, filter.RunID)
		argIdx++
	}
	if filter.AgentID != "" {
		conditions = append(conditions, fmt.Sprintf("agent_id = $%d", argIdx))
		args = append(args, filter.AgentID)
		argIdx++
	}
	if filter.Action != "" {
		conditions = append(conditions, fmt.Sprintf("action = $%d", argIdx))
		args = append(args, filter.Action)
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
	if cursor != "" {
		conditions = append(conditions, fmt.Sprintf("id > $%d", argIdx))
		args = append(args, cursor)
		argIdx++
	}

	where := strings.Join(conditions, " AND ")

	// Count total
	var total int
	countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM audit_trail WHERE %s`, where)
	if err := s.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count audit entries: %w", err)
	}

	// Fetch limit+1 to detect hasMore
	fetchSQL := fmt.Sprintf(
		`SELECT id, COALESCE(project_id::text, ''), COALESCE(run_id::text, ''), COALESCE(agent_id::text, ''), action, COALESCE(details, ''), created_at
		 FROM audit_trail WHERE %s ORDER BY created_at DESC LIMIT $%d`,
		where, argIdx)
	args = append(args, limit+1)

	rows, err := s.pool.Query(ctx, fetchSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("load audit: %w", err)
	}
	defer rows.Close()

	var entries []event.AuditEntry
	for rows.Next() {
		var e event.AuditEntry
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.RunID, &e.AgentID, &e.Action, &e.Details, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit entry: %w", err)
		}
		e.TenantID = tid
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	hasMore := len(entries) > limit
	if hasMore {
		entries = entries[:limit]
	}

	var nextCursor string
	if hasMore && len(entries) > 0 {
		nextCursor = entries[len(entries)-1].ID
	}

	return &event.AuditPage{
		Entries: entries,
		Cursor:  nextCursor,
		HasMore: hasMore,
		Total:   total,
	}, nil
}

// Ensure int-to-string conversion for cursor is available.
var _ = strconv.Itoa

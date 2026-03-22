package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
)

// queryBuilder constructs parameterized WHERE clauses with sequential
// placeholder indices, eliminating manual argIdx tracking and the
// associated SQL injection risk from fmt.Sprintf with user-influenced data.
type queryBuilder struct {
	conditions []string
	args       []any
	argIdx     int
}

// newQueryBuilder starts a builder pre-seeded with a tenant_id = $1 condition.
func newQueryBuilder(tenantID string) *queryBuilder {
	return &queryBuilder{
		conditions: []string{"tenant_id = $1"},
		args:       []any{tenantID},
		argIdx:     2,
	}
}

// newQueryBuilderWith starts a builder with an initial named condition and
// the tenant_id condition, e.g. newQueryBuilderWith("run_id", runID, tenantID).
func newQueryBuilderWith(col string, val any, tenantID string) *queryBuilder {
	return &queryBuilder{
		conditions: []string{col + " = $1", "tenant_id = $2"},
		args:       []any{val, tenantID},
		argIdx:     3,
	}
}

// addCondition appends a parameterized condition. tmpl must contain exactly
// one %d verb for the placeholder index, e.g. "created_at > $%d".
func (qb *queryBuilder) addCondition(tmpl string, val any) {
	qb.conditions = append(qb.conditions, fmt.Sprintf(tmpl, qb.argIdx))
	qb.args = append(qb.args, val)
	qb.argIdx++
}

// addRawCondition appends a literal condition with no parameter (e.g. a
// static expression). The caller is responsible for ensuring safety.
func (qb *queryBuilder) addRawCondition(cond string) {
	qb.conditions = append(qb.conditions, cond)
}

// where returns the joined WHERE clause.
func (qb *queryBuilder) where() string {
	return strings.Join(qb.conditions, " AND ")
}

// addLimit appends a LIMIT parameter and returns its placeholder index.
func (qb *queryBuilder) addLimit(limit int) int {
	idx := qb.argIdx
	qb.args = append(qb.args, limit)
	qb.argIdx++
	return idx
}

// EventStore implements eventstore.Store using PostgreSQL (append-only).
type EventStore struct {
	pool *pgxpool.Pool
}

// NewEventStore creates a new EventStore backed by the given connection pool.
func NewEventStore(pool *pgxpool.Pool) *EventStore {
	return &EventStore{pool: pool}
}

// Append inserts a new event into the agent_events table.
// The database assigns sequence_number via the sequence default; the assigned value
// is written back to ev.SequenceNumber.
func (s *EventStore) Append(ctx context.Context, ev *event.AgentEvent) error {
	tid := middleware.TenantIDFromContext(ctx)
	err := s.pool.QueryRow(ctx,
		`INSERT INTO agent_events (tenant_id, agent_id, task_id, project_id, run_id, event_type, payload, request_id, version, tool_name, model, tokens_in, tokens_out, cost_usd)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		 RETURNING sequence_number`,
		tid, ev.AgentID, ev.TaskID, ev.ProjectID, nullIfEmpty(ev.RunID), string(ev.Type), ev.Payload, ev.RequestID, ev.Version,
		ev.ToolName, ev.Model, ev.TokensIn, ev.TokensOut, ev.CostUSD).Scan(&ev.SequenceNumber)
	if err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	return nil
}

// eventColumns is the SELECT column list for agent_events queries.
const eventColumns = `id, agent_id, task_id, project_id, COALESCE(run_id::text, ''), event_type, payload, request_id, version, sequence_number, created_at, tool_name, model, tokens_in, tokens_out, cost_usd`

// scanEvent scans a row into an AgentEvent including per-tool token columns.
func scanEvent(scanner interface{ Scan(dest ...any) error }, ev *event.AgentEvent) error {
	return scanner.Scan(
		&ev.ID, &ev.AgentID, &ev.TaskID, &ev.ProjectID, &ev.RunID,
		&ev.Type, &ev.Payload, &ev.RequestID, &ev.Version, &ev.SequenceNumber, &ev.CreatedAt,
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
	return scanRows(rows, func(r pgx.Rows) (event.AgentEvent, error) {
		var ev event.AgentEvent
		err := scanEvent(r, &ev)
		return ev, err
	})
}

// LoadByAgent returns all events for the given agent, ordered by version ascending.
func (s *EventStore) LoadByAgent(ctx context.Context, agentID string) ([]event.AgentEvent, error) {
	tid := middleware.TenantIDFromContext(ctx)
	rows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM agent_events WHERE agent_id = $1 AND tenant_id = $2 ORDER BY version ASC`, eventColumns), agentID, tid)
	if err != nil {
		return nil, fmt.Errorf("load events by agent %s: %w", agentID, err)
	}
	return scanRows(rows, func(r pgx.Rows) (event.AgentEvent, error) {
		var ev event.AgentEvent
		err := scanEvent(r, &ev)
		return ev, err
	})
}

// LoadByRun returns all events for the given run, ordered by version ascending.
func (s *EventStore) LoadByRun(ctx context.Context, runID string) ([]event.AgentEvent, error) {
	tid := middleware.TenantIDFromContext(ctx)
	rows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM agent_events WHERE run_id = $1 AND tenant_id = $2 ORDER BY version ASC`, eventColumns), runID, tid)
	if err != nil {
		return nil, fmt.Errorf("load events by run %s: %w", runID, err)
	}
	return scanRows(rows, func(r pgx.Rows) (event.AgentEvent, error) {
		var ev event.AgentEvent
		err := scanEvent(r, &ev)
		return ev, err
	})
}

// LoadTrajectory returns a cursor-paginated page of events for a run with optional filtering.
func (s *EventStore) LoadTrajectory(ctx context.Context, runID string, filter eventstore.TrajectoryFilter, cursor string, limit int) (*eventstore.TrajectoryPage, error) {
	if limit <= 0 {
		limit = 50
	}

	tid := middleware.TenantIDFromContext(ctx)
	qb := newQueryBuilderWith("run_id", runID, tid)

	if cursor != "" {
		qb.addCondition("id > $%d", cursor)
	}
	if len(filter.Types) > 0 {
		types := make([]string, len(filter.Types))
		for i, t := range filter.Types {
			types[i] = string(t)
		}
		qb.addCondition("event_type = ANY($%d)", types)
	}
	if filter.After != nil {
		qb.addCondition("created_at > $%d", *filter.After)
	}
	if filter.Before != nil {
		qb.addCondition("created_at < $%d", *filter.Before)
	}
	if filter.AfterSequence > 0 {
		qb.addCondition("sequence_number > $%d", filter.AfterSequence)
	}

	// Count total matching events.
	var total int
	countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM agent_events WHERE %s`, qb.where())
	if err := s.pool.QueryRow(ctx, countSQL, qb.args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count trajectory events: %w", err)
	}

	// Fetch limit+1 to detect hasMore.
	limitIdx := qb.addLimit(limit + 1)
	fetchSQL := fmt.Sprintf(
		`SELECT %s FROM agent_events WHERE %s ORDER BY sequence_number ASC LIMIT $%d`,
		eventColumns, qb.where(), limitIdx)

	rows, err := s.pool.Query(ctx, fetchSQL, qb.args...)
	if err != nil {
		return nil, fmt.Errorf("load trajectory: %w", err)
	}
	events, err := scanRows(rows, func(r pgx.Rows) (event.AgentEvent, error) {
		var ev event.AgentEvent
		err := scanEvent(r, &ev)
		return ev, err
	})
	if err != nil {
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
	qb := newQueryBuilderWith("run_id", runID, tid)

	if fromEventID != "" {
		qb.addCondition("version >= (SELECT version FROM agent_events WHERE id = $%d)", fromEventID)
	}
	if toEventID != "" {
		qb.addCondition("version <= (SELECT version FROM agent_events WHERE id = $%d)", toEventID)
	}

	query := fmt.Sprintf(
		`SELECT %s FROM agent_events WHERE %s ORDER BY version ASC`, eventColumns, qb.where())

	rows, err := s.pool.Query(ctx, query, qb.args...)
	if err != nil {
		return nil, fmt.Errorf("load events range: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (event.AgentEvent, error) {
		var ev event.AgentEvent
		err := scanEvent(r, &ev)
		return ev, err
	})
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
	return scanRows(rows, func(r pgx.Rows) (event.AgentEvent, error) {
		var ev event.AgentEvent
		err := scanEvent(r, &ev)
		return ev, err
	})
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
	qb := newQueryBuilder(tid)

	if filter.ProjectID != "" {
		qb.addCondition("project_id = $%d", filter.ProjectID)
	}
	if filter.RunID != "" {
		qb.addCondition("run_id = $%d", filter.RunID)
	}
	if filter.AgentID != "" {
		qb.addCondition("agent_id = $%d", filter.AgentID)
	}
	if filter.Action != "" {
		qb.addCondition("action = $%d", filter.Action)
	}
	if filter.After != nil {
		qb.addCondition("created_at > $%d", *filter.After)
	}
	if filter.Before != nil {
		qb.addCondition("created_at < $%d", *filter.Before)
	}
	if cursor != "" {
		qb.addCondition("id > $%d", cursor)
	}

	// Count total
	var total int
	countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM audit_trail WHERE %s`, qb.where())
	if err := s.pool.QueryRow(ctx, countSQL, qb.args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count audit entries: %w", err)
	}

	// Fetch limit+1 to detect hasMore
	limitIdx := qb.addLimit(limit + 1)
	fetchSQL := fmt.Sprintf(
		`SELECT id, COALESCE(project_id::text, ''), COALESCE(run_id::text, ''), COALESCE(agent_id::text, ''), action, COALESCE(details, ''), created_at
		 FROM audit_trail WHERE %s ORDER BY created_at DESC LIMIT $%d`,
		qb.where(), limitIdx)

	rows, err := s.pool.Query(ctx, fetchSQL, qb.args...)
	if err != nil {
		return nil, fmt.Errorf("load audit: %w", err)
	}
	entries, err := scanRows(rows, func(r pgx.Rows) (event.AuditEntry, error) {
		var e event.AuditEntry
		if err := r.Scan(&e.ID, &e.ProjectID, &e.RunID, &e.AgentID, &e.Action, &e.Details, &e.CreatedAt); err != nil {
			return e, err
		}
		e.TenantID = tid
		return e, nil
	})
	if err != nil {
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

package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
)

// Worker heartbeats (KI-65): work acked on accept (ADR-016) is never
// redelivered, so the stuck-work watchdog ends work whose worker stopped
// sending heartbeats. A heartbeat only counts for work that still waits for
// its worker; work without any heartbeat has not been accepted yet.

// TouchRunHeartbeat records a worker heartbeat of a running run of the
// caller's tenant; heartbeats of other runs are ignored.
func (s *Store) TouchRunHeartbeat(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE runs SET last_heartbeat_at = now()
		 WHERE id = $1 AND tenant_id = $2 AND status = 'running'`, id, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("touch run heartbeat %s: %w", id, err)
	}
	return nil
}

// ListRunsWithStaleHeartbeat returns up to limit running runs whose last
// heartbeat is older than idleFor, oldest heartbeat first.
//
// INTENTIONALLY CROSS-TENANT: the stuck-work watchdog ends the runs of every
// tenant whose worker died. The returned runs carry their tenant_id, and the
// caller ends each one in its tenant's context.
func (s *Store) ListRunsWithStaleHeartbeat(ctx context.Context, idleFor time.Duration, limit int) ([]run.Run, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+runColumns+` FROM runs
		 WHERE status = 'running' AND last_heartbeat_at < now() - $1::interval
		 ORDER BY last_heartbeat_at LIMIT $2`, idleFor, limit)
	if err != nil {
		return nil, fmt.Errorf("list runs with stale heartbeat: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (run.Run, error) {
		return scanRun(r)
	})
}

// BeginConversationTurn makes turnID the active turn of a conversation of the
// caller's tenant, without heartbeat.
func (s *Store) BeginConversationTurn(ctx context.Context, conversationID, turnID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE conversations SET active_turn_id = $2, active_turn_heartbeat_at = NULL
		 WHERE id = $1 AND tenant_id = $3`, conversationID, turnID, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("begin conversation turn %s: %w", conversationID, err)
	}
	return nil
}

// EndConversationTurn clears the active turn of a conversation of the
// caller's tenant if it is turnID (turnID "" clears any active turn), and
// reports whether it did: false when the turn already ended or another turn
// is active.
func (s *Store) EndConversationTurn(ctx context.Context, conversationID, turnID string) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE conversations SET active_turn_id = NULL, active_turn_heartbeat_at = NULL
		 WHERE id = $1 AND tenant_id = $2 AND active_turn_id IS NOT NULL
		   AND ($3 = '' OR active_turn_id = $3)`, conversationID, tenantFromCtx(ctx), turnID)
	if err != nil {
		return false, fmt.Errorf("end conversation turn %s: %w", conversationID, err)
	}
	return tag.RowsAffected() > 0, nil
}

// TouchConversationTurnHeartbeat records a worker heartbeat of the active
// turn of a conversation of the caller's tenant; heartbeats of another turn
// (a stopped run) are ignored.
func (s *Store) TouchConversationTurnHeartbeat(ctx context.Context, conversationID, turnID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE conversations SET active_turn_heartbeat_at = now()
		 WHERE id = $1 AND tenant_id = $2 AND active_turn_id = $3`, conversationID, tenantFromCtx(ctx), turnID)
	if err != nil {
		return fmt.Errorf("touch conversation turn heartbeat %s: %w", conversationID, err)
	}
	return nil
}

// ListConversationTurnsWithStaleHeartbeat returns up to limit active turns
// whose last heartbeat is older than idleFor, oldest heartbeat first.
//
// INTENTIONALLY CROSS-TENANT: the stuck-work watchdog ends the conversation
// runs of every tenant whose worker died. The returned turns carry their
// tenant_id, and the caller ends each one in its tenant's context.
func (s *Store) ListConversationTurnsWithStaleHeartbeat(ctx context.Context, idleFor time.Duration, limit int) ([]conversation.ActiveTurn, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, active_turn_id FROM conversations
		 WHERE active_turn_id IS NOT NULL AND active_turn_heartbeat_at < now() - $1::interval
		 ORDER BY active_turn_heartbeat_at LIMIT $2`, idleFor, limit)
	if err != nil {
		return nil, fmt.Errorf("list conversation turns with stale heartbeat: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (conversation.ActiveTurn, error) {
		var t conversation.ActiveTurn
		err := r.Scan(&t.ConversationID, &t.TenantID, &t.TurnID)
		return t, err
	})
}

// TouchTaskHeartbeat records a worker heartbeat of a queued or running task
// of the caller's tenant for its dispatch dispatchID. A heartbeat of another
// dispatch (a late one of an earlier dispatch) is ignored: a re-dispatched
// task has no heartbeat until the worker of its new dispatch sends one. A
// task dispatched before dispatches had IDs matches dispatchID "".
func (s *Store) TouchTaskHeartbeat(ctx context.Context, id, dispatchID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO task_heartbeats (task_id, tenant_id, task_version, dispatch_id, beat_at)
		 SELECT id, tenant_id, version, dispatch_id, now() FROM tasks
		 WHERE id = $1 AND tenant_id = $2 AND status IN ('queued', 'running')
		   AND COALESCE(dispatch_id, '') = $3
		 ON CONFLICT (task_id) DO UPDATE
		 SET task_version = EXCLUDED.task_version, dispatch_id = EXCLUDED.dispatch_id, beat_at = EXCLUDED.beat_at`,
		id, tenantFromCtx(ctx), dispatchID)
	if err != nil {
		return fmt.Errorf("touch task heartbeat %s: %w", id, err)
	}
	return nil
}

// taskColumnsPrefixed are the task columns scanDispatchedTask reads, of table alias t.
var taskColumnsPrefixed = "t." + strings.Join([]string{
	"id", "project_id", "agent_id", "title", "prompt", "status", "result", "cost_usd", "version", "created_at", "updated_at", "tenant_id",
}, ", t.") + ", COALESCE(t.dispatch_id, '')"

// ListTasksWithStaleHeartbeat returns up to limit queued or running tasks
// whose last heartbeat for their current dispatch is older than idleFor,
// oldest heartbeat first, with their dispatch.
//
// INTENTIONALLY CROSS-TENANT: the stuck-work watchdog fails the tasks of
// every tenant whose worker died. The returned tasks carry their tenant_id,
// and the caller fails each one in its tenant's context.
func (s *Store) ListTasksWithStaleHeartbeat(ctx context.Context, idleFor time.Duration, limit int) ([]task.Task, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+taskColumnsPrefixed+` FROM tasks t
		 JOIN task_heartbeats h ON h.task_id = t.id AND h.dispatch_id IS NOT DISTINCT FROM t.dispatch_id
		 WHERE t.status IN ('queued', 'running') AND h.beat_at < now() - $1::interval
		 ORDER BY h.beat_at LIMIT $2`, idleFor, limit)
	if err != nil {
		return nil, fmt.Errorf("list tasks with stale heartbeat: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (task.Task, error) {
		return scanDispatchedTask(r)
	})
}

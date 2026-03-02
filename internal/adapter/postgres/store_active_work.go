package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain/task"
)

// ListActiveWork returns all running/queued tasks for a project, joined with
// their assigned agent metadata and latest run metrics.
func (s *Store) ListActiveWork(ctx context.Context, projectID string) ([]task.ActiveWorkItem, error) {
	const q = `
		SELECT t.id, t.title, t.status, t.project_id,
			   COALESCE(t.agent_id, '') AS agent_id,
			   COALESCE(a.name, '') AS agent_name,
			   COALESCE(a.mode_id, '') AS agent_mode,
			   COALESCE(r.id, '') AS run_id,
			   COALESCE(r.step_count, 0) AS step_count,
			   COALESCE(r.cost_usd, 0) AS cost_usd,
			   COALESCE(r.started_at, t.updated_at) AS started_at
		FROM tasks t
		LEFT JOIN agents a ON a.id = t.agent_id
		LEFT JOIN LATERAL (
			SELECT id, step_count, cost_usd, started_at
			FROM runs
			WHERE runs.task_id = t.id
			ORDER BY created_at DESC
			LIMIT 1
		) r ON true
		WHERE t.project_id = $1
		  AND t.status IN ('queued', 'running')
		  AND t.tenant_id = $2
		ORDER BY t.updated_at ASC`

	rows, err := s.pool.Query(ctx, q, projectID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list active work: %w", err)
	}
	defer rows.Close()

	var items []task.ActiveWorkItem
	for rows.Next() {
		var item task.ActiveWorkItem
		if err := rows.Scan(
			&item.TaskID, &item.TaskTitle, &item.TaskStatus, &item.ProjectID,
			&item.AgentID, &item.AgentName, &item.AgentMode,
			&item.RunID, &item.StepCount, &item.CostUSD, &item.StartedAt,
		); err != nil {
			return nil, fmt.Errorf("scan active work item: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ClaimTask atomically claims a pending task for the given agent using
// optimistic locking. Returns ClaimResult{Claimed: false} if the task is
// no longer pending or the version has changed (concurrent claim).
func (s *Store) ClaimTask(ctx context.Context, taskID, agentID string, version int) (*task.ClaimResult, error) {
	const q = `
		UPDATE tasks
		SET agent_id = $2, status = 'queued', version = version + 1, updated_at = NOW()
		WHERE id = $1 AND status = 'pending' AND version = $3 AND tenant_id = $4
		RETURNING id, project_id, agent_id, title, prompt, status, result, cost_usd, version, created_at, updated_at`

	t, err := scanTask(s.pool.QueryRow(ctx, q, taskID, agentID, version, tenantFromCtx(ctx)))
	if err != nil {
		// No rows affected means task was already claimed or version mismatch.
		if errors.Is(err, pgx.ErrNoRows) {
			return &task.ClaimResult{Claimed: false, Reason: "task already claimed or version mismatch"}, nil
		}
		return nil, fmt.Errorf("claim task %s: %w", taskID, err)
	}

	return &task.ClaimResult{Task: &t, Claimed: true}, nil
}

// ReleaseStaleWork finds tasks stuck in running/queued status longer than the
// given threshold and resets them to pending with no assigned agent.
// Returns the list of released tasks.
func (s *Store) ReleaseStaleWork(ctx context.Context, threshold time.Duration) ([]task.Task, error) {
	const q = `
		UPDATE tasks
		SET status = 'pending', agent_id = NULL, version = version + 1, updated_at = NOW()
		WHERE status IN ('running', 'queued')
		  AND updated_at < NOW() - $1::interval
		RETURNING id, project_id, agent_id, title, prompt, status, result, cost_usd, version, created_at, updated_at`

	rows, err := s.pool.Query(ctx, q, threshold)
	if err != nil {
		return nil, fmt.Errorf("release stale work: %w", err)
	}
	defer rows.Close()

	var tasks []task.Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

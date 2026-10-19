package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// projectActiveWork tells whether a project has a run that has not ended, a
// conversation with an active turn, or a task queued or running.
const projectActiveWork = `
	SELECT EXISTS (SELECT 1 FROM runs
	               WHERE project_id = $1 AND tenant_id = $2 AND status IN ('pending', 'running', 'quality_gate'))
	    OR EXISTS (SELECT 1 FROM conversations
	               WHERE project_id = $1 AND tenant_id = $2 AND active_turn_id IS NOT NULL)
	    OR EXISTS (SELECT 1 FROM tasks
	               WHERE project_id = $1 AND tenant_id = $2 AND status IN ('queued', 'running'))`

// DeleteProjectForWorkspaceDeletion deletes the project and records its
// workspace's deletion in one transaction (KI-96 D11). The project row is
// locked first, so no new work of it can be stored meanwhile.
func (s *Store) DeleteProjectForWorkspaceDeletion(ctx context.Context, projectID string, d *project.WorkspaceDeletion) error {
	tenantID := tenantFromCtx(ctx)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("delete project %s: begin tx: %w", projectID, err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	var id string
	if err := tx.QueryRow(ctx, `SELECT id FROM projects WHERE id = $1 AND tenant_id = $2 FOR UPDATE`,
		projectID, tenantID).Scan(&id); err != nil {
		return notFoundWrap(err, "delete project %s", projectID)
	}
	var busy bool
	if err := tx.QueryRow(ctx, projectActiveWork, projectID, tenantID).Scan(&busy); err != nil {
		return fmt.Errorf("delete project %s: active work: %w", projectID, err)
	}
	if busy {
		return fmt.Errorf("delete project %s: %w", projectID, project.ErrProjectBusy)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM projects WHERE id = $1 AND tenant_id = $2`, projectID, tenantID); err != nil {
		return fmt.Errorf("delete project %s: %w", projectID, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO workspace_deletions (id, tenant_id, project_id, workspace_path, tool_uid)
		 VALUES ($1, $2, $3, $4, $5)`,
		d.ID, tenantID, projectID, d.WorkspacePath, d.ToolUID); err != nil {
		return fmt.Errorf("record workspace deletion of project %s: %w", projectID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("delete project %s: commit: %w", projectID, err)
	}
	return nil
}

// RecordWorkspaceDeletion records the deletion of a workspace a re-clone
// replaces, for a project of the tenant in ctx (domain.ErrNotFound for
// another's); the project stays.
func (s *Store) RecordWorkspaceDeletion(ctx context.Context, d *project.WorkspaceDeletion) error {
	tag, err := s.pool.Exec(ctx,
		`INSERT INTO workspace_deletions (id, tenant_id, project_id, workspace_path, tool_uid)
		 SELECT $1::uuid, tenant_id, id, $4::text, $5::integer FROM projects WHERE id = $3 AND tenant_id = $2`,
		d.ID, tenantFromCtx(ctx), d.ProjectID, d.WorkspacePath, d.ToolUID)
	return execExpectOne(tag, err, "record workspace deletion of project %s", d.ProjectID)
}

// MarkWorkspaceDeletionDone marks a deletion of the tenant in ctx done.
func (s *Store) MarkWorkspaceDeletionDone(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE workspace_deletions SET done_at = COALESCE(done_at, now()), attempts = attempts + 1, last_error = ''
		 WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "mark workspace deletion %s done", id)
}

// RecordWorkspaceDeletionFailure counts a failed attempt of a pending deletion.
func (s *Store) RecordWorkspaceDeletionFailure(ctx context.Context, id, message string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE workspace_deletions SET attempts = attempts + 1, last_error = $3
		 WHERE id = $1 AND tenant_id = $2 AND done_at IS NULL`, id, tenantFromCtx(ctx), message)
	if err != nil {
		return fmt.Errorf("record workspace deletion failure %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("record workspace deletion failure %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

// ListPendingWorkspaceDeletions returns deletions not done, requested before
// olderThan ago, oldest first.
//
// INTENTIONALLY CROSS-TENANT: the retry job republishes every tenant's pending
// deletions; each is published under its own tenant.
func (s *Store) ListPendingWorkspaceDeletions(ctx context.Context, olderThan time.Duration, limit int) ([]project.WorkspaceDeletion, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, project_id, workspace_path, tool_uid, requested_at, done_at, attempts, last_error
		 FROM workspace_deletions
		 WHERE done_at IS NULL AND requested_at < now() - make_interval(secs => $1)
		 ORDER BY requested_at
		 LIMIT $2`, olderThan.Seconds(), limit)
	if err != nil {
		return nil, fmt.Errorf("list pending workspace deletions: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (project.WorkspaceDeletion, error) {
		var d project.WorkspaceDeletion
		err := r.Scan(&d.ID, &d.TenantID, &d.ProjectID, &d.WorkspacePath, &d.ToolUID, &d.RequestedAt, &d.DoneAt,
			&d.Attempts, &d.LastError)
		return d, err
	})
}

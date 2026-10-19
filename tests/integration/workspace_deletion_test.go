//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-96 D11: with workspace.tool_acls: required a deleted project's workspace
// is removed by the worker as the tenant's tool UID. The project row's
// removal and the workspace_deletions record are one transaction; the retry
// job republishes pending deletions of every tenant.

func insertProject(t *testing.T, pool *pgxpool.Pool, tenantID string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO projects (name, tenant_id, workspace_path) VALUES ('p', $1, $2) RETURNING id`,
		tenantID, "/data/workspaces/"+tenantID+"/p").Scan(&id); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return id
}

func newDeletion(tenantID, projectID string) *project.WorkspaceDeletion {
	return &project.WorkspaceDeletion{
		ID:            uuid.NewString(),
		TenantID:      tenantID,
		ProjectID:     projectID,
		WorkspacePath: "/data/workspaces/" + tenantID + "/" + projectID,
		ToolUID:       20000,
	}
}

func projectExists(t *testing.T, pool *pgxpool.Pool, id string) bool {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(context.Background(), `SELECT EXISTS (SELECT 1 FROM projects WHERE id = $1)`, id).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	return exists
}

func deletionCount(t *testing.T, pool *pgxpool.Pool, projectID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM workspace_deletions WHERE project_id = $1`, projectID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDeleteProjectForWorkspaceDeletion(t *testing.T) {
	store, pool := toolUIDDatabase(t)
	a := insertTenant(t, pool, "a", time.Now())
	b := insertTenant(t, pool, "b", time.Now())
	ctxA := tenantctx.WithTenant(context.Background(), a)
	ctxB := tenantctx.WithTenant(context.Background(), b)
	p := insertProject(t, pool, a)

	// Another tenant's project is not found and stays.
	if err := store.DeleteProjectForWorkspaceDeletion(ctxB, p, newDeletion(b, p)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other tenant: err = %v, want ErrNotFound", err)
	}
	if !projectExists(t, pool, p) || deletionCount(t, pool, p) != 0 {
		t.Fatal("another tenant deleted the project or recorded a deletion")
	}

	d := newDeletion(a, p)
	if err := store.DeleteProjectForWorkspaceDeletion(ctxA, p, d); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if projectExists(t, pool, p) {
		t.Fatal("the project row is still there")
	}
	var got project.WorkspaceDeletion
	if err := pool.QueryRow(context.Background(),
		`SELECT id, tenant_id, project_id, workspace_path, tool_uid, done_at, attempts, last_error
		 FROM workspace_deletions WHERE id = $1`, d.ID).Scan(
		&got.ID, &got.TenantID, &got.ProjectID, &got.WorkspacePath, &got.ToolUID, &got.DoneAt, &got.Attempts, &got.LastError); err != nil {
		t.Fatalf("read the deletion: %v", err)
	}
	if got.TenantID != a || got.ProjectID != p || got.WorkspacePath != d.WorkspacePath || got.ToolUID != 20000 ||
		got.DoneAt != nil || got.Attempts != 0 || got.LastError != "" {
		t.Fatalf("deletion = %+v", got)
	}
	// A second delete of the same project is not found and records nothing.
	if err := store.DeleteProjectForWorkspaceDeletion(ctxA, p, newDeletion(a, p)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second delete: err = %v, want ErrNotFound", err)
	}
	if n := deletionCount(t, pool, p); n != 1 {
		t.Fatalf("%d deletions recorded, want 1", n)
	}
}

// TestDeleteProjectForWorkspaceDeletion_ActiveWork: a project with a run that
// has not ended, a conversation with an active turn or a queued or running
// task is not deleted (409), and nothing is recorded.
func TestDeleteProjectForWorkspaceDeletion_ActiveWork(t *testing.T) {
	store, pool := toolUIDDatabase(t)
	ctx := context.Background()
	a := insertTenant(t, pool, "a", time.Now())
	ctxA := tenantctx.WithTenant(ctx, a)

	cases := []struct {
		name  string
		setup func(t *testing.T, projectID string) (end string)
	}{
		{"running run", func(t *testing.T, projectID string) string {
			var taskID, agentID string
			if err := pool.QueryRow(ctx, `INSERT INTO tasks (project_id, tenant_id, title, status) VALUES ($1, $2, 't', 'completed') RETURNING id`,
				projectID, a).Scan(&taskID); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `INSERT INTO agents (project_id, tenant_id, name) VALUES ($1, $2, 'ag') RETURNING id`,
				projectID, a).Scan(&agentID); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO runs (task_id, agent_id, project_id, tenant_id, status) VALUES ($1, $2, $3, $4, 'quality_gate')`,
				taskID, agentID, projectID, a); err != nil {
				t.Fatal(err)
			}
			return `UPDATE runs SET status = 'completed' WHERE project_id = $1`
		}},
		{"active conversation turn", func(t *testing.T, projectID string) string {
			if _, err := pool.Exec(ctx, `INSERT INTO conversations (project_id, tenant_id, active_turn_id) VALUES ($1, $2, 'turn-1')`,
				projectID, a); err != nil {
				t.Fatal(err)
			}
			return `UPDATE conversations SET active_turn_id = NULL WHERE project_id = $1`
		}},
		{"queued task", func(t *testing.T, projectID string) string {
			if _, err := pool.Exec(ctx, `INSERT INTO tasks (project_id, tenant_id, title, status) VALUES ($1, $2, 't', 'queued')`,
				projectID, a); err != nil {
				t.Fatal(err)
			}
			return `UPDATE tasks SET status = 'cancelled' WHERE project_id = $1`
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := insertProject(t, pool, a)
			end := tc.setup(t, p)
			if err := store.DeleteProjectForWorkspaceDeletion(ctxA, p, newDeletion(a, p)); !errors.Is(err, project.ErrProjectBusy) {
				t.Fatalf("err = %v, want ErrProjectBusy", err)
			}
			if !projectExists(t, pool, p) || deletionCount(t, pool, p) != 0 {
				t.Fatal("a busy project was deleted or its deletion recorded")
			}
			if _, err := pool.Exec(ctx, end, p); err != nil {
				t.Fatal(err)
			}
			if err := store.DeleteProjectForWorkspaceDeletion(ctxA, p, newDeletion(a, p)); err != nil {
				t.Fatalf("after the work ended: %v", err)
			}
		})
	}
}

// TestRecordWorkspaceDeletion: a workspace a re-clone replaces (KI-189) is
// recorded for a project of the tenant in ctx; the project stays.
func TestRecordWorkspaceDeletion(t *testing.T) {
	store, pool := toolUIDDatabase(t)
	a := insertTenant(t, pool, "a", time.Now())
	b := insertTenant(t, pool, "b", time.Now())
	ctxA := tenantctx.WithTenant(context.Background(), a)
	ctxB := tenantctx.WithTenant(context.Background(), b)
	p := insertProject(t, pool, a)

	if err := store.RecordWorkspaceDeletion(ctxB, newDeletion(b, p)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other tenant's project: err = %v, want ErrNotFound", err)
	}
	if deletionCount(t, pool, p) != 0 {
		t.Fatal("a deletion was recorded for another tenant's project")
	}
	d := newDeletion(a, p)
	d.WorkspacePath += ".discarded-" + d.ID
	if err := store.RecordWorkspaceDeletion(ctxA, d); err != nil {
		t.Fatalf("record: %v", err)
	}
	if !projectExists(t, pool, p) {
		t.Fatal("the project row was deleted")
	}
	pending, err := store.ListPendingWorkspaceDeletions(context.Background(), -time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != d.ID || pending[0].TenantID != a || pending[0].ProjectID != p ||
		pending[0].WorkspacePath != d.WorkspacePath || pending[0].ToolUID != d.ToolUID {
		t.Fatalf("pending = %+v, want %+v", pending, d)
	}
}

func TestWorkspaceDeletionLifecycle(t *testing.T) {
	store, pool := toolUIDDatabase(t)
	ctx := context.Background()
	a := insertTenant(t, pool, "a", time.Now())
	b := insertTenant(t, pool, "b", time.Now())
	ctxA := tenantctx.WithTenant(ctx, a)
	ctxB := tenantctx.WithTenant(ctx, b)

	pa1, pa2, pb := insertProject(t, pool, a), insertProject(t, pool, a), insertProject(t, pool, b)
	da1, da2, db := newDeletion(a, pa1), newDeletion(a, pa2), newDeletion(b, pb)
	for _, x := range []struct {
		ctx context.Context
		p   string
		d   *project.WorkspaceDeletion
	}{{ctxA, pa1, da1}, {ctxA, pa2, da2}, {ctxB, pb, db}} {
		if err := store.DeleteProjectForWorkspaceDeletion(x.ctx, x.p, x.d); err != nil {
			t.Fatal(err)
		}
	}
	// Requested 30, 20 and 1 minutes ago.
	for id, ago := range map[string]string{da1.ID: "30 minutes", db.ID: "20 minutes", da2.ID: "1 minute"} {
		if _, err := pool.Exec(ctx, `UPDATE workspace_deletions SET requested_at = now() - $2::interval WHERE id = $1`, id, ago); err != nil {
			t.Fatal(err)
		}
	}

	// The retry job sees every tenant's deletions older than the interval, oldest first.
	pending, err := store.ListPendingWorkspaceDeletions(ctx, 10*time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 || pending[0].ID != da1.ID || pending[1].ID != db.ID ||
		pending[0].TenantID != a || pending[1].TenantID != b || pending[0].ToolUID != 20000 {
		t.Fatalf("pending = %+v", pending)
	}
	if limited, err := store.ListPendingWorkspaceDeletions(ctx, 10*time.Minute, 1); err != nil || len(limited) != 1 {
		t.Fatalf("limit 1: %d, %v", len(limited), err)
	}

	// A failure is counted and kept; another tenant cannot touch the deletion.
	if err := store.RecordWorkspaceDeletionFailure(ctxB, da1.ID, "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other tenant failure: %v", err)
	}
	if err := store.RecordWorkspaceDeletionFailure(ctxA, da1.ID, "rm: Permission denied"); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkWorkspaceDeletionDone(ctxB, da1.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other tenant done: %v", err)
	}
	var attempts int
	var lastError string
	if err := pool.QueryRow(ctx, `SELECT attempts, last_error FROM workspace_deletions WHERE id = $1`, da1.ID).Scan(&attempts, &lastError); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || lastError != "rm: Permission denied" {
		t.Fatalf("attempts %d, last_error %q", attempts, lastError)
	}

	// Done: no longer pending; a redelivered result keeps the first done_at.
	if err := store.MarkWorkspaceDeletionDone(ctxA, da1.ID); err != nil {
		t.Fatal(err)
	}
	var doneAt time.Time
	if err := pool.QueryRow(ctx, `SELECT done_at, last_error FROM workspace_deletions WHERE id = $1`, da1.ID).Scan(&doneAt, &lastError); err != nil {
		t.Fatal(err)
	}
	if lastError != "" {
		t.Fatalf("last_error after done: %q", lastError)
	}
	if err := store.MarkWorkspaceDeletionDone(ctxA, da1.ID); err != nil {
		t.Fatalf("done twice: %v", err)
	}
	var doneAgain time.Time
	if err := pool.QueryRow(ctx, `SELECT done_at FROM workspace_deletions WHERE id = $1`, da1.ID).Scan(&doneAgain); err != nil {
		t.Fatal(err)
	}
	if !doneAgain.Equal(doneAt) {
		t.Fatalf("done_at moved from %v to %v", doneAt, doneAgain)
	}
	// A failure reported after done changes nothing.
	if err := store.RecordWorkspaceDeletionFailure(ctxA, da1.ID, "late"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("failure after done: %v", err)
	}
	pending, err = store.ListPendingWorkspaceDeletions(ctx, 10*time.Minute, 10)
	if err != nil || len(pending) != 1 || pending[0].ID != db.ID {
		t.Fatalf("pending after done = %+v, %v", pending, err)
	}
	if err := store.MarkWorkspaceDeletionDone(ctxA, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown deletion: %v", err)
	}

	// Only tool UIDs of the tenant range are stored.
	if _, err := pool.Exec(ctx, `UPDATE workspace_deletions SET tool_uid = 10002 WHERE id = $1`, db.ID); err == nil {
		t.Fatal("a tool_uid outside 20000-29999 was stored")
	}
}

// TestWorkspaceDeletionsMigrationDown: migration 121 rolls back cleanly.
func TestWorkspaceDeletionsMigrationDown(t *testing.T) {
	ctx := context.Background()
	dsn := scratchDatabase(t)
	if err := postgres.RunMigrations(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	if err := postgres.RollbackMigrations(ctx, dsn, migrationsAfter(t, 120)); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('workspace_deletions') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("workspace_deletions survived the down migration")
	}
}

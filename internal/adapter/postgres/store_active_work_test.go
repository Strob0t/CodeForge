package postgres_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/task"
)

// TestReleaseStaleWork_IntentionallyCrossTenant verifies that ReleaseStaleWork
// is documented as an intentionally cross-tenant system-level operation.
// This test guards against accidental regression of the documentation.
func TestReleaseStaleWork_IntentionallyCrossTenant(t *testing.T) {
	const expectedComment = "INTENTIONALLY CROSS-TENANT"
	src := readSourceFile(t, "store_active_work.go")
	if !strings.Contains(src, expectedComment) {
		t.Fatalf("ReleaseStaleWork must contain %q comment documenting it as intentionally cross-tenant", expectedComment)
	}
}

// TestStore_ReleaseStaleWork_ReturnsTenant verifies that released tasks carry
// their tenant: the release spans all tenants, and the caller needs the
// tenant to send each "released" WebSocket event to the right tenant only.
func TestStore_ReleaseStaleWork_ReturnsTenant(t *testing.T) {
	store := setupStore(t)
	tenantID := createTestTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	proj, err := store.CreateProject(ctx, &project.CreateRequest{Name: "stale-work-tenant", Provider: "local"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteProject(ctx, proj.ID) })
	tsk, err := store.CreateTask(ctx, task.CreateRequest{ProjectID: proj.ID, Title: "stale", Prompt: "p"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Age the task far beyond any real task so the release touches only it
	// (the shared test database may hold other tenants' running tasks). The
	// updated_at trigger is bypassed for this transaction only.
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
		t.Skipf("cannot bypass the updated_at trigger: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE tasks SET status = 'running', updated_at = NOW() - interval '100 years' WHERE id = $1`, tsk.ID); err != nil {
		t.Fatalf("age task: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	released, err := store.ReleaseStaleWork(context.Background(), 99*365*24*time.Hour)
	if err != nil {
		t.Fatalf("ReleaseStaleWork: %v", err)
	}
	for i := range released {
		if released[i].ID != tsk.ID {
			continue
		}
		if released[i].TenantID != tenantID {
			t.Fatalf("released task tenant = %q, want %q", released[i].TenantID, tenantID)
		}
		if released[i].Status != task.StatusPending {
			t.Fatalf("released task status = %q, want pending", released[i].Status)
		}
		return
	}
	t.Fatalf("task %s was not released", tsk.ID)
}

// readSourceFile reads a Go source file from the adapter/postgres package.
func readSourceFile(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location")
	}
	dir := filepath.Dir(thisFile)
	data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // test reads from known package dir
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

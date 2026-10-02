//go:build integration

package integration_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/domain/tenant"
)

// toolUIDDatabase returns a store and pool on a scratch database with every
// migration applied: the tool UID sequence is global, and these tests move it.
func toolUIDDatabase(t *testing.T) (*postgres.Store, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	dsn := scratchDatabase(t)
	if err := postgres.RunMigrations(ctx, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return postgres.NewStore(pool), pool
}

func insertTenant(t *testing.T, pool *pgxpool.Pool, slug string, created time.Time) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, slug, created_at) VALUES ($1, $1, $2) RETURNING id`, slug, created,
	).Scan(&id); err != nil {
		t.Fatalf("insert tenant %s: %v", slug, err)
	}
	return id
}

func toolUIDOf(t *testing.T, pool *pgxpool.Pool, tenantID string) *int32 {
	t.Helper()
	var uid *int32
	if err := pool.QueryRow(context.Background(), `SELECT tool_uid FROM tenants WHERE id = $1`, tenantID).Scan(&uid); err != nil {
		t.Fatalf("read tool_uid: %v", err)
	}
	return uid
}

// TestToolUIDBackfill: migration 120 numbers only the tenants that have
// projects (they may have tenant directories to migrate), in creation order.
func TestToolUIDBackfill(t *testing.T) {
	ctx := context.Background()
	dsn := scratchDatabase(t)
	if err := postgres.RunMigrations(ctx, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := postgres.RollbackMigrations(ctx, dsn, 1); err != nil {
		t.Fatalf("roll back 120: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	second := insertTenant(t, pool, "second", base.Add(2*time.Hour))
	first := insertTenant(t, pool, "first", base.Add(time.Hour))
	idle := insertTenant(t, pool, "idle", base)
	for _, id := range []string{first, second, second} {
		if _, err := pool.Exec(ctx, `INSERT INTO projects (name, tenant_id) VALUES ('p', $1)`, id); err != nil {
			t.Fatal(err)
		}
	}

	if err := postgres.RunMigrations(ctx, dsn); err != nil {
		t.Fatalf("migrate 120: %v", err)
	}
	if uid := toolUIDOf(t, pool, first); uid == nil || *uid != 20000 {
		t.Fatalf("first: tool_uid %v, want 20000", uid)
	}
	if uid := toolUIDOf(t, pool, second); uid == nil || *uid != 20001 {
		t.Fatalf("second: tool_uid %v, want 20001", uid)
	}
	if uid := toolUIDOf(t, pool, idle); uid != nil {
		t.Fatalf("a tenant without projects got tool_uid %d", *uid)
	}
	store := postgres.NewStore(pool)
	if uid, err := store.AllocateToolUID(ctx, idle); err != nil || uid != 20002 {
		t.Fatalf("next allocation: uid %d, err %v, want 20002", uid, err)
	}
}

// TestAllocateToolUIDConcurrently: two first allocations for one tenant get
// one UID and leave no gap in the sequence.
func TestAllocateToolUIDConcurrently(t *testing.T) {
	store, pool := toolUIDDatabase(t)
	ctx := context.Background()
	a := insertTenant(t, pool, "a", time.Now())
	b := insertTenant(t, pool, "b", time.Now())

	var wg sync.WaitGroup
	uids := make([]int, 8)
	errs := make([]error, 8)
	for i := range uids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			uids[i], errs[i] = store.AllocateToolUID(ctx, a)
		}(i)
	}
	wg.Wait()
	for i := range uids {
		if errs[i] != nil || uids[i] != 20000 {
			t.Fatalf("allocation %d: uid %d, err %v", i, uids[i], errs[i])
		}
	}
	if uid, err := store.AllocateToolUID(ctx, b); err != nil || uid != 20001 {
		t.Fatalf("second tenant: uid %d, err %v, want 20001 (no gap)", uid, err)
	}
	got, err := store.GetTenant(ctx, a)
	if err != nil || got.ToolUID == nil || *got.ToolUID != 20000 {
		t.Fatalf("GetTenant: %+v, %v", got, err)
	}
	if _, err := store.AllocateToolUID(ctx, "00000000-0000-0000-0000-00000000dead"); err == nil {
		t.Fatal("a missing tenant got a UID")
	}
}

// TestToolUIDConstraints: NULL -> value is allowed once; a set value never
// changes; values are unique and inside 20000-29999.
func TestToolUIDConstraints(t *testing.T) {
	_, pool := toolUIDDatabase(t)
	ctx := context.Background()
	a := insertTenant(t, pool, "a", time.Now())
	b := insertTenant(t, pool, "b", time.Now())

	if _, err := pool.Exec(ctx, `UPDATE tenants SET tool_uid = 20005 WHERE id = $1`, a); err != nil {
		t.Fatalf("NULL to a value: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE tenants SET tool_uid = 20006 WHERE id = $1`, a); err == nil {
		t.Fatal("a set tool_uid changed")
	}
	if _, err := pool.Exec(ctx, `UPDATE tenants SET tool_uid = NULL WHERE id = $1`, a); err == nil {
		t.Fatal("a set tool_uid was cleared")
	}
	if _, err := pool.Exec(ctx, `UPDATE tenants SET tool_uid = 20005 WHERE id = $1`, b); err == nil {
		t.Fatal("two tenants share a tool_uid")
	}
	for _, uid := range []int{19999, 30000, 10002} {
		if _, err := pool.Exec(ctx, `UPDATE tenants SET tool_uid = $2 WHERE id = $1`, b, uid); err == nil {
			t.Fatalf("tool_uid %d outside the range was stored", uid)
		}
	}
}

// TestToolUIDExhaustion: past 29999 the allocation fails with
// ErrToolUIDRangeExhausted and leaves the tenant without a UID.
func TestToolUIDExhaustion(t *testing.T) {
	store, pool := toolUIDDatabase(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `SELECT setval('tenant_tool_uid_seq', 29998, true)`); err != nil {
		t.Fatal(err)
	}
	a := insertTenant(t, pool, "a", time.Now())
	b := insertTenant(t, pool, "b", time.Now())
	if uid, err := store.AllocateToolUID(ctx, a); err != nil || uid != 29999 {
		t.Fatalf("last UID: %d, %v", uid, err)
	}
	if _, err := store.AllocateToolUID(ctx, b); !errors.Is(err, tenant.ErrToolUIDRangeExhausted) {
		t.Fatalf("exhausted: err = %v", err)
	}
	if uid := toolUIDOf(t, pool, b); uid != nil {
		t.Fatalf("the failed allocation stored %d", *uid)
	}
}

// TestAdvanceToolUIDSequence: after a restore older than the workspaces
// volume, the sequence moves past the UIDs the volume binds.
func TestAdvanceToolUIDSequence(t *testing.T) {
	store, pool := toolUIDDatabase(t)
	ctx := context.Background()
	if moved, err := store.AdvanceToolUIDSequence(ctx, 0); moved || err != nil {
		t.Fatalf("no bindings: moved %v, err %v", moved, err)
	}
	if moved, err := store.AdvanceToolUIDSequence(ctx, 20004); !moved || err != nil {
		t.Fatalf("advance: moved %v, err %v", moved, err)
	}
	if moved, err := store.AdvanceToolUIDSequence(ctx, 20002); moved || err != nil {
		t.Fatalf("below the sequence: moved %v, err %v", moved, err)
	}
	a := insertTenant(t, pool, "a", time.Now())
	if uid, err := store.AllocateToolUID(ctx, a); err != nil || uid != 20005 {
		t.Fatalf("after the advance: uid %d, err %v, want 20005", uid, err)
	}
}

// resetToolUIDSequence keeps the shared test database from running out of
// tool UIDs: tests create and delete tenants on every run. It sets the
// sequence to the highest tool_uid still in use (or back to its start).
func resetToolUIDSequence(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `SELECT setval('tenant_tool_uid_seq',
		COALESCE((SELECT max(tool_uid) FROM tenants), 20000),
		EXISTS (SELECT 1 FROM tenants WHERE tool_uid IS NOT NULL))`)
	return err
}

func TestResetToolUIDSequence(t *testing.T) {
	store, pool := toolUIDDatabase(t)
	ctx := context.Background()
	a := insertTenant(t, pool, "a", time.Now())
	b := insertTenant(t, pool, "b", time.Now())
	if _, err := store.AllocateToolUID(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AllocateToolUID(ctx, b); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, b); err != nil {
		t.Fatal(err)
	}
	if err := resetToolUIDSequence(ctx, pool); err != nil {
		t.Fatal(err)
	}
	c := insertTenant(t, pool, "c", time.Now())
	if uid, err := store.AllocateToolUID(ctx, c); err != nil || uid != 20001 {
		t.Fatalf("after the reset: uid %d, err %v, want 20001", uid, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE tenants SET tool_uid = NULL`); err == nil {
		t.Fatal("cleared set UIDs")
	}
}

// TestListAdoptedWorkspaces lists, across tenants, the projects whose
// workspace lies outside the workspace root (the startup check of adopted
// workspaces, KI-96).
func TestListAdoptedWorkspaces(t *testing.T) {
	store, pool := toolUIDDatabase(t)
	ctx := context.Background()
	a := insertTenant(t, pool, "a", time.Now())
	b := insertTenant(t, pool, "b", time.Now())
	for _, row := range []struct{ tenant, path string }{
		{a, "/data/workspaces/" + a + "/p1"},
		{a, "/srv/adopted/one"},
		{b, "/srv/adopted/two"},
		{b, ""},
		{b, "/data/workspaces"},
		{b, "/data/workspaces-elsewhere/x"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO projects (name, tenant_id, workspace_path) VALUES ('p', $1, $2)`, row.tenant, row.path); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.ListAdoptedWorkspaces(ctx, "/data/workspaces")
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{}
	for _, p := range got {
		paths[p.WorkspacePath] = p.TenantID
	}
	want := map[string]string{"/srv/adopted/one": a, "/srv/adopted/two": b, "/data/workspaces-elsewhere/x": b}
	if len(paths) != len(want) {
		t.Fatalf("adopted = %v, want %v", paths, want)
	}
	for path, tenantID := range want {
		if paths[path] != tenantID {
			t.Fatalf("adopted = %v, want %v", paths, want)
		}
	}
}

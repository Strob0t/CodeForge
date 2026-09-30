package postgres_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// ListStaleRuns feeds the stuck-work watchdog (KI-28): it spans all tenants
// and returns the runs of one status that were not updated for a while.

func TestListStaleRuns_IntentionallyCrossTenant(t *testing.T) {
	if src := readSourceFile(t, "store_run.go"); !strings.Contains(src, "INTENTIONALLY CROSS-TENANT") {
		t.Fatal("ListStaleRuns must document that it is intentionally cross-tenant")
	}
}

// ageRun sets a run's last update far into the past, beyond any real run,
// so the query touches only the test's runs in the shared database.
func ageRun(t *testing.T, id string) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `UPDATE runs SET updated_at = NOW() - interval '100 years' WHERE id = $1`, id); err != nil {
		t.Fatalf("age run: %v", err)
	}
}

func TestStore_ListStaleRuns(t *testing.T) {
	f := newStatusFixture(t)
	stuck := f.runIn(t, run.StatusQualityGate)
	ageRun(t, stuck.ID)
	fresh := f.runIn(t, run.StatusQualityGate)
	oldRunning := f.runIn(t, run.StatusRunning)
	ageRun(t, oldRunning.ID)
	oldDone := f.runIn(t, run.StatusCompleted)
	ageRun(t, oldDone.ID)

	// No tenant in the context: the watchdog sweeps every tenant.
	stale, err := f.store.ListStaleRuns(context.Background(), run.StatusQualityGate, 99*365*24*time.Hour, 1000)
	if err != nil {
		t.Fatalf("ListStaleRuns: %v", err)
	}
	found := map[string]run.Run{}
	for i := range stale {
		found[stale[i].ID] = stale[i]
	}
	got, ok := found[stuck.ID]
	if !ok {
		t.Fatalf("stuck quality_gate run %s not listed", stuck.ID)
	}
	if got.TenantID == "" || got.Status != run.StatusQualityGate || got.ProjectID != f.project.ID {
		t.Fatalf("listed run = %+v, want its tenant, status and project", got)
	}
	for _, id := range []string{fresh.ID, oldRunning.ID, oldDone.ID} {
		if _, listed := found[id]; listed {
			t.Errorf("run %s listed, want only stale quality_gate runs", id)
		}
	}

	limited, err := f.store.ListStaleRuns(context.Background(), run.StatusQualityGate, 99*365*24*time.Hour, 1)
	if err != nil {
		t.Fatalf("ListStaleRuns with limit: %v", err)
	}
	if len(limited) > 1 {
		t.Fatalf("limit 1 returned %d runs", len(limited))
	}
}

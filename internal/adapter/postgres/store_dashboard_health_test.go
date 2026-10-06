package postgres_test

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/dashboard"
	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// KI-129: a new project without runs scored 35 from its neutral factors and
// was rated critical; without runs in the last 7 days its level is unknown.
func TestProjectHealth_NewProjectIsUnknown(t *testing.T) {
	store := setupStore(t)
	ctx := ctxWithTenant(t, createTestTenant(t, store))
	proj, err := store.CreateProject(ctx, &project.CreateRequest{Name: "health-new", Provider: "local"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	ph, err := store.ProjectHealth(ctx, proj.ID)
	if err != nil {
		t.Fatalf("ProjectHealth: %v", err)
	}
	if ph.Level != dashboard.HealthUnknown {
		t.Errorf("level = %q (score %d), want %q", ph.Level, ph.Score, dashboard.HealthUnknown)
	}
	if ph.Stats.TotalRuns7d != 0 {
		t.Errorf("runs in 7 days = %d, want 0", ph.Stats.TotalRuns7d)
	}
}

package postgres_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// KI-33: ListEndedTeams feeds the stuck-work watchdog: across tenants, the
// teams that have not ended although all their plans have.

func TestListEndedTeams_IntentionallyCrossTenant(t *testing.T) {
	src := readStoreSource(t, "store_team.go")
	if doc := methodDocComment(t, src, "store_team.go", "ListEndedTeams"); !strings.Contains(doc, "INTENTIONALLY CROSS-TENANT") {
		t.Fatal("ListEndedTeams must document why it is intentionally cross-tenant")
	}
}

func (f *statusFixture) team(t *testing.T) *agent.Team {
	t.Helper()
	team, err := f.store.CreateTeam(f.ctx, agent.CreateTeamRequest{
		ProjectID: f.project.ID, Name: "ended-teams", Protocol: "sequential",
		Members: []agent.CreateMemberRequest{{AgentID: f.agent.ID, Role: agent.RoleCoder}},
	})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	return team
}

// teamPlan creates a plan of the team and puts it in status.
func (f *statusFixture) teamPlan(t *testing.T, teamID string, status plan.Status) {
	t.Helper()
	p := &plan.ExecutionPlan{
		ProjectID: f.project.ID, TeamID: teamID, Name: "ended-teams", Protocol: plan.ProtocolSequential,
		Status: plan.StatusPending, MaxParallel: 1,
		Steps: []plan.Step{{TaskID: f.task.ID, AgentID: f.agent.ID, Status: plan.StepStatusPending}},
	}
	if err := f.store.CreatePlan(f.ctx, p); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	pool := retentionPool(t)
	if _, err := pool.Exec(context.Background(), `UPDATE execution_plans SET status = $2 WHERE id = $1`, p.ID, string(status)); err != nil {
		t.Fatalf("set plan status: %v", err)
	}
}

func TestStore_ListEndedTeams(t *testing.T) {
	a, b := newStatusFixture(t), newStatusFixture(t)

	done, cancelled, running, noPlans, alreadyEnded := a.team(t), a.team(t), a.team(t), a.team(t), a.team(t)
	a.teamPlan(t, done.ID, plan.StatusCompleted)
	a.teamPlan(t, done.ID, plan.StatusCompleted)
	a.teamPlan(t, cancelled.ID, plan.StatusCompleted)
	a.teamPlan(t, cancelled.ID, plan.StatusCancelled)
	a.teamPlan(t, running.ID, plan.StatusCompleted)
	a.teamPlan(t, running.ID, plan.StatusRunning)
	a.teamPlan(t, alreadyEnded.ID, plan.StatusCompleted)
	if err := a.store.UpdateTeamStatus(a.ctx, alreadyEnded.ID, agent.TeamStatusCompleted); err != nil {
		t.Fatalf("UpdateTeamStatus: %v", err)
	}
	otherTenant := b.team(t)
	b.teamPlan(t, otherTenant.ID, plan.StatusFailed)

	ended, err := a.store.ListEndedTeams(context.Background(), 10_000) // the system job runs without a tenant
	if err != nil {
		t.Fatalf("ListEndedTeams: %v", err)
	}
	got := map[string]database.EndedTeam{}
	for _, e := range ended {
		got[e.ID] = e
	}

	tenantA, tenantB := middleware.TenantIDFromContext(a.ctx), middleware.TenantIDFromContext(b.ctx)
	want := map[string]database.EndedTeam{
		done.ID:        {ID: done.ID, TenantID: tenantA, Failed: false},
		cancelled.ID:   {ID: cancelled.ID, TenantID: tenantA, Failed: true},
		otherTenant.ID: {ID: otherTenant.ID, TenantID: tenantB, Failed: true},
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("team %s = %+v, want %+v", id, got[id], w)
		}
	}
	for name, id := range map[string]string{"running plan": running.ID, "no plans": noPlans.ID, "already ended": alreadyEnded.ID} {
		if _, listed := got[id]; listed {
			t.Errorf("team with %s listed as ended", name)
		}
	}
}

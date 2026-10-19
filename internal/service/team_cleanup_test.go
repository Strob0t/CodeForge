package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-33: a team ends with its plan (completed or failed); teams whose plans
// all ended otherwise (cancelled, or while Go Core was down) are ended by the
// stuck-work watchdog, in their tenant.

func newTeamPlanSetup(t *testing.T) (*orchMockStore, *service.OrchestratorService, *service.PoolManagerService) {
	t.Helper()
	store, orchSvc, _ := newOrchRuntimeSetup()
	pool := service.NewPoolManagerService(store, &runtimeMockBroadcaster{}, &config.Orchestrator{MaxTeamSize: 5})
	orchSvc.AddOnPlanComplete(pool.PlanEnded)
	return store, orchSvc, pool
}

func newTeam(t *testing.T, pool *service.PoolManagerService, agentIDs ...string) *agent.Team {
	t.Helper()
	var members []agent.CreateMemberRequest
	for _, id := range agentIDs {
		members = append(members, agent.CreateMemberRequest{AgentID: id, Role: agent.RoleCoder})
	}
	team, err := pool.CreateTeam(context.Background(), &agent.CreateTeamRequest{
		ProjectID: "proj-1", Name: "team", Protocol: "sequential", Members: members,
	})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	return team
}

func teamPlan(t *testing.T, orchSvc *service.OrchestratorService, teamID, taskID, agentID string) *plan.ExecutionPlan {
	t.Helper()
	ctx := context.Background()
	p, err := orchSvc.CreatePlan(ctx, &plan.CreatePlanRequest{
		Name: "team plan", ProjectID: "proj-1", TeamID: teamID, Protocol: plan.ProtocolSequential,
		Steps: []plan.CreateStepRequest{{TaskID: taskID, AgentID: agentID}},
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	if _, err := orchSvc.StartPlan(ctx, p.ID); err != nil {
		t.Fatalf("StartPlan: %v", err)
	}
	return p
}

func teamStatus(t *testing.T, store *orchMockStore, id string) agent.TeamStatus {
	t.Helper()
	team, err := store.GetTeam(context.Background(), id)
	if err != nil {
		t.Fatalf("GetTeam: %v", err)
	}
	return team.Status
}

func TestTeamEndsWithItsPlan(t *testing.T) {
	for _, tt := range []struct {
		runStatus run.Status
		want      agent.TeamStatus
	}{
		{run.StatusCompleted, agent.TeamStatusCompleted},
		{run.StatusFailed, agent.TeamStatusFailed},
	} {
		t.Run(string(tt.runStatus), func(t *testing.T) {
			store, orchSvc, pool := newTeamPlanSetup(t)
			team := newTeam(t, pool, "a1")
			p := teamPlan(t, orchSvc, team.ID, "t1", "a1")
			if got := teamStatus(t, store, team.ID); got.IsTerminal() {
				t.Fatalf("team %s while its plan runs", got)
			}

			endFirstRun(t, store, orchSvc, p.ID, tt.runStatus, "")

			if got := teamStatus(t, store, team.ID); got != tt.want {
				t.Fatalf("team = %s after the plan ended, want %s", got, tt.want)
			}
		})
	}
}

// A team with another plan still running goes on.
func TestTeamWithARunningPlanGoesOn(t *testing.T) {
	store, orchSvc, pool := newTeamPlanSetup(t)
	team := newTeam(t, pool, "a1", "a2")
	first := teamPlan(t, orchSvc, team.ID, "t1", "a1")
	teamPlan(t, orchSvc, team.ID, "t2", "a2")

	endFirstRun(t, store, orchSvc, first.ID, run.StatusCompleted, "")

	if got := teamStatus(t, store, team.ID); got.IsTerminal() {
		t.Fatalf("team = %s while its second plan runs", got)
	}
}

// Ending a team leaves its members' status to their runs: a member may run
// work of another team or plan by now (CreateTeam reserves no agent).
func TestCleanupTeam_LeavesMembersToTheirRuns(t *testing.T) {
	svc, store := newPoolManagerTestEnv()
	ctx := context.Background()
	team, err := svc.CreateTeam(ctx, &agent.CreateTeamRequest{
		ProjectID: "proj-1", Name: "busy member", Protocol: "sequential",
		Members: []agent.CreateMemberRequest{{AgentID: "a1", Role: agent.RoleCoder}},
	})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if err := store.UpdateAgentStatus(ctx, "a1", agent.StatusRunning); err != nil {
		t.Fatalf("UpdateAgentStatus: %v", err)
	}

	if err := svc.CleanupTeam(ctx, team.ID, false); err != nil {
		t.Fatalf("CleanupTeam: %v", err)
	}
	if got, _ := store.GetTeam(ctx, team.ID); got.Status != agent.TeamStatusCompleted {
		t.Fatalf("team = %s, want completed", got.Status)
	}
	if ag, _ := store.GetAgent(ctx, "a1"); ag.Status != agent.StatusRunning {
		t.Fatalf("running member = %s after the cleanup, want it still running", ag.Status)
	}
}

// endedTeams lists teams as the store would, recording the call.
type endedTeams struct {
	teams []database.EndedTeam
	err   error
	limit int
}

func (e *endedTeams) ListEndedTeams(_ context.Context, limit int) ([]database.EndedTeam, error) {
	e.limit = limit
	return e.teams, e.err
}

func TestCleanupEndedTeams(t *testing.T) {
	svc, store := newPoolManagerTestEnv()
	ctx := context.Background()
	cancelled := newTeamInTenant(t, svc, "a1")
	completed := newTeamInTenant(t, svc, "a2")

	lister := &endedTeams{teams: []database.EndedTeam{
		{ID: cancelled.ID, TenantID: tenantctx.DefaultTenantID, Failed: true},
		{ID: completed.ID, TenantID: tenantctx.DefaultTenantID},
		{ID: "gone", TenantID: tenantctx.DefaultTenantID}, // deleted meanwhile
	}}
	n, err := svc.CleanupEndedTeams(ctx, lister)
	if err != nil || n != 2 || lister.limit <= 0 {
		t.Fatalf("CleanupEndedTeams = %d, %v (limit %d), want 2 teams ended", n, err, lister.limit)
	}
	if got, _ := store.GetTeam(ctx, cancelled.ID); got.Status != agent.TeamStatusFailed {
		t.Errorf("team of a cancelled plan = %s, want failed", got.Status)
	}
	if got, _ := store.GetTeam(ctx, completed.ID); got.Status != agent.TeamStatusCompleted {
		t.Errorf("team of a completed plan = %s, want completed", got.Status)
	}

	if _, err := svc.CleanupEndedTeams(ctx, &endedTeams{err: errors.New("db down")}); err == nil {
		t.Fatal("a failing lookup is not reported")
	}
}

func newTeamInTenant(t *testing.T, svc *service.PoolManagerService, agentID string) *agent.Team {
	t.Helper()
	team, err := svc.CreateTeam(context.Background(), &agent.CreateTeamRequest{
		ProjectID: "proj-1", Name: "team " + agentID, Protocol: "sequential",
		Members: []agent.CreateMemberRequest{{AgentID: agentID, Role: agent.RoleCoder}},
	})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	return team
}

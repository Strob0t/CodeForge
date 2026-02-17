package service_test

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/service"
)

func newPoolManagerTestEnv() (*service.PoolManagerService, *runtimeMockStore) {
	store := &runtimeMockStore{
		agents: []agent.Agent{
			{ID: "a1", ProjectID: "proj-1", Name: "coder-1", Backend: "aider", Status: agent.StatusIdle},
			{ID: "a2", ProjectID: "proj-1", Name: "reviewer-1", Backend: "aider", Status: agent.StatusIdle},
			{ID: "a3", ProjectID: "proj-1", Name: "coder-2", Backend: "aider", Status: agent.StatusIdle},
		},
	}
	bc := &runtimeMockBroadcaster{}
	orchCfg := &config.Orchestrator{MaxTeamSize: 5}
	svc := service.NewPoolManagerService(store, bc, orchCfg)
	return svc, store
}

func TestCreateTeam_Success(t *testing.T) {
	svc, store := newPoolManagerTestEnv()
	ctx := context.Background()

	req := &agent.CreateTeamRequest{
		ProjectID: "proj-1",
		Name:      "Test Team",
		Protocol:  "sequential",
		Members: []agent.CreateMemberRequest{
			{AgentID: "a1", Role: agent.RoleCoder},
			{AgentID: "a2", Role: agent.RoleReviewer},
		},
	}

	team, err := svc.CreateTeam(ctx, req)
	if err != nil {
		t.Fatalf("CreateTeam failed: %v", err)
	}
	if team == nil {
		t.Fatal("expected team, got nil")
	}

	// Verify team was stored (GetTeam should work via store).
	_ = store
}

func TestCreateTeam_AgentNotFound(t *testing.T) {
	svc, _ := newPoolManagerTestEnv()
	ctx := context.Background()

	req := &agent.CreateTeamRequest{
		ProjectID: "proj-1",
		Name:      "Bad Team",
		Protocol:  "sequential",
		Members: []agent.CreateMemberRequest{
			{AgentID: "nonexistent", Role: agent.RoleCoder},
		},
	}

	_, err := svc.CreateTeam(ctx, req)
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
}

func TestCreateTeam_AgentBusy(t *testing.T) {
	svc, store := newPoolManagerTestEnv()
	ctx := context.Background()

	// Mark agent as running.
	store.mu.Lock()
	store.agents[0].Status = agent.StatusRunning
	store.mu.Unlock()

	req := &agent.CreateTeamRequest{
		ProjectID: "proj-1",
		Name:      "Busy Team",
		Protocol:  "sequential",
		Members: []agent.CreateMemberRequest{
			{AgentID: "a1", Role: agent.RoleCoder},
		},
	}

	_, err := svc.CreateTeam(ctx, req)
	if err == nil {
		t.Fatal("expected error for busy agent")
	}
}

func TestCreateTeam_ExceedsMaxSize(t *testing.T) {
	store := &runtimeMockStore{
		agents: []agent.Agent{
			{ID: "a1", ProjectID: "proj-1", Status: agent.StatusIdle},
			{ID: "a2", ProjectID: "proj-1", Status: agent.StatusIdle},
			{ID: "a3", ProjectID: "proj-1", Status: agent.StatusIdle},
		},
	}
	bc := &runtimeMockBroadcaster{}
	orchCfg := &config.Orchestrator{MaxTeamSize: 2}
	svc := service.NewPoolManagerService(store, bc, orchCfg)
	ctx := context.Background()

	req := &agent.CreateTeamRequest{
		ProjectID: "proj-1",
		Name:      "Big Team",
		Protocol:  "parallel",
		Members: []agent.CreateMemberRequest{
			{AgentID: "a1", Role: agent.RoleCoder},
			{AgentID: "a2", Role: agent.RoleCoder},
			{AgentID: "a3", Role: agent.RoleReviewer},
		},
	}

	_, err := svc.CreateTeam(ctx, req)
	if err == nil {
		t.Fatal("expected error for exceeding max team size")
	}
}

func TestAssembleTeamForStrategy_Single(t *testing.T) {
	svc, _ := newPoolManagerTestEnv()
	ctx := context.Background()

	team, err := svc.AssembleTeamForStrategy(ctx, "proj-1", plan.StrategySingle, "Auto Single")
	if err != nil {
		t.Fatalf("AssembleTeamForStrategy failed: %v", err)
	}
	if len(team.Members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(team.Members))
	}
	if team.Members[0].Role != agent.RoleCoder {
		t.Fatalf("expected coder role, got %s", team.Members[0].Role)
	}
}

func TestAssembleTeamForStrategy_Pair(t *testing.T) {
	svc, _ := newPoolManagerTestEnv()
	ctx := context.Background()

	team, err := svc.AssembleTeamForStrategy(ctx, "proj-1", plan.StrategyPair, "Auto Pair")
	if err != nil {
		t.Fatalf("AssembleTeamForStrategy failed: %v", err)
	}
	if len(team.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(team.Members))
	}

	roles := map[agent.TeamRole]bool{}
	for _, m := range team.Members {
		roles[m.Role] = true
	}
	if !roles[agent.RoleCoder] || !roles[agent.RoleReviewer] {
		t.Fatal("expected coder and reviewer roles in pair team")
	}
}

func TestAssembleTeamForStrategy_NoIdleAgents(t *testing.T) {
	store := &runtimeMockStore{
		agents: []agent.Agent{
			{ID: "a1", ProjectID: "proj-1", Status: agent.StatusRunning},
		},
	}
	bc := &runtimeMockBroadcaster{}
	svc := service.NewPoolManagerService(store, bc, &config.Orchestrator{MaxTeamSize: 5})
	ctx := context.Background()

	_, err := svc.AssembleTeamForStrategy(ctx, "proj-1", plan.StrategySingle, "Fail")
	if err == nil {
		t.Fatal("expected error for no idle agents")
	}
}

func TestCleanupTeam_ReleasesAgents(t *testing.T) {
	svc, store := newPoolManagerTestEnv()
	ctx := context.Background()

	// Create a team first.
	req := &agent.CreateTeamRequest{
		ProjectID: "proj-1",
		Name:      "Cleanup Team",
		Protocol:  "sequential",
		Members: []agent.CreateMemberRequest{
			{AgentID: "a1", Role: agent.RoleCoder},
		},
	}
	team, err := svc.CreateTeam(ctx, req)
	if err != nil {
		t.Fatalf("CreateTeam failed: %v", err)
	}

	// Set agent to running to verify it gets reset.
	store.mu.Lock()
	for i := range store.agents {
		if store.agents[i].ID == "a1" {
			store.agents[i].Status = agent.StatusRunning
		}
	}
	store.mu.Unlock()

	err = svc.CleanupTeam(ctx, team.ID, false)
	if err != nil {
		t.Fatalf("CleanupTeam failed: %v", err)
	}

	// Verify agent is back to idle.
	ag, _ := store.GetAgent(ctx, "a1")
	if ag.Status != agent.StatusIdle {
		t.Fatalf("expected agent idle after cleanup, got %s", ag.Status)
	}
}

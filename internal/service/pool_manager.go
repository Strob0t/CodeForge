package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// PoolManagerService manages agent team lifecycle: creation, assembly, and cleanup.
type PoolManagerService struct {
	store     database.Store
	hub       broadcast.Broadcaster
	orchCfg   *config.Orchestrator
	sharedCtx *SharedContextService
}

// SetSharedContext sets the shared context service for auto-initializing team contexts.
func (s *PoolManagerService) SetSharedContext(sc *SharedContextService) {
	s.sharedCtx = sc
}

// NewPoolManagerService creates a new PoolManagerService.
func NewPoolManagerService(
	store database.Store,
	hub broadcast.Broadcaster,
	orchCfg *config.Orchestrator,
) *PoolManagerService {
	return &PoolManagerService{store: store, hub: hub, orchCfg: orchCfg}
}

// CreateTeam validates the request, verifies all agents exist and are idle,
// then persists the team in the store.
func (s *PoolManagerService) CreateTeam(ctx context.Context, req *agent.CreateTeamRequest) (*agent.Team, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate team request: %w", err)
	}

	if s.orchCfg != nil && s.orchCfg.MaxTeamSize > 0 && len(req.Members) > s.orchCfg.MaxTeamSize {
		return nil, fmt.Errorf("team size %d exceeds max_team_size %d", len(req.Members), s.orchCfg.MaxTeamSize)
	}

	// Verify all agents exist, belong to the project, and are idle.
	for _, m := range req.Members {
		ag, err := s.store.GetAgent(ctx, m.AgentID)
		if err != nil {
			return nil, fmt.Errorf("agent %s: %w", m.AgentID, err)
		}
		if ag.ProjectID != req.ProjectID {
			return nil, fmt.Errorf("agent %s belongs to project %s, not %s", m.AgentID, ag.ProjectID, req.ProjectID)
		}
		if ag.Status != agent.StatusIdle {
			return nil, fmt.Errorf("agent %s is %s, expected idle", m.AgentID, ag.Status)
		}
	}

	team, err := s.store.CreateTeam(ctx, *req)
	if err != nil {
		return nil, fmt.Errorf("create team: %w", err)
	}

	// Auto-initialize shared context for the team.
	if s.sharedCtx != nil {
		if _, err := s.sharedCtx.InitForTeam(ctx, team.ID, req.ProjectID); err != nil {
			slog.Warn("shared context init failed", "team_id", team.ID, "error", err)
		}
	}

	// Broadcast team status via WebSocket.
	s.hub.BroadcastEvent(ctx, event.EventTeamStatus, event.TeamStatusEvent{
		TeamID:    team.ID,
		ProjectID: req.ProjectID,
		Status:    string(team.Status),
		Name:      team.Name,
	})

	slog.Info("team created", "team_id", team.ID, "project_id", req.ProjectID, "members", len(req.Members))
	return team, nil
}

// AssembleTeamForStrategy automatically creates a team by selecting idle agents
// from the project and assigning roles based on the strategy.
func (s *PoolManagerService) AssembleTeamForStrategy(
	ctx context.Context,
	projectID string,
	strategy plan.AgentStrategy,
	teamName string,
) (*agent.Team, error) {
	agents, err := s.store.ListAgents(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}

	// Filter idle agents.
	var idle []agent.Agent
	for i := range agents {
		if agents[i].Status == agent.StatusIdle {
			idle = append(idle, agents[i])
		}
	}
	if len(idle) == 0 {
		return nil, errors.New("no idle agents available")
	}

	var members []agent.CreateMemberRequest

	switch strategy {
	case plan.StrategySingle:
		members = append(members, agent.CreateMemberRequest{
			AgentID: idle[0].ID,
			Role:    agent.RoleCoder,
		})

	case plan.StrategyPair:
		members = append(members, agent.CreateMemberRequest{
			AgentID: idle[0].ID,
			Role:    agent.RoleCoder,
		})
		if len(idle) >= 2 {
			members = append(members, agent.CreateMemberRequest{
				AgentID: idle[1].ID,
				Role:    agent.RoleReviewer,
			})
		}

	case plan.StrategyTeam:
		maxSize := 5
		if s.orchCfg != nil && s.orchCfg.MaxTeamSize > 0 {
			maxSize = s.orchCfg.MaxTeamSize
		}
		for i := range idle {
			if i >= maxSize {
				break
			}
			role := agent.RoleCoder
			// Last agent gets reviewer role if we have more than one.
			if i == len(idle)-1 && i > 0 {
				role = agent.RoleReviewer
			}
			members = append(members, agent.CreateMemberRequest{
				AgentID: idle[i].ID,
				Role:    role,
			})
		}

	default:
		// Unknown strategy: default to single.
		members = append(members, agent.CreateMemberRequest{
			AgentID: idle[0].ID,
			Role:    agent.RoleCoder,
		})
	}

	protocol := plan.StrategyToProtocol(strategy)
	req := &agent.CreateTeamRequest{
		ProjectID: projectID,
		Name:      teamName,
		Protocol:  string(protocol),
		Members:   members,
	}

	return s.CreateTeam(ctx, req)
}

// CleanupTeam marks a team as completed or failed. Its members' status is
// left to their runs: CreateTeam reserves no agent, every run resets its agent
// when it ends, and a member may run work of another team or plan by now.
func (s *PoolManagerService) CleanupTeam(ctx context.Context, teamID string, failed bool) error {
	status := agent.TeamStatusCompleted
	if failed {
		status = agent.TeamStatusFailed
	}

	if err := s.store.UpdateTeamStatus(ctx, teamID, status); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			// Cleaned up before (KI-31).
			slog.InfoContext(ctx, "team already ended, cleanup skipped", "team_id", teamID)
			return nil
		}
		return fmt.Errorf("update team status: %w", err)
	}

	slog.Info("team cleaned up", "team_id", teamID, "status", status)
	return nil
}

// PlanEnded is an orchestrator plan-end callback (AddOnPlanComplete, KI-33):
// the team of a plan that completed or failed ends with it, unless another
// plan of the team has not ended.
func (s *PoolManagerService) PlanEnded(ctx context.Context, planID, status string) {
	p, err := s.store.GetPlan(ctx, planID)
	if err != nil || p.TeamID == "" {
		return
	}
	plans, err := s.store.ListPlansByProject(ctx, p.ProjectID)
	if err != nil {
		slog.Warn("team not ended with its plan, the watchdog ends it later", "team_id", p.TeamID, "error", err)
		return
	}
	for i := range plans {
		if plans[i].TeamID == p.TeamID && !plans[i].Status.IsTerminal() {
			return
		}
	}
	logBestEffort(ctx, s.CleanupTeam(ctx, p.TeamID, status != string(plan.StatusCompleted)),
		"CleanupTeam", slog.String("team_id", p.TeamID))
}

// endedTeamBatch limits the teams one watchdog check ends; the rest follow
// at the next check.
const endedTeamBatch = 100

// endedTeamLister finds teams whose plans all ended, across tenants
// (postgres.Store.ListEndedTeams).
type endedTeamLister interface {
	ListEndedTeams(ctx context.Context, limit int) ([]database.EndedTeam, error)
}

// CleanupEndedTeams ends the teams whose plans all ended but that were not
// ended with them (a cancelled plan, or a plan that ended while Go Core was
// down), each in its own tenant, and returns how many it ended. It runs at
// startup and as a stuck-work watchdog check.
func (s *PoolManagerService) CleanupEndedTeams(ctx context.Context, teams endedTeamLister) (int, error) {
	ended, err := teams.ListEndedTeams(ctx, endedTeamBatch)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, t := range ended {
		tctx := tenantctx.WithTenant(ctx, t.TenantID)
		if err := s.CleanupTeam(tctx, t.ID, t.Failed); err != nil {
			slog.Warn("end team whose plans ended", "team_id", t.ID, "error", err)
			continue
		}
		n++
	}
	return n, nil
}

// GetTeam returns a team by ID.
func (s *PoolManagerService) GetTeam(ctx context.Context, id string) (*agent.Team, error) {
	return s.store.GetTeam(ctx, id)
}

// ListTeams returns all teams for a project.
func (s *PoolManagerService) ListTeams(ctx context.Context, projectID string) ([]agent.Team, error) {
	return s.store.ListTeamsByProject(ctx, projectID)
}

// DeleteTeam removes a team from the store.
func (s *PoolManagerService) DeleteTeam(ctx context.Context, id string) error {
	return s.store.DeleteTeam(ctx, id)
}

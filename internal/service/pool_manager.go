package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
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
	s.hub.BroadcastEvent(ctx, ws.EventTeamStatus, ws.TeamStatusEvent{
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

// CleanupTeam marks a team as completed or failed and releases its agents.
func (s *PoolManagerService) CleanupTeam(ctx context.Context, teamID string, failed bool) error {
	team, err := s.store.GetTeam(ctx, teamID)
	if err != nil {
		return fmt.Errorf("get team: %w", err)
	}

	status := agent.TeamStatusCompleted
	if failed {
		status = agent.TeamStatusFailed
	}

	if err := s.store.UpdateTeamStatus(ctx, teamID, status); err != nil {
		return fmt.Errorf("update team status: %w", err)
	}

	// Release all agents back to idle.
	for _, m := range team.Members {
		if err := s.store.UpdateAgentStatus(ctx, m.AgentID, agent.StatusIdle); err != nil {
			slog.Warn("failed to release agent", "agent_id", m.AgentID, "error", err)
		}
	}

	slog.Info("team cleaned up", "team_id", teamID, "status", status)
	return nil
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

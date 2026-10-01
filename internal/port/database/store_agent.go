package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/resource"
)

// AgentStore defines database operations for agents, teams, and agent identity.
type AgentStore interface {
	// Agents
	ListAgents(ctx context.Context, projectID string) ([]agent.Agent, error)
	GetAgent(ctx context.Context, id string) (*agent.Agent, error)
	CreateAgent(ctx context.Context, projectID, name, backend string, config map[string]string, limits *resource.Limits) (*agent.Agent, error)
	UpdateAgentStatus(ctx context.Context, id string, status agent.Status) error
	DeleteAgent(ctx context.Context, id string) error

	// Agent Teams
	CreateTeam(ctx context.Context, req agent.CreateTeamRequest) (*agent.Team, error)
	GetTeam(ctx context.Context, id string) (*agent.Team, error)
	ListTeamsByProject(ctx context.Context, projectID string) ([]agent.Team, error)
	UpdateTeamStatus(ctx context.Context, id string, status agent.TeamStatus) error
	DeleteTeam(ctx context.Context, id string) error

	// Agent Identity (Phase 23C)
	IncrementAgentStats(ctx context.Context, id string, costDelta float64, success bool) error
	UpdateAgentState(ctx context.Context, id string, state map[string]string) error
	SendAgentMessage(ctx context.Context, msg *agent.InboxMessage) error
	// ClaimHandoff records that the stage ("request", "approved") of the
	// handoff handoffID is being carried out in the caller's tenant and
	// reports whether this call claimed it: false when it was claimed
	// before (a redelivered message).
	ClaimHandoff(ctx context.Context, handoffID, stage string) (bool, error)
	// ReleaseHandoff removes a claim of ClaimHandoff, so a retry can claim
	// the stage again.
	ReleaseHandoff(ctx context.Context, handoffID, stage string) error
	ListAgentInbox(ctx context.Context, agentID string, unreadOnly bool) ([]agent.InboxMessage, error)
	MarkInboxRead(ctx context.Context, messageID string) error
}

// EndedTeam is a team that has not ended although every execution plan of it
// has (the stuck-work watchdog ends it). Failed: a plan did not complete.
type EndedTeam struct {
	ID       string
	TenantID string
	Failed   bool
}

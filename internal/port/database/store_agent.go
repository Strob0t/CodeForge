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
	ListAgentInbox(ctx context.Context, agentID string, unreadOnly bool) ([]agent.InboxMessage, error)
	MarkInboxRead(ctx context.Context, messageID string) error
}

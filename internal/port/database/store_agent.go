package database

import (
	"context"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/orchestration"
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
	// ClaimHandoff claims the stage ("request", "approved") of the handoff
	// handoffID in the caller's tenant: Claimed for a new claim, or for a
	// claim that was never done and is older than lease (its process may
	// have died); Done when the stage was carried out before; otherwise the
	// Age of the claim in progress.
	ClaimHandoff(ctx context.Context, handoffID, stage string, lease time.Duration) (orchestration.HandoffClaim, error)
	// FinishHandoff marks a claimed stage done: it is never claimed again.
	FinishHandoff(ctx context.Context, handoffID, stage string) error
	// ReleaseHandoff lets a retry claim the stage again at once; the claim
	// keeps its task.
	ReleaseHandoff(ctx context.Context, handoffID, stage string) error
	// SetHandoffTask records the task a claimed stage created for its run,
	// which a retry of the stage reuses (HandoffClaim.TaskID).
	SetHandoffTask(ctx context.Context, handoffID, stage, taskID string) error
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

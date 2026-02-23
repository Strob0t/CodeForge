// Package database defines the database store port (interface).
package database

import (
	"context"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/agent"
	bp "github.com/Strob0t/CodeForge/internal/domain/branchprotection"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/cost"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/resource"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/domain/tenant"
	"github.com/Strob0t/CodeForge/internal/domain/user"
)

// Store is the port interface for database operations.
type Store interface {
	// Projects
	ListProjects(ctx context.Context) ([]project.Project, error)
	GetProject(ctx context.Context, id string) (*project.Project, error)
	CreateProject(ctx context.Context, req project.CreateRequest) (*project.Project, error)
	UpdateProject(ctx context.Context, p *project.Project) error
	DeleteProject(ctx context.Context, id string) error

	// Agents
	ListAgents(ctx context.Context, projectID string) ([]agent.Agent, error)
	GetAgent(ctx context.Context, id string) (*agent.Agent, error)
	CreateAgent(ctx context.Context, projectID, name, backend string, config map[string]string, limits *resource.Limits) (*agent.Agent, error)
	UpdateAgentStatus(ctx context.Context, id string, status agent.Status) error
	DeleteAgent(ctx context.Context, id string) error

	// Tasks
	ListTasks(ctx context.Context, projectID string) ([]task.Task, error)
	GetTask(ctx context.Context, id string) (*task.Task, error)
	CreateTask(ctx context.Context, req task.CreateRequest) (*task.Task, error)
	UpdateTaskStatus(ctx context.Context, id string, status task.Status) error
	UpdateTaskResult(ctx context.Context, id string, result task.Result, costUSD float64) error

	// Runs
	CreateRun(ctx context.Context, r *run.Run) error
	GetRun(ctx context.Context, id string) (*run.Run, error)
	UpdateRunStatus(ctx context.Context, id string, status run.Status, stepCount int, costUSD float64, tokensIn, tokensOut int64) error
	CompleteRun(ctx context.Context, id string, status run.Status, output, errMsg string, costUSD float64, stepCount int, tokensIn, tokensOut int64, model string) error
	UpdateRunArtifact(ctx context.Context, id, artifactType string, valid *bool, errors []string) error
	ListRunsByTask(ctx context.Context, taskID string) ([]run.Run, error)

	// Agent Teams
	CreateTeam(ctx context.Context, req agent.CreateTeamRequest) (*agent.Team, error)
	GetTeam(ctx context.Context, id string) (*agent.Team, error)
	ListTeamsByProject(ctx context.Context, projectID string) ([]agent.Team, error)
	UpdateTeamStatus(ctx context.Context, id string, status agent.TeamStatus) error
	DeleteTeam(ctx context.Context, id string) error

	// Execution Plans
	CreatePlan(ctx context.Context, p *plan.ExecutionPlan) error
	GetPlan(ctx context.Context, id string) (*plan.ExecutionPlan, error)
	ListPlansByProject(ctx context.Context, projectID string) ([]plan.ExecutionPlan, error)
	UpdatePlanStatus(ctx context.Context, id string, status plan.Status) error
	CreatePlanStep(ctx context.Context, step *plan.Step) error
	ListPlanSteps(ctx context.Context, planID string) ([]plan.Step, error)
	UpdatePlanStepStatus(ctx context.Context, stepID string, status plan.StepStatus, runID string, errMsg string) error
	GetPlanStepByRunID(ctx context.Context, runID string) (*plan.Step, error)
	UpdatePlanStepRound(ctx context.Context, stepID string, round int) error

	// Context Packs
	CreateContextPack(ctx context.Context, pack *cfcontext.ContextPack) error
	GetContextPack(ctx context.Context, id string) (*cfcontext.ContextPack, error)
	GetContextPackByTask(ctx context.Context, taskID string) (*cfcontext.ContextPack, error)
	DeleteContextPack(ctx context.Context, id string) error

	// Shared Context
	CreateSharedContext(ctx context.Context, sc *cfcontext.SharedContext) error
	GetSharedContext(ctx context.Context, id string) (*cfcontext.SharedContext, error)
	GetSharedContextByTeam(ctx context.Context, teamID string) (*cfcontext.SharedContext, error)
	AddSharedContextItem(ctx context.Context, req cfcontext.AddSharedItemRequest) (*cfcontext.SharedContextItem, error)
	DeleteSharedContext(ctx context.Context, id string) error

	// Repo Maps
	UpsertRepoMap(ctx context.Context, m *cfcontext.RepoMap) error
	GetRepoMap(ctx context.Context, projectID string) (*cfcontext.RepoMap, error)
	DeleteRepoMap(ctx context.Context, projectID string) error

	// Cost Aggregation
	CostSummaryGlobal(ctx context.Context) ([]cost.ProjectSummary, error)
	CostSummaryByProject(ctx context.Context, projectID string) (*cost.Summary, error)
	CostByModel(ctx context.Context, projectID string) ([]cost.ModelSummary, error)
	CostTimeSeries(ctx context.Context, projectID string, days int) ([]cost.DailyCost, error)
	RecentRunsWithCost(ctx context.Context, projectID string, limit int) ([]run.Run, error)

	// Roadmaps
	CreateRoadmap(ctx context.Context, req roadmap.CreateRoadmapRequest) (*roadmap.Roadmap, error)
	GetRoadmap(ctx context.Context, id string) (*roadmap.Roadmap, error)
	GetRoadmapByProject(ctx context.Context, projectID string) (*roadmap.Roadmap, error)
	UpdateRoadmap(ctx context.Context, r *roadmap.Roadmap) error
	DeleteRoadmap(ctx context.Context, id string) error

	// Milestones
	CreateMilestone(ctx context.Context, req roadmap.CreateMilestoneRequest) (*roadmap.Milestone, error)
	GetMilestone(ctx context.Context, id string) (*roadmap.Milestone, error)
	ListMilestones(ctx context.Context, roadmapID string) ([]roadmap.Milestone, error)
	UpdateMilestone(ctx context.Context, m *roadmap.Milestone) error
	DeleteMilestone(ctx context.Context, id string) error

	// Features
	CreateFeature(ctx context.Context, req *roadmap.CreateFeatureRequest) (*roadmap.Feature, error)
	GetFeature(ctx context.Context, id string) (*roadmap.Feature, error)
	ListFeatures(ctx context.Context, milestoneID string) ([]roadmap.Feature, error)
	ListFeaturesByRoadmap(ctx context.Context, roadmapID string) ([]roadmap.Feature, error)
	UpdateFeature(ctx context.Context, f *roadmap.Feature) error
	DeleteFeature(ctx context.Context, id string) error

	// Tenants
	CreateTenant(ctx context.Context, req tenant.CreateRequest) (*tenant.Tenant, error)
	GetTenant(ctx context.Context, id string) (*tenant.Tenant, error)
	ListTenants(ctx context.Context) ([]tenant.Tenant, error)
	UpdateTenant(ctx context.Context, t *tenant.Tenant) error

	// Branch Protection Rules
	CreateBranchProtectionRule(ctx context.Context, req bp.CreateRuleRequest) (*bp.ProtectionRule, error)
	GetBranchProtectionRule(ctx context.Context, id string) (*bp.ProtectionRule, error)
	ListBranchProtectionRules(ctx context.Context, projectID string) ([]bp.ProtectionRule, error)
	UpdateBranchProtectionRule(ctx context.Context, rule *bp.ProtectionRule) error
	DeleteBranchProtectionRule(ctx context.Context, id string) error

	// Sessions
	CreateSession(ctx context.Context, s *run.Session) error
	GetSession(ctx context.Context, id string) (*run.Session, error)
	ListSessions(ctx context.Context, projectID string) ([]run.Session, error)
	UpdateSessionStatus(ctx context.Context, id string, status run.SessionStatus, currentRunID string) error

	// Users
	CreateUser(ctx context.Context, u *user.User) error
	GetUser(ctx context.Context, id string) (*user.User, error)
	GetUserByEmail(ctx context.Context, email, tenantID string) (*user.User, error)
	ListUsers(ctx context.Context, tenantID string) ([]user.User, error)
	UpdateUser(ctx context.Context, u *user.User) error
	DeleteUser(ctx context.Context, id string) error

	// Refresh Tokens
	CreateRefreshToken(ctx context.Context, rt *user.RefreshToken) error
	GetRefreshTokenByHash(ctx context.Context, tokenHash string) (*user.RefreshToken, error)
	DeleteRefreshToken(ctx context.Context, id string) error
	DeleteRefreshTokensByUser(ctx context.Context, userID string) error

	// API Keys
	CreateAPIKey(ctx context.Context, key *user.APIKey) error
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*user.APIKey, error)
	ListAPIKeysByUser(ctx context.Context, userID string) ([]user.APIKey, error)
	DeleteAPIKey(ctx context.Context, id string) error

	// Token Revocation
	RevokeToken(ctx context.Context, jti string, expiresAt time.Time) error
	IsTokenRevoked(ctx context.Context, jti string) (bool, error)
	PurgeExpiredTokens(ctx context.Context) (int64, error)

	// Atomic Refresh Token Rotation
	RotateRefreshToken(ctx context.Context, oldID string, newRT *user.RefreshToken) error

	// Retrieval Scopes
	CreateScope(ctx context.Context, req cfcontext.CreateScopeRequest) (*cfcontext.RetrievalScope, error)
	GetScope(ctx context.Context, id string) (*cfcontext.RetrievalScope, error)
	ListScopes(ctx context.Context) ([]cfcontext.RetrievalScope, error)
	UpdateScope(ctx context.Context, id string, req cfcontext.UpdateScopeRequest) (*cfcontext.RetrievalScope, error)
	DeleteScope(ctx context.Context, id string) error
	ListScopesByProject(ctx context.Context, projectID string) ([]cfcontext.RetrievalScope, error)
	AddProjectToScope(ctx context.Context, scopeID, projectID string) error
	RemoveProjectFromScope(ctx context.Context, scopeID, projectID string) error
}

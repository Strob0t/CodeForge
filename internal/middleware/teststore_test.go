package middleware_test

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/autoagent"
	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	bp "github.com/Strob0t/CodeForge/internal/domain/branchprotection"
	"github.com/Strob0t/CodeForge/internal/domain/channel"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/cost"
	"github.com/Strob0t/CodeForge/internal/domain/dashboard"
	"github.com/Strob0t/CodeForge/internal/domain/experience"
	"github.com/Strob0t/CodeForge/internal/domain/feedback"
	"github.com/Strob0t/CodeForge/internal/domain/goal"
	"github.com/Strob0t/CodeForge/internal/domain/knowledgebase"
	"github.com/Strob0t/CodeForge/internal/domain/llmkey"
	"github.com/Strob0t/CodeForge/internal/domain/mcp"
	"github.com/Strob0t/CodeForge/internal/domain/memory"
	"github.com/Strob0t/CodeForge/internal/domain/microagent"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/prompt"
	"github.com/Strob0t/CodeForge/internal/domain/quarantine"
	"github.com/Strob0t/CodeForge/internal/domain/resource"
	"github.com/Strob0t/CodeForge/internal/domain/review"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/domain/routing"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/settings"
	"github.com/Strob0t/CodeForge/internal/domain/skill"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/domain/tenant"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/domain/vcsaccount"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// Compile-time check.
var _ database.Store = (*testStore)(nil)

// testStore is a minimal in-memory Store for middleware auth tests.
// It implements the working methods needed for JWT validation (user lookup,
// token revocation check) and API key validation, with stubs for everything else.
type testStore struct {
	users         []user.User
	refreshTokens []user.RefreshToken
	apiKeys       []user.APIKey
	revokedTokens map[string]time.Time
}

// --- Working user methods ---

func (s *testStore) CreateUser(_ context.Context, u *user.User) error {
	s.users = append(s.users, *u)
	return nil
}

func (s *testStore) GetUser(_ context.Context, id string) (*user.User, error) {
	for i := range s.users {
		if s.users[i].ID == id {
			return &s.users[i], nil
		}
	}
	return nil, domain.ErrNotFound
}

func (s *testStore) GetUserByEmail(_ context.Context, email, tenantID string) (*user.User, error) {
	for i := range s.users {
		if s.users[i].Email == email && s.users[i].TenantID == tenantID {
			return &s.users[i], nil
		}
	}
	return nil, domain.ErrNotFound
}

func (s *testStore) ListUsers(_ context.Context, tenantID string) ([]user.User, error) {
	var result []user.User
	for i := range s.users {
		if s.users[i].TenantID == tenantID {
			result = append(result, s.users[i])
		}
	}
	return result, nil
}

func (s *testStore) UpdateUser(_ context.Context, u *user.User) error {
	for i := range s.users {
		if s.users[i].ID == u.ID {
			s.users[i] = *u
			return nil
		}
	}
	return domain.ErrNotFound
}

// --- Working refresh token methods ---

func (s *testStore) CreateRefreshToken(_ context.Context, rt *user.RefreshToken) error {
	s.refreshTokens = append(s.refreshTokens, *rt)
	return nil
}

func (s *testStore) GetRefreshTokenByHash(_ context.Context, hash string) (*user.RefreshToken, error) {
	for i := range s.refreshTokens {
		if s.refreshTokens[i].TokenHash == hash {
			return &s.refreshTokens[i], nil
		}
	}
	return nil, domain.ErrNotFound
}

// --- Working token revocation methods ---

func (s *testStore) IsTokenRevoked(_ context.Context, jti string) (bool, error) {
	if s.revokedTokens == nil {
		return false, nil
	}
	_, revoked := s.revokedTokens[jti]
	return revoked, nil
}

func (s *testStore) RevokeToken(_ context.Context, jti string, expiresAt time.Time) error {
	if s.revokedTokens == nil {
		s.revokedTokens = make(map[string]time.Time)
	}
	s.revokedTokens[jti] = expiresAt
	return nil
}

// --- Working API key methods ---

func (s *testStore) CreateAPIKey(_ context.Context, key *user.APIKey) error {
	s.apiKeys = append(s.apiKeys, *key)
	return nil
}

func (s *testStore) GetAPIKeyByHash(_ context.Context, hash string) (*user.APIKey, error) {
	for i := range s.apiKeys {
		if s.apiKeys[i].KeyHash == hash {
			return &s.apiKeys[i], nil
		}
	}
	return nil, domain.ErrNotFound
}

// --- Stubs for the rest of the Store interface ---

func (s *testStore) DeleteUser(_ context.Context, _ string) error                { return nil }
func (s *testStore) DeleteRefreshToken(_ context.Context, _ string) error        { return nil }
func (s *testStore) DeleteRefreshTokensByUser(_ context.Context, _ string) error { return nil }
func (s *testStore) PurgeExpiredTokens(_ context.Context) (int64, error)         { return 0, nil }
func (s *testStore) RotateRefreshToken(_ context.Context, _ string, _ *user.RefreshToken) error {
	return nil
}
func (s *testStore) ListAPIKeysByUser(_ context.Context, _ string) ([]user.APIKey, error) {
	return nil, nil
}
func (s *testStore) DeleteAPIKey(_ context.Context, _, _ string) error { return nil }
func (s *testStore) CreatePasswordResetToken(_ context.Context, _ *user.PasswordResetToken) error {
	return nil
}
func (s *testStore) GetPasswordResetTokenByHash(_ context.Context, _ string) (*user.PasswordResetToken, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) MarkPasswordResetTokenUsed(_ context.Context, _ string) error { return nil }
func (s *testStore) DeleteExpiredPasswordResetTokens(_ context.Context) (int64, error) {
	return 0, nil
}

// Project stubs
func (s *testStore) ListProjects(_ context.Context) ([]project.Project, error) { return nil, nil }
func (s *testStore) GetProject(_ context.Context, _ string) (*project.Project, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) CreateProject(_ context.Context, _ *project.CreateRequest) (*project.Project, error) {
	return nil, nil
}
func (s *testStore) UpdateProject(_ context.Context, _ *project.Project) error { return nil }
func (s *testStore) DeleteProject(_ context.Context, _ string) error           { return nil }
func (s *testStore) BatchDeleteProjects(_ context.Context, _ []string) ([]string, error) {
	return nil, nil
}
func (s *testStore) BatchGetProjects(_ context.Context, _ []string) ([]project.Project, error) {
	return nil, nil
}
func (s *testStore) GetProjectByRepoName(_ context.Context, _ string) (*project.Project, error) {
	return nil, nil
}

// Agent stubs
func (s *testStore) ListAgents(_ context.Context, _ string) ([]agent.Agent, error) { return nil, nil }
func (s *testStore) GetAgent(_ context.Context, _ string) (*agent.Agent, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) CreateAgent(_ context.Context, _, _, _ string, _ map[string]string, _ *resource.Limits) (*agent.Agent, error) {
	return nil, nil
}
func (s *testStore) UpdateAgentStatus(_ context.Context, _ string, _ agent.Status) error { return nil }
func (s *testStore) DeleteAgent(_ context.Context, _ string) error                       { return nil }

// Task stubs
func (s *testStore) ListTasks(_ context.Context, _ string) ([]task.Task, error) { return nil, nil }
func (s *testStore) GetTask(_ context.Context, _ string) (*task.Task, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) CreateTask(_ context.Context, _ task.CreateRequest) (*task.Task, error) {
	return nil, nil
}
func (s *testStore) UpdateTaskStatus(_ context.Context, _ string, _ task.Status) error { return nil }
func (s *testStore) UpdateTaskResult(_ context.Context, _ string, _ task.Result, _ float64) error {
	return nil
}

// Run stubs
func (s *testStore) CreateRun(_ context.Context, _ *run.Run) error { return nil }
func (s *testStore) GetRun(_ context.Context, _ string) (*run.Run, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) UpdateRunStatus(_ context.Context, _ string, _ run.Status, _ int, _ float64, _, _ int64) error {
	return nil
}
func (s *testStore) CompleteRun(_ context.Context, _ string, _ run.Status, _, _ string, _ float64, _ int, _, _ int64, _ string) error {
	return nil
}
func (s *testStore) UpdateRunArtifact(_ context.Context, _, _ string, _ *bool, _ []string) error {
	return nil
}
func (s *testStore) ListRunsByTask(_ context.Context, _ string) ([]run.Run, error) { return nil, nil }

// Plan stubs
func (s *testStore) CreatePlan(_ context.Context, _ *plan.ExecutionPlan) error { return nil }
func (s *testStore) GetPlan(_ context.Context, _ string) (*plan.ExecutionPlan, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) ListPlansByProject(_ context.Context, _ string) ([]plan.ExecutionPlan, error) {
	return nil, nil
}
func (s *testStore) UpdatePlanStatus(_ context.Context, _ string, _ plan.Status) error { return nil }
func (s *testStore) CreatePlanStep(_ context.Context, _ *plan.Step) error              { return nil }
func (s *testStore) ListPlanSteps(_ context.Context, _ string) ([]plan.Step, error)    { return nil, nil }
func (s *testStore) UpdatePlanStepStatus(_ context.Context, _ string, _ plan.StepStatus, _, _ string) error {
	return nil
}
func (s *testStore) GetPlanStepByRunID(_ context.Context, _ string) (*plan.Step, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) UpdatePlanStepRound(_ context.Context, _ string, _ int) error { return nil }

// Team stubs
func (s *testStore) CreateTeam(_ context.Context, _ agent.CreateTeamRequest) (*agent.Team, error) {
	return nil, nil
}
func (s *testStore) GetTeam(_ context.Context, _ string) (*agent.Team, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) ListTeamsByProject(_ context.Context, _ string) ([]agent.Team, error) {
	return nil, nil
}
func (s *testStore) UpdateTeamStatus(_ context.Context, _ string, _ agent.TeamStatus) error {
	return nil
}
func (s *testStore) DeleteTeam(_ context.Context, _ string) error { return nil }

// Context Pack stubs
func (s *testStore) CreateContextPack(_ context.Context, _ *cfcontext.ContextPack) error { return nil }
func (s *testStore) GetContextPack(_ context.Context, _ string) (*cfcontext.ContextPack, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) GetContextPackByTask(_ context.Context, _ string) (*cfcontext.ContextPack, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) DeleteContextPack(_ context.Context, _ string) error { return nil }

// Shared Context stubs
func (s *testStore) CreateSharedContext(_ context.Context, _ *cfcontext.SharedContext) error {
	return nil
}
func (s *testStore) GetSharedContext(_ context.Context, _ string) (*cfcontext.SharedContext, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) GetSharedContextByTeam(_ context.Context, _ string) (*cfcontext.SharedContext, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) AddSharedContextItem(_ context.Context, _ cfcontext.AddSharedItemRequest) (*cfcontext.SharedContextItem, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) DeleteSharedContext(_ context.Context, _ string) error { return nil }

// Repo Map stubs
func (s *testStore) UpsertRepoMap(_ context.Context, _ *cfcontext.RepoMap) error { return nil }
func (s *testStore) GetRepoMap(_ context.Context, _ string) (*cfcontext.RepoMap, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) DeleteRepoMap(_ context.Context, _ string) error { return nil }

// Cost stubs
func (s *testStore) CostSummaryGlobal(_ context.Context) ([]cost.ProjectSummary, error) {
	return nil, nil
}
func (s *testStore) CostSummaryByProject(_ context.Context, _ string) (*cost.Summary, error) {
	return &cost.Summary{}, nil
}
func (s *testStore) CostByModel(_ context.Context, _ string) ([]cost.ModelSummary, error) {
	return nil, nil
}
func (s *testStore) CostTimeSeries(_ context.Context, _ string, _ int) ([]cost.DailyCost, error) {
	return nil, nil
}
func (s *testStore) RecentRunsWithCost(_ context.Context, _ string, _ int) ([]run.Run, error) {
	return nil, nil
}
func (s *testStore) CostByTool(_ context.Context, _ string) ([]cost.ToolSummary, error) {
	return nil, nil
}
func (s *testStore) CostByToolForRun(_ context.Context, _ string) ([]cost.ToolSummary, error) {
	return nil, nil
}

// Dashboard Aggregation stubs
func (s *testStore) DashboardStats(_ context.Context) (*dashboard.DashboardStats, error) {
	return &dashboard.DashboardStats{}, nil
}
func (s *testStore) ProjectHealth(_ context.Context, _ string) (*dashboard.ProjectHealth, error) {
	return &dashboard.ProjectHealth{Sparkline: []float64{}}, nil
}
func (s *testStore) DashboardRunOutcomes(_ context.Context, _ int) ([]dashboard.RunOutcome, error) {
	return []dashboard.RunOutcome{}, nil
}
func (s *testStore) DashboardAgentPerformance(_ context.Context) ([]dashboard.AgentPerf, error) {
	return []dashboard.AgentPerf{}, nil
}
func (s *testStore) DashboardModelUsage(_ context.Context) ([]dashboard.ModelUsage, error) {
	return []dashboard.ModelUsage{}, nil
}
func (s *testStore) DashboardCostByProject(_ context.Context) ([]dashboard.ProjectCost, error) {
	return []dashboard.ProjectCost{}, nil
}
func (s *testStore) DashboardCostTrend(_ context.Context, _ int) ([]cost.DailyCost, error) {
	return []cost.DailyCost{}, nil
}

// Roadmap stubs
func (s *testStore) CreateRoadmap(_ context.Context, _ roadmap.CreateRoadmapRequest) (*roadmap.Roadmap, error) {
	return nil, nil
}
func (s *testStore) GetRoadmap(_ context.Context, _ string) (*roadmap.Roadmap, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) GetRoadmapByProject(_ context.Context, _ string) (*roadmap.Roadmap, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) UpdateRoadmap(_ context.Context, _ *roadmap.Roadmap) error { return nil }
func (s *testStore) DeleteRoadmap(_ context.Context, _ string) error           { return nil }
func (s *testStore) CreateMilestone(_ context.Context, _ roadmap.CreateMilestoneRequest) (*roadmap.Milestone, error) {
	return nil, nil
}
func (s *testStore) GetMilestone(_ context.Context, _ string) (*roadmap.Milestone, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) ListMilestones(_ context.Context, _ string) ([]roadmap.Milestone, error) {
	return nil, nil
}
func (s *testStore) UpdateMilestone(_ context.Context, _ *roadmap.Milestone) error { return nil }
func (s *testStore) DeleteMilestone(_ context.Context, _ string) error             { return nil }
func (s *testStore) FindMilestoneByTitle(_ context.Context, _, _ string) (*roadmap.Milestone, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) CreateFeature(_ context.Context, _ *roadmap.CreateFeatureRequest) (*roadmap.Feature, error) {
	return nil, nil
}
func (s *testStore) GetFeature(_ context.Context, _ string) (*roadmap.Feature, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) FindFeatureBySpecRef(_ context.Context, _, _ string) (*roadmap.Feature, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) ListFeatures(_ context.Context, _ string) ([]roadmap.Feature, error) {
	return nil, nil
}
func (s *testStore) ListFeaturesByRoadmap(_ context.Context, _ string) ([]roadmap.Feature, error) {
	return nil, nil
}
func (s *testStore) UpdateFeature(_ context.Context, _ *roadmap.Feature) error { return nil }
func (s *testStore) DeleteFeature(_ context.Context, _ string) error           { return nil }

// Tenant stubs
func (s *testStore) CreateTenant(_ context.Context, _ tenant.CreateRequest) (*tenant.Tenant, error) {
	return nil, nil
}
func (s *testStore) GetTenant(_ context.Context, _ string) (*tenant.Tenant, error) { return nil, nil }
func (s *testStore) ListTenants(_ context.Context) ([]tenant.Tenant, error)        { return nil, nil }
func (s *testStore) UpdateTenant(_ context.Context, _ *tenant.Tenant) error        { return nil }

// Branch Protection stubs
func (s *testStore) CreateBranchProtectionRule(_ context.Context, _ bp.CreateRuleRequest) (*bp.ProtectionRule, error) {
	return nil, nil
}
func (s *testStore) GetBranchProtectionRule(_ context.Context, _ string) (*bp.ProtectionRule, error) {
	return nil, nil
}
func (s *testStore) ListBranchProtectionRules(_ context.Context, _ string) ([]bp.ProtectionRule, error) {
	return nil, nil
}
func (s *testStore) UpdateBranchProtectionRule(_ context.Context, _ *bp.ProtectionRule) error {
	return nil
}
func (s *testStore) DeleteBranchProtectionRule(_ context.Context, _ string) error { return nil }

// Session stubs
func (s *testStore) CreateSession(_ context.Context, _ *run.Session) error { return nil }
func (s *testStore) GetSession(_ context.Context, _ string) (*run.Session, error) {
	return nil, nil
}
func (s *testStore) GetSessionByConversation(_ context.Context, _ string) (*run.Session, error) {
	return nil, nil
}
func (s *testStore) ListSessions(_ context.Context, _ string) ([]run.Session, error) {
	return nil, nil
}
func (s *testStore) UpdateSessionStatus(_ context.Context, _ string, _ run.SessionStatus, _ string) error {
	return nil
}

// Review Policy stubs
func (s *testStore) CreateReviewPolicy(_ context.Context, _ *review.ReviewPolicy) error { return nil }
func (s *testStore) GetReviewPolicy(_ context.Context, _ string) (*review.ReviewPolicy, error) {
	return nil, nil
}
func (s *testStore) ListReviewPoliciesByProject(_ context.Context, _ string) ([]review.ReviewPolicy, error) {
	return nil, nil
}
func (s *testStore) UpdateReviewPolicy(_ context.Context, _ *review.ReviewPolicy) error { return nil }
func (s *testStore) DeleteReviewPolicy(_ context.Context, _ string) error               { return nil }
func (s *testStore) ListEnabledPoliciesByTrigger(_ context.Context, _ review.TriggerType) ([]review.ReviewPolicy, error) {
	return nil, nil
}
func (s *testStore) IncrementCommitCounter(_ context.Context, _ string, _ int) (int, error) {
	return 0, nil
}
func (s *testStore) ResetCommitCounter(_ context.Context, _ string) error   { return nil }
func (s *testStore) CreateReview(_ context.Context, _ *review.Review) error { return nil }
func (s *testStore) GetReview(_ context.Context, _ string) (*review.Review, error) {
	return nil, nil
}
func (s *testStore) ListReviewsByProject(_ context.Context, _ string) ([]review.Review, error) {
	return nil, nil
}
func (s *testStore) UpdateReviewStatus(_ context.Context, _ string, _ review.Status, _ *time.Time) error {
	return nil
}
func (s *testStore) GetReviewByPlanID(_ context.Context, _ string) (*review.Review, error) {
	return nil, nil
}

// Retrieval Scope stubs
func (s *testStore) CreateScope(_ context.Context, _ cfcontext.CreateScopeRequest) (*cfcontext.RetrievalScope, error) {
	return nil, nil
}
func (s *testStore) GetScope(_ context.Context, _ string) (*cfcontext.RetrievalScope, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) ListScopes(_ context.Context) ([]cfcontext.RetrievalScope, error) {
	return nil, nil
}
func (s *testStore) UpdateScope(_ context.Context, _ string, _ cfcontext.UpdateScopeRequest) (*cfcontext.RetrievalScope, error) {
	return nil, nil
}
func (s *testStore) DeleteScope(_ context.Context, _ string) error { return nil }
func (s *testStore) ListScopesByProject(_ context.Context, _ string) ([]cfcontext.RetrievalScope, error) {
	return nil, nil
}
func (s *testStore) AddProjectToScope(_ context.Context, _, _ string) error      { return nil }
func (s *testStore) RemoveProjectFromScope(_ context.Context, _, _ string) error { return nil }

// Knowledge Base stubs
func (s *testStore) CreateKnowledgeBase(_ context.Context, _ *knowledgebase.CreateRequest) (*knowledgebase.KnowledgeBase, error) {
	return nil, nil
}
func (s *testStore) GetKnowledgeBase(_ context.Context, _ string) (*knowledgebase.KnowledgeBase, error) {
	return nil, nil
}
func (s *testStore) ListKnowledgeBases(_ context.Context) ([]knowledgebase.KnowledgeBase, error) {
	return nil, nil
}
func (s *testStore) UpdateKnowledgeBase(_ context.Context, _ string, _ knowledgebase.UpdateRequest) (*knowledgebase.KnowledgeBase, error) {
	return nil, nil
}
func (s *testStore) DeleteKnowledgeBase(_ context.Context, _ string) error { return nil }
func (s *testStore) UpdateKnowledgeBaseStatus(_ context.Context, _, _ string, _ int) error {
	return nil
}
func (s *testStore) AddKnowledgeBaseToScope(_ context.Context, _, _ string) error      { return nil }
func (s *testStore) RemoveKnowledgeBaseFromScope(_ context.Context, _, _ string) error { return nil }
func (s *testStore) ListKnowledgeBasesByScope(_ context.Context, _ string) ([]knowledgebase.KnowledgeBase, error) {
	return nil, nil
}

// Settings stubs
func (s *testStore) ListSettings(_ context.Context) ([]settings.Setting, error) { return nil, nil }
func (s *testStore) GetSetting(_ context.Context, _ string) (*settings.Setting, error) {
	return nil, nil
}
func (s *testStore) UpsertSetting(_ context.Context, _ string, _ json.RawMessage) error { return nil }

// VCS Account stubs
func (s *testStore) ListVCSAccounts(_ context.Context) ([]vcsaccount.VCSAccount, error) {
	return nil, nil
}
func (s *testStore) CreateVCSAccount(_ context.Context, _ *vcsaccount.VCSAccount) (*vcsaccount.VCSAccount, error) {
	return nil, nil
}
func (s *testStore) GetVCSAccount(_ context.Context, _ string) (*vcsaccount.VCSAccount, error) {
	return nil, nil
}
func (s *testStore) DeleteVCSAccount(_ context.Context, _ string) error { return nil }

// OAuth State stubs
func (s *testStore) CreateOAuthState(_ context.Context, _ *vcsaccount.OAuthState) error {
	return nil
}
func (s *testStore) GetOAuthState(_ context.Context, _ string) (*vcsaccount.OAuthState, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) DeleteOAuthState(_ context.Context, _ string) error        { return nil }
func (s *testStore) DeleteExpiredOAuthStates(_ context.Context) (int64, error) { return 0, nil }

// Conversation stubs
func (s *testStore) CreateConversation(_ context.Context, _ *conversation.Conversation) (*conversation.Conversation, error) {
	return nil, nil
}
func (s *testStore) GetConversation(_ context.Context, _ string) (*conversation.Conversation, error) {
	return nil, nil
}
func (s *testStore) ListConversationsByProject(_ context.Context, _ string) ([]conversation.Conversation, error) {
	return nil, nil
}
func (s *testStore) DeleteConversation(_ context.Context, _ string) error { return nil }
func (s *testStore) CreateMessage(_ context.Context, _ *conversation.Message) (*conversation.Message, error) {
	return nil, nil
}
func (s *testStore) CreateToolMessages(_ context.Context, _ string, _ []conversation.Message) error {
	return nil
}
func (s *testStore) ListMessages(_ context.Context, _ string) ([]conversation.Message, error) {
	return nil, nil
}
func (s *testStore) DeleteConversationMessages(_ context.Context, _ string) error { return nil }
func (s *testStore) UpdateConversationMode(_ context.Context, _, _ string) error  { return nil }
func (s *testStore) UpdateConversationModel(_ context.Context, _, _ string) error { return nil }
func (s *testStore) SearchConversationMessages(_ context.Context, _ string, _ []string, _ int) ([]conversation.Message, error) {
	return nil, nil
}

// MCP stubs
func (s *testStore) CreateMCPServer(_ context.Context, _ *mcp.ServerDef) error { return nil }
func (s *testStore) GetMCPServer(_ context.Context, _ string) (*mcp.ServerDef, error) {
	return nil, nil
}
func (s *testStore) ListMCPServers(_ context.Context) ([]mcp.ServerDef, error) { return nil, nil }
func (s *testStore) UpdateMCPServer(_ context.Context, _ *mcp.ServerDef) error { return nil }
func (s *testStore) DeleteMCPServer(_ context.Context, _ string) error         { return nil }
func (s *testStore) UpdateMCPServerStatus(_ context.Context, _ string, _ mcp.ServerStatus) error {
	return nil
}
func (s *testStore) AssignMCPServerToProject(_ context.Context, _, _ string) error     { return nil }
func (s *testStore) UnassignMCPServerFromProject(_ context.Context, _, _ string) error { return nil }
func (s *testStore) ListMCPServersByProject(_ context.Context, _ string) ([]mcp.ServerDef, error) {
	return nil, nil
}
func (s *testStore) UpsertMCPServerTools(_ context.Context, _ string, _ []mcp.ServerTool) error {
	return nil
}
func (s *testStore) ListMCPServerTools(_ context.Context, _ string) ([]mcp.ServerTool, error) {
	return nil, nil
}

// Prompt stubs
func (s *testStore) ListPromptSections(_ context.Context, _ string) ([]prompt.SectionRow, error) {
	return nil, nil
}
func (s *testStore) UpsertPromptSection(_ context.Context, _ *prompt.SectionRow) error { return nil }
func (s *testStore) DeletePromptSection(_ context.Context, _ string) error             { return nil }

// Benchmark suite stubs (Phase 26)
func (s *testStore) CreateBenchmarkSuite(_ context.Context, _ *benchmark.Suite) error { return nil }
func (s *testStore) GetBenchmarkSuite(_ context.Context, _ string) (*benchmark.Suite, error) {
	return nil, nil
}
func (s *testStore) ListBenchmarkSuites(_ context.Context) ([]benchmark.Suite, error) {
	return nil, nil
}
func (s *testStore) DeleteBenchmarkSuite(_ context.Context, _ string) error           { return nil }
func (s *testStore) UpdateBenchmarkSuite(_ context.Context, _ *benchmark.Suite) error { return nil }
func (s *testStore) ListBenchmarkRunsFiltered(_ context.Context, _ *benchmark.RunFilter) ([]benchmark.Run, error) {
	return nil, nil
}

// Benchmark stubs
func (s *testStore) CreateBenchmarkRun(_ context.Context, _ *benchmark.Run) error { return nil }
func (s *testStore) GetBenchmarkRun(_ context.Context, _ string) (*benchmark.Run, error) {
	return nil, nil
}
func (s *testStore) ListBenchmarkRuns(_ context.Context) ([]benchmark.Run, error) { return nil, nil }
func (s *testStore) UpdateBenchmarkRun(_ context.Context, _ *benchmark.Run) error { return nil }
func (s *testStore) DeleteBenchmarkRun(_ context.Context, _ string) error         { return nil }
func (s *testStore) CreateBenchmarkResult(_ context.Context, _ *benchmark.Result) error {
	return nil
}
func (s *testStore) ListBenchmarkResults(_ context.Context, _ string) ([]benchmark.Result, error) {
	return nil, nil
}

// Experience Pool stubs
func (s *testStore) CreateExperienceEntry(_ context.Context, _ *experience.Entry) error { return nil }
func (s *testStore) GetExperienceEntry(_ context.Context, _ string) (*experience.Entry, error) {
	return nil, nil
}
func (s *testStore) ListExperienceEntries(_ context.Context, _ string) ([]experience.Entry, error) {
	return nil, nil
}
func (s *testStore) DeleteExperienceEntry(_ context.Context, _ string) error { return nil }
func (s *testStore) UpdateExperienceHit(_ context.Context, _ string) error   { return nil }

// Memory stubs
func (s *testStore) CreateMemory(_ context.Context, _ *memory.Memory) error { return nil }
func (s *testStore) ListMemories(_ context.Context, _ string) ([]memory.Memory, error) {
	return nil, nil
}

// Microagent stubs
func (s *testStore) CreateMicroagent(_ context.Context, _ *microagent.Microagent) error { return nil }
func (s *testStore) GetMicroagent(_ context.Context, _ string) (*microagent.Microagent, error) {
	return nil, nil
}
func (s *testStore) ListMicroagents(_ context.Context, _ string) ([]microagent.Microagent, error) {
	return nil, nil
}
func (s *testStore) UpdateMicroagent(_ context.Context, _ *microagent.Microagent) error { return nil }
func (s *testStore) DeleteMicroagent(_ context.Context, _ string) error                 { return nil }

// Skill stubs
func (s *testStore) CreateSkill(_ context.Context, _ *skill.Skill) error           { return nil }
func (s *testStore) GetSkill(_ context.Context, _ string) (*skill.Skill, error)    { return nil, nil }
func (s *testStore) ListSkills(_ context.Context, _ string) ([]skill.Skill, error) { return nil, nil }
func (s *testStore) UpdateSkill(_ context.Context, _ *skill.Skill) error           { return nil }
func (s *testStore) DeleteSkill(_ context.Context, _ string) error                 { return nil }
func (s *testStore) IncrementSkillUsage(_ context.Context, _ string) error         { return nil }
func (s *testStore) ListActiveSkills(_ context.Context, _ string) ([]skill.Skill, error) {
	return nil, nil
}

// Feedback stubs
func (s *testStore) CreateFeedbackAudit(_ context.Context, _ *feedback.AuditEntry) error {
	return nil
}
func (s *testStore) ListFeedbackByRun(_ context.Context, _ string) ([]feedback.AuditEntry, error) {
	return nil, nil
}

// Auto-Agent stubs
func (s *testStore) UpsertAutoAgent(_ context.Context, _ *autoagent.AutoAgent) error { return nil }
func (s *testStore) GetAutoAgent(_ context.Context, _ string) (*autoagent.AutoAgent, error) {
	return nil, nil
}
func (s *testStore) UpdateAutoAgentStatus(_ context.Context, _ string, _ autoagent.Status, _ string) error {
	return nil
}
func (s *testStore) UpdateAutoAgentProgress(_ context.Context, _ *autoagent.AutoAgent) error {
	return nil
}
func (s *testStore) DeleteAutoAgent(_ context.Context, _ string) error { return nil }

// Quarantine (Phase 23B)
func (s *testStore) QuarantineMessage(_ context.Context, _ *quarantine.Message) error { return nil }
func (s *testStore) GetQuarantinedMessage(_ context.Context, _ string) (*quarantine.Message, error) {
	return nil, domain.ErrNotFound
}
func (s *testStore) ListQuarantinedMessages(_ context.Context, _ string, _ quarantine.Status, _, _ int) ([]*quarantine.Message, error) {
	return nil, nil
}
func (s *testStore) UpdateQuarantineStatus(_ context.Context, _ string, _ quarantine.Status, _, _ string) error {
	return nil
}

// Agent Identity (Phase 23C)
func (s *testStore) IncrementAgentStats(_ context.Context, _ string, _ float64, _ bool) error {
	return nil
}
func (s *testStore) UpdateAgentState(_ context.Context, _ string, _ map[string]string) error {
	return nil
}
func (s *testStore) SendAgentMessage(_ context.Context, _ *agent.InboxMessage) error { return nil }
func (s *testStore) ListAgentInbox(_ context.Context, _ string, _ bool) ([]agent.InboxMessage, error) {
	return nil, nil
}
func (s *testStore) MarkInboxRead(_ context.Context, _ string) error { return nil }

// Active Work Visibility (Phase 24)
func (s *testStore) ListActiveWork(_ context.Context, _ string) ([]task.ActiveWorkItem, error) {
	return nil, nil
}
func (s *testStore) ClaimTask(_ context.Context, _, _ string, _ int) (*task.ClaimResult, error) {
	return nil, nil
}
func (s *testStore) ReleaseStaleWork(_ context.Context, _ time.Duration) ([]task.Task, error) {
	return nil, nil
}

// --- Routing stubs (Phase 26) ---

func (s *testStore) CreateRoutingOutcome(_ context.Context, _ *routing.RoutingOutcome) error {
	return nil
}
func (s *testStore) ListRoutingStats(_ context.Context, _, _ string) ([]routing.ModelPerformanceStats, error) {
	return nil, nil
}
func (s *testStore) UpsertRoutingStats(_ context.Context, _ *routing.ModelPerformanceStats) error {
	return nil
}
func (s *testStore) AggregateRoutingOutcomes(_ context.Context) error { return nil }
func (s *testStore) ListRoutingOutcomes(_ context.Context, _ int) ([]routing.RoutingOutcome, error) {
	return nil, nil
}

// A2A stubs (Phase 27)
func (s *testStore) CreateA2ATask(_ context.Context, _ *a2adomain.A2ATask) error { return nil }
func (s *testStore) GetA2ATask(_ context.Context, _ string) (*a2adomain.A2ATask, error) {
	return nil, nil
}
func (s *testStore) UpdateA2ATask(_ context.Context, _ *a2adomain.A2ATask) error { return nil }
func (s *testStore) ListA2ATasks(_ context.Context, _ *database.A2ATaskFilter) ([]a2adomain.A2ATask, int, error) {
	return nil, 0, nil
}
func (s *testStore) DeleteA2ATask(_ context.Context, _ string) error { return nil }
func (s *testStore) CreateRemoteAgent(_ context.Context, _ *a2adomain.RemoteAgent) error {
	return nil
}
func (s *testStore) GetRemoteAgent(_ context.Context, _ string) (*a2adomain.RemoteAgent, error) {
	return nil, nil
}
func (s *testStore) ListRemoteAgents(_ context.Context, _ string, _ bool) ([]a2adomain.RemoteAgent, error) {
	return nil, nil
}
func (s *testStore) UpdateRemoteAgent(_ context.Context, _ *a2adomain.RemoteAgent) error {
	return nil
}
func (s *testStore) DeleteRemoteAgent(_ context.Context, _ string) error { return nil }
func (s *testStore) CreateA2APushConfig(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}
func (s *testStore) GetA2APushConfig(_ context.Context, _ string) (_, _, _ string, _ error) {
	return "", "", "", nil
}
func (s *testStore) ListA2APushConfigs(_ context.Context, _ string) ([]database.A2APushConfig, error) {
	return nil, nil
}
func (s *testStore) DeleteA2APushConfig(_ context.Context, _ string) error     { return nil }
func (s *testStore) DeleteAllA2APushConfigs(_ context.Context, _ string) error { return nil }

// Project Goal stubs
func (s *testStore) CreateProjectGoal(_ context.Context, _ *goal.ProjectGoal) error { return nil }
func (s *testStore) GetProjectGoal(_ context.Context, _ string) (*goal.ProjectGoal, error) {
	return nil, nil
}
func (s *testStore) ListProjectGoals(_ context.Context, _ string) ([]goal.ProjectGoal, error) {
	return nil, nil
}
func (s *testStore) ListEnabledGoals(_ context.Context, _ string) ([]goal.ProjectGoal, error) {
	return nil, nil
}
func (s *testStore) UpdateProjectGoal(_ context.Context, _ *goal.ProjectGoal) error { return nil }
func (s *testStore) DeleteProjectGoal(_ context.Context, _ string) error            { return nil }
func (s *testStore) DeleteProjectGoalsBySource(_ context.Context, _, _ string) error {
	return nil
}

// LLM Key stubs
func (s *testStore) CreateLLMKey(_ context.Context, _ *llmkey.LLMKey) error { return nil }
func (s *testStore) ListLLMKeysByUser(_ context.Context, _ string) ([]llmkey.LLMKey, error) {
	return nil, nil
}
func (s *testStore) GetLLMKeyByUserProvider(_ context.Context, _, _ string) (*llmkey.LLMKey, error) {
	return nil, nil
}
func (s *testStore) DeleteLLMKey(_ context.Context, _, _ string) error { return nil }

// Channel stubs
func (s *testStore) CreateChannel(_ context.Context, _ *channel.Channel) (*channel.Channel, error) {
	return nil, nil
}
func (s *testStore) GetChannel(_ context.Context, _ string) (*channel.Channel, error) {
	return nil, nil
}
func (s *testStore) ListChannels(_ context.Context, _ string) ([]channel.Channel, error) {
	return nil, nil
}
func (s *testStore) DeleteChannel(_ context.Context, _ string) error { return nil }
func (s *testStore) CreateChannelMessage(_ context.Context, _ *channel.Message) (*channel.Message, error) {
	return nil, nil
}
func (s *testStore) ListChannelMessages(_ context.Context, _, _ string, _ int) ([]channel.Message, error) {
	return nil, nil
}
func (s *testStore) AddChannelMember(_ context.Context, _ *channel.Member) error { return nil }
func (s *testStore) UpdateChannelMemberNotify(_ context.Context, _, _ string, _ channel.NotifySetting) error {
	return nil
}

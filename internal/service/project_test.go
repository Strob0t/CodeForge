package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/autoagent"
	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/domain/boundary"
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

// Ensure mockStore implements database.Store at compile time.
var _ database.Store = (*mockStore)(nil)

// mockStore is a minimal in-memory implementation of database.Store for testing.
type mockStore struct {
	projects      []project.Project
	agents        []agent.Agent
	tasks         []task.Task
	users         []user.User
	refreshTokens []user.RefreshToken
	apiKeys       []user.APIKey

	// Auth-related fields for token revocation and password reset flows.
	revokedTokens       map[string]time.Time // jti -> expiresAt
	passwordResetTokens []user.PasswordResetToken
	isTokenRevokedErr   error // injectable error for fail-closed test

	// Agent inbox (Phase 23C).
	inboxMessages []agent.InboxMessage
	inboxNextID   int

	// Error hooks — set these to inject failures.
	listProjectsErr  error
	getProjectErr    error
	createProjectErr error
	updateProjectErr error
	deleteProjectErr error
}

func (m *mockStore) ListProjects(_ context.Context) ([]project.Project, error) {
	return m.projects, m.listProjectsErr
}

func (m *mockStore) GetProject(_ context.Context, id string) (*project.Project, error) {
	if m.getProjectErr != nil {
		return nil, m.getProjectErr
	}
	for i := range m.projects {
		if m.projects[i].ID == id {
			return &m.projects[i], nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockStore) CreateProject(_ context.Context, req *project.CreateRequest) (*project.Project, error) {
	if m.createProjectErr != nil {
		return nil, m.createProjectErr
	}
	p := project.Project{
		ID:       "proj-1",
		Name:     req.Name,
		Provider: req.Provider,
		RepoURL:  req.RepoURL,
		Config:   req.Config,
	}
	m.projects = append(m.projects, p)
	return &p, nil
}

func (m *mockStore) UpdateProject(_ context.Context, p *project.Project) error {
	if m.updateProjectErr != nil {
		return m.updateProjectErr
	}
	for i := range m.projects {
		if m.projects[i].ID == p.ID {
			m.projects[i] = *p
			return nil
		}
	}
	return domain.ErrNotFound
}

func (m *mockStore) DeleteProject(_ context.Context, id string) error {
	if m.deleteProjectErr != nil {
		return m.deleteProjectErr
	}
	for i := range m.projects {
		if m.projects[i].ID == id {
			m.projects = append(m.projects[:i], m.projects[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}
func (m *mockStore) BatchDeleteProjects(_ context.Context, _ []string) ([]string, error) {
	return nil, nil
}
func (m *mockStore) BatchGetProjects(_ context.Context, _ []string) ([]project.Project, error) {
	return nil, nil
}

func (m *mockStore) ListAgents(_ context.Context, _ string) ([]agent.Agent, error) {
	return m.agents, nil
}

func (m *mockStore) GetAgent(_ context.Context, id string) (*agent.Agent, error) {
	for i := range m.agents {
		if m.agents[i].ID == id {
			return &m.agents[i], nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockStore) CreateAgent(_ context.Context, projectID, name, backend string, config map[string]string, limits *resource.Limits) (*agent.Agent, error) {
	a := agent.Agent{ID: "agent-1", ProjectID: projectID, Name: name, Backend: backend, Config: config, ResourceLimits: limits}
	m.agents = append(m.agents, a)
	return &a, nil
}

func (m *mockStore) UpdateAgentStatus(_ context.Context, id string, status agent.Status) error {
	for i := range m.agents {
		if m.agents[i].ID == id {
			m.agents[i].Status = status
			return nil
		}
	}
	return domain.ErrNotFound
}

func (m *mockStore) DeleteAgent(_ context.Context, id string) error {
	for i := range m.agents {
		if m.agents[i].ID == id {
			m.agents = append(m.agents[:i], m.agents[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (m *mockStore) ListTasks(_ context.Context, _ string) ([]task.Task, error) {
	return m.tasks, nil
}

func (m *mockStore) GetTask(_ context.Context, id string) (*task.Task, error) {
	for i := range m.tasks {
		if m.tasks[i].ID == id {
			return &m.tasks[i], nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockStore) CreateTask(_ context.Context, req task.CreateRequest) (*task.Task, error) {
	t := task.Task{ID: "task-1", ProjectID: req.ProjectID, Title: req.Title, Status: task.StatusPending}
	m.tasks = append(m.tasks, t)
	return &t, nil
}

func (m *mockStore) UpdateTaskStatus(_ context.Context, _ string, _ task.Status) error {
	return nil
}

func (m *mockStore) UpdateTaskResult(_ context.Context, _ string, _ task.Result, _ float64) error {
	return nil
}

// --- Run methods (satisfy database.Store interface) ---

func (m *mockStore) CreateRun(_ context.Context, _ *run.Run) error { return nil }
func (m *mockStore) GetRun(_ context.Context, _ string) (*run.Run, error) {
	return nil, domain.ErrNotFound
}
func (m *mockStore) UpdateRunStatus(_ context.Context, _ string, _ run.Status, _ int, _ float64, _, _ int64) error {
	return nil
}
func (m *mockStore) CompleteRun(_ context.Context, _ string, _ run.Status, _, _ string, _ float64, _ int, _, _ int64, _ string) error {
	return nil
}
func (m *mockStore) UpdateRunArtifact(_ context.Context, _, _ string, _ *bool, _ []string) error {
	return nil
}
func (m *mockStore) ListRunsByTask(_ context.Context, _ string) ([]run.Run, error) { return nil, nil }

// --- Plan stub methods (satisfy database.Store interface) ---

func (m *mockStore) CreatePlan(_ context.Context, _ *plan.ExecutionPlan) error { return nil }
func (m *mockStore) GetPlan(_ context.Context, _ string) (*plan.ExecutionPlan, error) {
	return nil, domain.ErrNotFound
}
func (m *mockStore) ListPlansByProject(_ context.Context, _ string) ([]plan.ExecutionPlan, error) {
	return nil, nil
}
func (m *mockStore) UpdatePlanStatus(_ context.Context, _ string, _ plan.Status) error { return nil }
func (m *mockStore) CreatePlanStep(_ context.Context, _ *plan.Step) error              { return nil }
func (m *mockStore) ListPlanSteps(_ context.Context, _ string) ([]plan.Step, error)    { return nil, nil }
func (m *mockStore) UpdatePlanStepStatus(_ context.Context, _ string, _ plan.StepStatus, _, _ string) error {
	return nil
}
func (m *mockStore) GetPlanStepByRunID(_ context.Context, _ string) (*plan.Step, error) {
	return nil, domain.ErrNotFound
}
func (m *mockStore) UpdatePlanStepRound(_ context.Context, _ string, _ int) error { return nil }

// --- Agent Team stub methods (satisfy database.Store interface) ---

func (m *mockStore) CreateTeam(_ context.Context, _ agent.CreateTeamRequest) (*agent.Team, error) {
	return nil, nil
}
func (m *mockStore) GetTeam(_ context.Context, _ string) (*agent.Team, error) {
	return nil, domain.ErrNotFound
}
func (m *mockStore) ListTeamsByProject(_ context.Context, _ string) ([]agent.Team, error) {
	return nil, nil
}
func (m *mockStore) UpdateTeamStatus(_ context.Context, _ string, _ agent.TeamStatus) error {
	return nil
}
func (m *mockStore) DeleteTeam(_ context.Context, _ string) error { return nil }

// Context Pack stubs
func (m *mockStore) CreateContextPack(_ context.Context, _ *cfcontext.ContextPack) error {
	return nil
}
func (m *mockStore) GetContextPack(_ context.Context, _ string) (*cfcontext.ContextPack, error) {
	return nil, domain.ErrNotFound
}
func (m *mockStore) GetContextPackByTask(_ context.Context, _ string) (*cfcontext.ContextPack, error) {
	return nil, domain.ErrNotFound
}
func (m *mockStore) DeleteContextPack(_ context.Context, _ string) error { return nil }

// Shared Context stubs
func (m *mockStore) CreateSharedContext(_ context.Context, _ *cfcontext.SharedContext) error {
	return nil
}
func (m *mockStore) GetSharedContext(_ context.Context, _ string) (*cfcontext.SharedContext, error) {
	return nil, domain.ErrNotFound
}
func (m *mockStore) GetSharedContextByTeam(_ context.Context, _ string) (*cfcontext.SharedContext, error) {
	return nil, domain.ErrNotFound
}
func (m *mockStore) AddSharedContextItem(_ context.Context, _ cfcontext.AddSharedItemRequest) (*cfcontext.SharedContextItem, error) {
	return nil, domain.ErrNotFound
}
func (m *mockStore) DeleteSharedContext(_ context.Context, _ string) error { return nil }

// Repo Map stubs
func (m *mockStore) UpsertRepoMap(_ context.Context, _ *cfcontext.RepoMap) error { return nil }
func (m *mockStore) GetRepoMap(_ context.Context, _ string) (*cfcontext.RepoMap, error) {
	return nil, domain.ErrNotFound
}
func (m *mockStore) DeleteRepoMap(_ context.Context, _ string) error { return nil }

// Cost Aggregation stubs
func (m *mockStore) CostSummaryGlobal(_ context.Context) ([]cost.ProjectSummary, error) {
	return nil, nil
}
func (m *mockStore) CostSummaryByProject(_ context.Context, _ string) (*cost.Summary, error) {
	return &cost.Summary{}, nil
}
func (m *mockStore) CostByModel(_ context.Context, _ string) ([]cost.ModelSummary, error) {
	return nil, nil
}
func (m *mockStore) CostTimeSeries(_ context.Context, _ string, _ int) ([]cost.DailyCost, error) {
	return nil, nil
}
func (m *mockStore) RecentRunsWithCost(_ context.Context, _ string, _ int) ([]run.Run, error) {
	return nil, nil
}
func (m *mockStore) CostByTool(_ context.Context, _ string) ([]cost.ToolSummary, error) {
	return nil, nil
}
func (m *mockStore) CostByToolForRun(_ context.Context, _ string) ([]cost.ToolSummary, error) {
	return nil, nil
}

// Dashboard Aggregation stubs
func (m *mockStore) DashboardStats(_ context.Context) (*dashboard.DashboardStats, error) {
	return &dashboard.DashboardStats{}, nil
}
func (m *mockStore) ProjectHealth(_ context.Context, _ string) (*dashboard.ProjectHealth, error) {
	return &dashboard.ProjectHealth{Level: "critical", Sparkline: []float64{}}, nil
}
func (m *mockStore) DashboardRunOutcomes(_ context.Context, _ int) ([]dashboard.RunOutcome, error) {
	return []dashboard.RunOutcome{}, nil
}
func (m *mockStore) DashboardAgentPerformance(_ context.Context) ([]dashboard.AgentPerf, error) {
	return []dashboard.AgentPerf{}, nil
}
func (m *mockStore) DashboardModelUsage(_ context.Context) ([]dashboard.ModelUsage, error) {
	return []dashboard.ModelUsage{}, nil
}
func (m *mockStore) DashboardCostByProject(_ context.Context) ([]dashboard.ProjectCost, error) {
	return []dashboard.ProjectCost{}, nil
}
func (m *mockStore) DashboardCostTrend(_ context.Context, _ int) ([]cost.DailyCost, error) {
	return []cost.DailyCost{}, nil
}

// Project repo lookup
func (m *mockStore) GetProjectByRepoName(_ context.Context, _ string) (*project.Project, error) {
	return nil, nil
}

// Review Policy stubs
func (m *mockStore) CreateReviewPolicy(_ context.Context, _ *review.ReviewPolicy) error {
	return nil
}
func (m *mockStore) GetReviewPolicy(_ context.Context, _ string) (*review.ReviewPolicy, error) {
	return nil, nil
}
func (m *mockStore) ListReviewPoliciesByProject(_ context.Context, _ string) ([]review.ReviewPolicy, error) {
	return nil, nil
}
func (m *mockStore) UpdateReviewPolicy(_ context.Context, _ *review.ReviewPolicy) error {
	return nil
}
func (m *mockStore) DeleteReviewPolicy(_ context.Context, _ string) error { return nil }
func (m *mockStore) ListEnabledPoliciesByTrigger(_ context.Context, _ review.TriggerType) ([]review.ReviewPolicy, error) {
	return nil, nil
}
func (m *mockStore) IncrementCommitCounter(_ context.Context, _ string, _ int) (int, error) {
	return 0, nil
}
func (m *mockStore) ResetCommitCounter(_ context.Context, _ string) error   { return nil }
func (m *mockStore) CreateReview(_ context.Context, _ *review.Review) error { return nil }
func (m *mockStore) GetReview(_ context.Context, _ string) (*review.Review, error) {
	return nil, nil
}
func (m *mockStore) ListReviewsByProject(_ context.Context, _ string) ([]review.Review, error) {
	return nil, nil
}
func (m *mockStore) UpdateReviewStatus(_ context.Context, _ string, _ review.Status, _ *time.Time) error {
	return nil
}
func (m *mockStore) GetReviewByPlanID(_ context.Context, _ string) (*review.Review, error) {
	return nil, nil
}

// Roadmap stubs
func (m *mockStore) CreateRoadmap(_ context.Context, _ roadmap.CreateRoadmapRequest) (*roadmap.Roadmap, error) {
	return &roadmap.Roadmap{}, nil
}
func (m *mockStore) GetRoadmap(_ context.Context, _ string) (*roadmap.Roadmap, error) {
	return nil, domain.ErrNotFound
}
func (m *mockStore) GetRoadmapByProject(_ context.Context, _ string) (*roadmap.Roadmap, error) {
	return nil, domain.ErrNotFound
}
func (m *mockStore) UpdateRoadmap(_ context.Context, _ *roadmap.Roadmap) error { return nil }
func (m *mockStore) DeleteRoadmap(_ context.Context, _ string) error           { return nil }
func (m *mockStore) CreateMilestone(_ context.Context, _ roadmap.CreateMilestoneRequest) (*roadmap.Milestone, error) {
	return &roadmap.Milestone{}, nil
}
func (m *mockStore) GetMilestone(_ context.Context, _ string) (*roadmap.Milestone, error) {
	return nil, domain.ErrNotFound
}
func (m *mockStore) ListMilestones(_ context.Context, _ string) ([]roadmap.Milestone, error) {
	return nil, nil
}
func (m *mockStore) UpdateMilestone(_ context.Context, _ *roadmap.Milestone) error { return nil }
func (m *mockStore) DeleteMilestone(_ context.Context, _ string) error             { return nil }
func (m *mockStore) FindMilestoneByTitle(_ context.Context, _, _ string) (*roadmap.Milestone, error) {
	return nil, domain.ErrNotFound
}
func (m *mockStore) CreateFeature(_ context.Context, _ *roadmap.CreateFeatureRequest) (*roadmap.Feature, error) {
	return &roadmap.Feature{}, nil
}
func (m *mockStore) GetFeature(_ context.Context, _ string) (*roadmap.Feature, error) {
	return nil, domain.ErrNotFound
}
func (m *mockStore) FindFeatureBySpecRef(_ context.Context, _, _ string) (*roadmap.Feature, error) {
	return nil, domain.ErrNotFound
}
func (m *mockStore) ListFeatures(_ context.Context, _ string) ([]roadmap.Feature, error) {
	return nil, nil
}
func (m *mockStore) ListFeaturesByRoadmap(_ context.Context, _ string) ([]roadmap.Feature, error) {
	return nil, nil
}
func (m *mockStore) UpdateFeature(_ context.Context, _ *roadmap.Feature) error { return nil }
func (m *mockStore) DeleteFeature(_ context.Context, _ string) error           { return nil }

// Tenant stubs
func (m *mockStore) CreateTenant(_ context.Context, _ tenant.CreateRequest) (*tenant.Tenant, error) {
	return nil, nil
}
func (m *mockStore) GetTenant(_ context.Context, _ string) (*tenant.Tenant, error) {
	return nil, nil
}
func (m *mockStore) ListTenants(_ context.Context) ([]tenant.Tenant, error) { return nil, nil }
func (m *mockStore) UpdateTenant(_ context.Context, _ *tenant.Tenant) error { return nil }

// Branch Protection Rule stubs
func (m *mockStore) CreateBranchProtectionRule(_ context.Context, _ bp.CreateRuleRequest) (*bp.ProtectionRule, error) {
	return nil, nil
}
func (m *mockStore) GetBranchProtectionRule(_ context.Context, _ string) (*bp.ProtectionRule, error) {
	return nil, nil
}
func (m *mockStore) ListBranchProtectionRules(_ context.Context, _ string) ([]bp.ProtectionRule, error) {
	return nil, nil
}
func (m *mockStore) UpdateBranchProtectionRule(_ context.Context, _ *bp.ProtectionRule) error {
	return nil
}
func (m *mockStore) DeleteBranchProtectionRule(_ context.Context, _ string) error {
	return nil
}

// Session stubs
func (m *mockStore) CreateSession(_ context.Context, _ *run.Session) error { return nil }
func (m *mockStore) GetSession(_ context.Context, _ string) (*run.Session, error) {
	return nil, nil
}
func (m *mockStore) GetSessionByConversation(_ context.Context, _ string) (*run.Session, error) {
	return nil, nil
}
func (m *mockStore) ListSessions(_ context.Context, _ string) ([]run.Session, error) {
	return nil, nil
}
func (m *mockStore) UpdateSessionStatus(_ context.Context, _ string, _ run.SessionStatus, _ string) error {
	return nil
}

// --- User/Auth (in-memory implementation for auth tests) ---

func (m *mockStore) CreateUser(_ context.Context, u *user.User) error {
	for i := range m.users {
		if m.users[i].Email == u.Email && m.users[i].TenantID == u.TenantID {
			return fmt.Errorf("create user: duplicate email %q", u.Email)
		}
	}
	m.users = append(m.users, *u)
	return nil
}

func (m *mockStore) GetUser(_ context.Context, id string) (*user.User, error) {
	for i := range m.users {
		if m.users[i].ID == id {
			return &m.users[i], nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockStore) GetUserByEmail(_ context.Context, email, tenantID string) (*user.User, error) {
	for i := range m.users {
		if m.users[i].Email == email && m.users[i].TenantID == tenantID {
			return &m.users[i], nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockStore) ListUsers(_ context.Context, tenantID string) ([]user.User, error) {
	var result []user.User
	for i := range m.users {
		if m.users[i].TenantID == tenantID {
			result = append(result, m.users[i])
		}
	}
	return result, nil
}

func (m *mockStore) UpdateUser(_ context.Context, u *user.User) error {
	for i := range m.users {
		if m.users[i].ID == u.ID {
			m.users[i] = *u
			return nil
		}
	}
	return domain.ErrNotFound
}

func (m *mockStore) DeleteUser(_ context.Context, id string) error {
	for i := range m.users {
		if m.users[i].ID == id {
			m.users = append(m.users[:i], m.users[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (m *mockStore) CreateRefreshToken(_ context.Context, rt *user.RefreshToken) error {
	m.refreshTokens = append(m.refreshTokens, *rt)
	return nil
}

func (m *mockStore) GetRefreshTokenByHash(_ context.Context, hash string) (*user.RefreshToken, error) {
	for i := range m.refreshTokens {
		if m.refreshTokens[i].TokenHash == hash {
			return &m.refreshTokens[i], nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockStore) DeleteRefreshToken(_ context.Context, id string) error {
	for i := range m.refreshTokens {
		if m.refreshTokens[i].ID == id {
			m.refreshTokens = append(m.refreshTokens[:i], m.refreshTokens[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *mockStore) DeleteRefreshTokensByUser(_ context.Context, userID string) error {
	filtered := m.refreshTokens[:0]
	for _, rt := range m.refreshTokens {
		if rt.UserID != userID {
			filtered = append(filtered, rt)
		}
	}
	m.refreshTokens = filtered
	return nil
}

func (m *mockStore) CreateAPIKey(_ context.Context, key *user.APIKey) error {
	m.apiKeys = append(m.apiKeys, *key)
	return nil
}

func (m *mockStore) GetAPIKeyByHash(_ context.Context, hash string) (*user.APIKey, error) {
	for i := range m.apiKeys {
		if m.apiKeys[i].KeyHash == hash {
			return &m.apiKeys[i], nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockStore) ListAPIKeysByUser(_ context.Context, userID string) ([]user.APIKey, error) {
	var result []user.APIKey
	for i := range m.apiKeys {
		if m.apiKeys[i].UserID == userID {
			result = append(result, m.apiKeys[i])
		}
	}
	return result, nil
}

func (m *mockStore) DeleteAPIKey(_ context.Context, id, _ string) error {
	for i := range m.apiKeys {
		if m.apiKeys[i].ID == id {
			m.apiKeys = append(m.apiKeys[:i], m.apiKeys[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (m *mockStore) RevokeToken(_ context.Context, jti string, expiresAt time.Time) error {
	if m.revokedTokens == nil {
		m.revokedTokens = make(map[string]time.Time)
	}
	m.revokedTokens[jti] = expiresAt
	return nil
}

func (m *mockStore) IsTokenRevoked(_ context.Context, jti string) (bool, error) {
	if m.isTokenRevokedErr != nil {
		return false, m.isTokenRevokedErr
	}
	if m.revokedTokens == nil {
		return false, nil
	}
	_, revoked := m.revokedTokens[jti]
	return revoked, nil
}

func (m *mockStore) PurgeExpiredTokens(_ context.Context) (int64, error) { return 0, nil }

func (m *mockStore) RotateRefreshToken(_ context.Context, oldHash string, newRT *user.RefreshToken) error {
	for i := range m.refreshTokens {
		if m.refreshTokens[i].TokenHash == oldHash {
			m.refreshTokens = append(m.refreshTokens[:i], m.refreshTokens[i+1:]...)
			break
		}
	}
	m.refreshTokens = append(m.refreshTokens, *newRT)
	return nil
}

// Password Reset Token methods
func (m *mockStore) CreatePasswordResetToken(_ context.Context, prt *user.PasswordResetToken) error {
	m.passwordResetTokens = append(m.passwordResetTokens, *prt)
	return nil
}

func (m *mockStore) GetPasswordResetTokenByHash(_ context.Context, hash string) (*user.PasswordResetToken, error) {
	for i := range m.passwordResetTokens {
		if m.passwordResetTokens[i].TokenHash == hash {
			return &m.passwordResetTokens[i], nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockStore) MarkPasswordResetTokenUsed(_ context.Context, id string) error {
	for i := range m.passwordResetTokens {
		if m.passwordResetTokens[i].ID == id {
			m.passwordResetTokens[i].Used = true
			return nil
		}
	}
	return domain.ErrNotFound
}

func (m *mockStore) DeleteExpiredPasswordResetTokens(_ context.Context) (int64, error) {
	return 0, nil
}

// Retrieval Scope stubs
func (m *mockStore) CreateScope(_ context.Context, _ cfcontext.CreateScopeRequest) (*cfcontext.RetrievalScope, error) {
	return nil, nil
}
func (m *mockStore) GetScope(_ context.Context, _ string) (*cfcontext.RetrievalScope, error) {
	return nil, domain.ErrNotFound
}
func (m *mockStore) ListScopes(_ context.Context) ([]cfcontext.RetrievalScope, error) {
	return nil, nil
}
func (m *mockStore) UpdateScope(_ context.Context, _ string, _ cfcontext.UpdateScopeRequest) (*cfcontext.RetrievalScope, error) {
	return nil, nil
}
func (m *mockStore) DeleteScope(_ context.Context, _ string) error { return nil }
func (m *mockStore) ListScopesByProject(_ context.Context, _ string) ([]cfcontext.RetrievalScope, error) {
	return nil, nil
}
func (m *mockStore) GetScopesForProject(_ context.Context, _ string) ([]cfcontext.RetrievalScope, error) {
	return nil, nil
}
func (m *mockStore) AddProjectToScope(_ context.Context, _, _ string) error      { return nil }
func (m *mockStore) RemoveProjectFromScope(_ context.Context, _, _ string) error { return nil }

// Knowledge Base stubs
func (m *mockStore) CreateKnowledgeBase(_ context.Context, _ *knowledgebase.CreateRequest) (*knowledgebase.KnowledgeBase, error) {
	return nil, nil
}
func (m *mockStore) GetKnowledgeBase(_ context.Context, _ string) (*knowledgebase.KnowledgeBase, error) {
	return nil, nil
}
func (m *mockStore) ListKnowledgeBases(_ context.Context) ([]knowledgebase.KnowledgeBase, error) {
	return nil, nil
}
func (m *mockStore) UpdateKnowledgeBase(_ context.Context, _ string, _ knowledgebase.UpdateRequest) (*knowledgebase.KnowledgeBase, error) {
	return nil, nil
}
func (m *mockStore) DeleteKnowledgeBase(_ context.Context, _ string) error { return nil }
func (m *mockStore) UpdateKnowledgeBaseStatus(_ context.Context, _, _ string, _ int) error {
	return nil
}
func (m *mockStore) AddKnowledgeBaseToScope(_ context.Context, _, _ string) error      { return nil }
func (m *mockStore) RemoveKnowledgeBaseFromScope(_ context.Context, _, _ string) error { return nil }
func (m *mockStore) ListKnowledgeBasesByScope(_ context.Context, _ string) ([]knowledgebase.KnowledgeBase, error) {
	return nil, nil
}

// Settings stubs
func (m *mockStore) ListSettings(_ context.Context) ([]settings.Setting, error) { return nil, nil }
func (m *mockStore) GetSetting(_ context.Context, _ string) (*settings.Setting, error) {
	return nil, nil
}
func (m *mockStore) UpsertSetting(_ context.Context, _ string, _ json.RawMessage) error { return nil }

// VCS Account stubs
func (m *mockStore) ListVCSAccounts(_ context.Context) ([]vcsaccount.VCSAccount, error) {
	return nil, nil
}
func (m *mockStore) CreateVCSAccount(_ context.Context, _ *vcsaccount.VCSAccount) (*vcsaccount.VCSAccount, error) {
	return nil, nil
}
func (m *mockStore) GetVCSAccount(_ context.Context, _ string) (*vcsaccount.VCSAccount, error) {
	return nil, nil
}
func (m *mockStore) DeleteVCSAccount(_ context.Context, _ string) error { return nil }

// OAuth State stubs
func (m *mockStore) CreateOAuthState(_ context.Context, _ *vcsaccount.OAuthState) error {
	return nil
}
func (m *mockStore) GetOAuthState(_ context.Context, _ string) (*vcsaccount.OAuthState, error) {
	return nil, domain.ErrNotFound
}
func (m *mockStore) DeleteOAuthState(_ context.Context, _ string) error        { return nil }
func (m *mockStore) DeleteExpiredOAuthStates(_ context.Context) (int64, error) { return 0, nil }

// Conversation stubs
func (m *mockStore) CreateConversation(_ context.Context, _ *conversation.Conversation) (*conversation.Conversation, error) {
	return nil, nil
}
func (m *mockStore) GetConversation(_ context.Context, _ string) (*conversation.Conversation, error) {
	return nil, nil
}
func (m *mockStore) ListConversationsByProject(_ context.Context, _ string) ([]conversation.Conversation, error) {
	return nil, nil
}
func (m *mockStore) DeleteConversation(_ context.Context, _ string) error { return nil }
func (m *mockStore) CreateMessage(_ context.Context, _ *conversation.Message) (*conversation.Message, error) {
	return nil, nil
}
func (m *mockStore) CreateToolMessages(_ context.Context, _ string, _ []conversation.Message) error {
	return nil
}
func (m *mockStore) ListMessages(_ context.Context, _ string) ([]conversation.Message, error) {
	return nil, nil
}
func (m *mockStore) DeleteConversationMessages(_ context.Context, _ string) error { return nil }
func (m *mockStore) UpdateConversationMode(_ context.Context, _, _ string) error  { return nil }
func (m *mockStore) UpdateConversationModel(_ context.Context, _, _ string) error { return nil }
func (m *mockStore) SearchConversationMessages(_ context.Context, _ string, _ []string, _ int) ([]conversation.Message, error) {
	return nil, nil
}

// MCP Servers
func (m *mockStore) CreateMCPServer(_ context.Context, _ *mcp.ServerDef) error { return nil }
func (m *mockStore) GetMCPServer(_ context.Context, _ string) (*mcp.ServerDef, error) {
	return nil, nil
}
func (m *mockStore) ListMCPServers(_ context.Context) ([]mcp.ServerDef, error) { return nil, nil }
func (m *mockStore) UpdateMCPServer(_ context.Context, _ *mcp.ServerDef) error { return nil }
func (m *mockStore) DeleteMCPServer(_ context.Context, _ string) error         { return nil }
func (m *mockStore) UpdateMCPServerStatus(_ context.Context, _ string, _ mcp.ServerStatus) error {
	return nil
}
func (m *mockStore) AssignMCPServerToProject(_ context.Context, _, _ string) error     { return nil }
func (m *mockStore) UnassignMCPServerFromProject(_ context.Context, _, _ string) error { return nil }
func (m *mockStore) ListMCPServersByProject(_ context.Context, _ string) ([]mcp.ServerDef, error) {
	return nil, nil
}
func (m *mockStore) UpsertMCPServerTools(_ context.Context, _ string, _ []mcp.ServerTool) error {
	return nil
}
func (m *mockStore) ListMCPServerTools(_ context.Context, _ string) ([]mcp.ServerTool, error) {
	return nil, nil
}

// --- Prompt Section stub methods (satisfy database.Store interface) ---
func (m *mockStore) ListPromptSections(_ context.Context, _ string) ([]prompt.SectionRow, error) {
	return nil, nil
}
func (m *mockStore) UpsertPromptSection(_ context.Context, _ *prompt.SectionRow) error { return nil }
func (m *mockStore) DeletePromptSection(_ context.Context, _ string) error             { return nil }

// Benchmark suite stubs (Phase 26)
func (m *mockStore) CreateBenchmarkSuite(_ context.Context, _ *benchmark.Suite) error { return nil }
func (m *mockStore) GetBenchmarkSuite(_ context.Context, _ string) (*benchmark.Suite, error) {
	return nil, nil
}
func (m *mockStore) ListBenchmarkSuites(_ context.Context) ([]benchmark.Suite, error) {
	return nil, nil
}
func (m *mockStore) DeleteBenchmarkSuite(_ context.Context, _ string) error           { return nil }
func (m *mockStore) UpdateBenchmarkSuite(_ context.Context, _ *benchmark.Suite) error { return nil }
func (m *mockStore) ListBenchmarkRunsFiltered(_ context.Context, _ *benchmark.RunFilter) ([]benchmark.Run, error) {
	return nil, nil
}

// Benchmark stubs
func (m *mockStore) CreateBenchmarkRun(_ context.Context, _ *benchmark.Run) error { return nil }
func (m *mockStore) GetBenchmarkRun(_ context.Context, _ string) (*benchmark.Run, error) {
	return nil, nil
}
func (m *mockStore) ListBenchmarkRuns(_ context.Context) ([]benchmark.Run, error) { return nil, nil }
func (m *mockStore) UpdateBenchmarkRun(_ context.Context, _ *benchmark.Run) error { return nil }
func (m *mockStore) DeleteBenchmarkRun(_ context.Context, _ string) error         { return nil }
func (m *mockStore) CreateBenchmarkResult(_ context.Context, _ *benchmark.Result) error {
	return nil
}
func (m *mockStore) ListBenchmarkResults(_ context.Context, _ string) ([]benchmark.Result, error) {
	return nil, nil
}

// Experience Pool stubs.
func (m *mockStore) CreateExperienceEntry(_ context.Context, _ *experience.Entry) error {
	return nil
}
func (m *mockStore) GetExperienceEntry(_ context.Context, _ string) (*experience.Entry, error) {
	return nil, nil
}
func (m *mockStore) ListExperienceEntries(_ context.Context, _ string) ([]experience.Entry, error) {
	return nil, nil
}
func (m *mockStore) DeleteExperienceEntry(_ context.Context, _ string) error { return nil }
func (m *mockStore) UpdateExperienceHit(_ context.Context, _ string) error   { return nil }

// Agent Memory stubs.
func (m *mockStore) CreateMemory(_ context.Context, _ *memory.Memory) error { return nil }
func (m *mockStore) ListMemories(_ context.Context, _ string) ([]memory.Memory, error) {
	return nil, nil
}

// Microagent stubs.
func (m *mockStore) CreateMicroagent(_ context.Context, _ *microagent.Microagent) error { return nil }
func (m *mockStore) GetMicroagent(_ context.Context, _ string) (*microagent.Microagent, error) {
	return nil, nil
}
func (m *mockStore) ListMicroagents(_ context.Context, _ string) ([]microagent.Microagent, error) {
	return nil, nil
}
func (m *mockStore) UpdateMicroagent(_ context.Context, _ *microagent.Microagent) error { return nil }
func (m *mockStore) DeleteMicroagent(_ context.Context, _ string) error                 { return nil }

// Skill stubs.
func (m *mockStore) CreateSkill(_ context.Context, _ *skill.Skill) error           { return nil }
func (m *mockStore) GetSkill(_ context.Context, _ string) (*skill.Skill, error)    { return nil, nil }
func (m *mockStore) ListSkills(_ context.Context, _ string) ([]skill.Skill, error) { return nil, nil }
func (m *mockStore) UpdateSkill(_ context.Context, _ *skill.Skill) error           { return nil }
func (m *mockStore) DeleteSkill(_ context.Context, _ string) error                 { return nil }
func (m *mockStore) IncrementSkillUsage(_ context.Context, _ string) error         { return nil }
func (m *mockStore) ListActiveSkills(_ context.Context, _ string) ([]skill.Skill, error) {
	return nil, nil
}

// Feedback Audit stubs.
func (m *mockStore) CreateFeedbackAudit(_ context.Context, _ *feedback.AuditEntry) error {
	return nil
}
func (m *mockStore) ListFeedbackByRun(_ context.Context, _ string) ([]feedback.AuditEntry, error) {
	return nil, nil
}

// Auto-Agent stubs
func (m *mockStore) UpsertAutoAgent(_ context.Context, _ *autoagent.AutoAgent) error { return nil }
func (m *mockStore) GetAutoAgent(_ context.Context, _ string) (*autoagent.AutoAgent, error) {
	return nil, nil
}
func (m *mockStore) UpdateAutoAgentStatus(_ context.Context, _ string, _ autoagent.Status, _ string) error {
	return nil
}
func (m *mockStore) UpdateAutoAgentProgress(_ context.Context, _ *autoagent.AutoAgent) error {
	return nil
}
func (m *mockStore) DeleteAutoAgent(_ context.Context, _ string) error { return nil }

// Quarantine (Phase 23B)
func (m *mockStore) QuarantineMessage(_ context.Context, _ *quarantine.Message) error { return nil }
func (m *mockStore) GetQuarantinedMessage(_ context.Context, _ string) (*quarantine.Message, error) {
	return nil, domain.ErrNotFound
}
func (m *mockStore) ListQuarantinedMessages(_ context.Context, _ string, _ quarantine.Status, _, _ int) ([]*quarantine.Message, error) {
	return nil, nil
}
func (m *mockStore) UpdateQuarantineStatus(_ context.Context, _ string, _ quarantine.Status, _, _ string) error {
	return nil
}

// Agent Identity (Phase 23C)
func (m *mockStore) IncrementAgentStats(_ context.Context, id string, costDelta float64, success bool) error {
	for i := range m.agents {
		if m.agents[i].ID != id {
			continue
		}
		m.agents[i].TotalRuns++
		m.agents[i].TotalCost += costDelta
		runs := float64(m.agents[i].TotalRuns)
		s := 0.0
		if success {
			s = 1.0
		}
		m.agents[i].SuccessRate = (m.agents[i].SuccessRate*(runs-1) + s) / runs
		return nil
	}
	return domain.ErrNotFound
}
func (m *mockStore) UpdateAgentState(_ context.Context, id string, state map[string]string) error {
	for i := range m.agents {
		if m.agents[i].ID == id {
			m.agents[i].State = state
			return nil
		}
	}
	return domain.ErrNotFound
}
func (m *mockStore) SendAgentMessage(_ context.Context, msg *agent.InboxMessage) error {
	m.inboxNextID++
	msg.ID = fmt.Sprintf("msg-%d", m.inboxNextID)
	m.inboxMessages = append(m.inboxMessages, *msg)
	return nil
}
func (m *mockStore) ListAgentInbox(_ context.Context, agentID string, unreadOnly bool) ([]agent.InboxMessage, error) {
	var result []agent.InboxMessage
	for _, msg := range m.inboxMessages {
		if msg.AgentID == agentID && (!unreadOnly || !msg.Read) {
			result = append(result, msg)
		}
	}
	return result, nil
}
func (m *mockStore) MarkInboxRead(_ context.Context, messageID string) error {
	for i := range m.inboxMessages {
		if m.inboxMessages[i].ID == messageID {
			m.inboxMessages[i].Read = true
			return nil
		}
	}
	return domain.ErrNotFound
}

// Phase 24: Active Work Visibility mock methods.

func (m *mockStore) ListActiveWork(_ context.Context, _ string) ([]task.ActiveWorkItem, error) {
	return nil, nil
}

func (m *mockStore) ClaimTask(_ context.Context, _, _ string, _ int) (*task.ClaimResult, error) {
	return &task.ClaimResult{Claimed: false, Reason: "not implemented in mock"}, nil
}

func (m *mockStore) ReleaseStaleWork(_ context.Context, _ time.Duration) ([]task.Task, error) {
	return nil, nil
}

// --- ProjectService Tests ---

func TestProjectServiceList(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{
			{ID: "p1", Name: "Alpha"},
			{ID: "p2", Name: "Beta"},
		},
	}
	svc := NewProjectService(store, t.TempDir())

	got, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(got))
	}
}

func TestProjectServiceListError(t *testing.T) {
	store := &mockStore{listProjectsErr: errors.New("db down")}
	svc := NewProjectService(store, t.TempDir())

	_, err := svc.List(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestProjectServiceGet(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "Alpha"}},
	}
	svc := NewProjectService(store, t.TempDir())

	p, err := svc.Get(context.Background(), "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "Alpha" {
		t.Fatalf("expected 'Alpha', got %q", p.Name)
	}
}

func TestProjectServiceGetNotFound(t *testing.T) {
	store := &mockStore{}
	svc := NewProjectService(store, t.TempDir())

	_, err := svc.Get(context.Background(), "nonexistent")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestProjectServiceCreate(t *testing.T) {
	store := &mockStore{}
	svc := NewProjectService(store, t.TempDir())

	req := &project.CreateRequest{Name: "New", Provider: "local"}
	p, err := svc.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "New" {
		t.Fatalf("expected 'New', got %q", p.Name)
	}
	if len(store.projects) != 1 {
		t.Fatalf("expected 1 project in store, got %d", len(store.projects))
	}
}

func TestProjectServiceCreateError(t *testing.T) {
	store := &mockStore{createProjectErr: errors.New("constraint violation")}
	svc := NewProjectService(store, t.TempDir())

	_, err := svc.Create(context.Background(), &project.CreateRequest{Name: "X"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestProjectServiceDelete(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "Alpha"}},
	}
	svc := NewProjectService(store, t.TempDir())

	if err := svc.Delete(context.Background(), "p1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.projects) != 0 {
		t.Fatalf("expected 0 projects after delete, got %d", len(store.projects))
	}
}

func TestProjectServiceDeleteNotFound(t *testing.T) {
	store := &mockStore{}
	svc := NewProjectService(store, t.TempDir())

	err := svc.Delete(context.Background(), "nonexistent")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestProjectServiceCloneNoRepoURL(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "No Repo", Provider: "local"}},
	}
	svc := NewProjectService(store, t.TempDir())

	_, err := svc.Clone(context.Background(), "p1", "test-tenant", "")
	if err == nil {
		t.Fatal("expected error for project without repo_url")
	}
	if got := err.Error(); got != "project p1 has no repo_url" {
		t.Fatalf("unexpected error message: %s", got)
	}
}

func TestProjectServiceCloneNotFound(t *testing.T) {
	store := &mockStore{}
	svc := NewProjectService(store, t.TempDir())

	_, err := svc.Clone(context.Background(), "nonexistent", "test-tenant", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestProjectServiceStatusNoWorkspace(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "No WS"}},
	}
	svc := NewProjectService(store, t.TempDir())

	_, err := svc.Status(context.Background(), "p1")
	if err == nil {
		t.Fatal("expected error for project without workspace")
	}
}

func TestProjectServicePullNoWorkspace(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "No WS"}},
	}
	svc := NewProjectService(store, t.TempDir())

	err := svc.Pull(context.Background(), "p1")
	if err == nil {
		t.Fatal("expected error for project without workspace")
	}
}

func TestProjectServiceListBranchesNoWorkspace(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "No WS"}},
	}
	svc := NewProjectService(store, t.TempDir())

	_, err := svc.ListBranches(context.Background(), "p1")
	if err == nil {
		t.Fatal("expected error for project without workspace")
	}
}

func TestProjectServiceCheckoutNoWorkspace(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "No WS"}},
	}
	svc := NewProjectService(store, t.TempDir())

	err := svc.Checkout(context.Background(), "p1", "main")
	if err == nil {
		t.Fatal("expected error for project without workspace")
	}
}

func TestProjectServiceDeleteCleansUpWorkspace(t *testing.T) {
	wsRoot := t.TempDir()
	wsDir := filepath.Join(wsRoot, "tenant", "p1")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create a file inside so we can verify removal.
	if err := os.WriteFile(filepath.Join(wsDir, "test.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "Alpha", WorkspacePath: wsDir}},
	}
	svc := NewProjectService(store, wsRoot)

	if err := svc.Delete(context.Background(), "p1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Workspace directory should be removed.
	if _, err := os.Stat(wsDir); !os.IsNotExist(err) {
		t.Fatalf("expected workspace directory to be removed, got err: %v", err)
	}
}

func TestProjectServiceDeleteSkipsOutsideRoot(t *testing.T) {
	wsRoot := t.TempDir()
	outsideDir := t.TempDir() // separate temp dir, not under wsRoot

	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "Alpha", WorkspacePath: outsideDir}},
	}
	svc := NewProjectService(store, wsRoot)

	if err := svc.Delete(context.Background(), "p1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Directory outside workspace root should NOT be removed.
	if _, err := os.Stat(outsideDir); err != nil {
		t.Fatalf("directory outside workspace root should not be removed: %v", err)
	}
}

func TestProjectService_IsUnderWorkspaceRoot(t *testing.T) {
	wsRoot := t.TempDir()
	// Create a real subdirectory so EvalSymlinks can resolve it.
	subDir := filepath.Join(wsRoot, "project-1")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	svc := &ProjectService{workspaceRoot: wsRoot}

	if !svc.isUnderWorkspaceRoot(subDir) {
		t.Error("expected path under workspace root to be accepted")
	}
	if svc.isUnderWorkspaceRoot("") {
		t.Error("expected empty path to be rejected")
	}
	if svc.isUnderWorkspaceRoot("/etc/passwd") {
		t.Error("expected /etc/passwd to be rejected (not under workspace root)")
	}
	if svc.isUnderWorkspaceRoot("/nonexistent/path/12345") {
		t.Error("expected nonexistent path to be rejected")
	}

	// Verify the workspace root itself is NOT accepted (must be strictly under).
	if svc.isUnderWorkspaceRoot(wsRoot) {
		t.Error("expected workspace root itself to be rejected")
	}

	// Test with empty workspace root.
	svcEmpty := &ProjectService{workspaceRoot: ""}
	if svcEmpty.isUnderWorkspaceRoot(subDir) {
		t.Error("expected any path to be rejected when workspace root is empty")
	}
}

func TestProjectServiceAdopt(t *testing.T) {
	adoptDir := t.TempDir()
	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "Alpha"}},
	}
	svc := NewProjectService(store, t.TempDir())

	p, err := svc.Adopt(context.Background(), "p1", adoptDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.WorkspacePath != adoptDir {
		t.Fatalf("expected workspace path %q, got %q", adoptDir, p.WorkspacePath)
	}
}

func TestProjectServiceAdoptEmptyPath(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "Alpha"}},
	}
	svc := NewProjectService(store, t.TempDir())

	_, err := svc.Adopt(context.Background(), "p1", "")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestProjectServiceAdoptNonexistentDir(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "Alpha"}},
	}
	svc := NewProjectService(store, t.TempDir())

	_, err := svc.Adopt(context.Background(), "p1", "/nonexistent/path/12345")
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

func TestProjectServiceWorkspaceHealthExisting(t *testing.T) {
	wsDir := t.TempDir()
	// Create a .git directory and a file.
	if err := os.MkdirAll(filepath.Join(wsDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "Alpha", WorkspacePath: wsDir}},
	}
	svc := NewProjectService(store, t.TempDir())

	info, err := svc.WorkspaceHealth(context.Background(), "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.Exists {
		t.Fatal("expected Exists=true")
	}
	if !info.GitRepo {
		t.Fatal("expected GitRepo=true")
	}
	if info.DiskUsageBytes == 0 {
		t.Fatal("expected non-zero disk usage")
	}
	if info.Path != wsDir {
		t.Fatalf("expected path %q, got %q", wsDir, info.Path)
	}
}

func TestProjectServiceWorkspaceHealthMissing(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "Alpha"}},
	}
	svc := NewProjectService(store, t.TempDir())

	info, err := svc.WorkspaceHealth(context.Background(), "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Exists {
		t.Fatal("expected Exists=false for project without workspace")
	}
}

// --- Routing stubs (Phase 26) ---

func (m *mockStore) CreateRoutingOutcome(_ context.Context, _ *routing.RoutingOutcome) error {
	return nil
}
func (m *mockStore) ListRoutingStats(_ context.Context, _, _ string) ([]routing.ModelPerformanceStats, error) {
	return nil, nil
}
func (m *mockStore) UpsertRoutingStats(_ context.Context, _ *routing.ModelPerformanceStats) error {
	return nil
}
func (m *mockStore) AggregateRoutingOutcomes(_ context.Context) error { return nil }
func (m *mockStore) ListRoutingOutcomes(_ context.Context, _ int) ([]routing.RoutingOutcome, error) {
	return nil, nil
}

// A2A stubs (Phase 27)
func (m *mockStore) CreateA2ATask(_ context.Context, _ *a2adomain.A2ATask) error { return nil }
func (m *mockStore) GetA2ATask(_ context.Context, _ string) (*a2adomain.A2ATask, error) {
	return nil, domain.ErrNotFound
}
func (m *mockStore) UpdateA2ATask(_ context.Context, _ *a2adomain.A2ATask) error { return nil }
func (m *mockStore) ListA2ATasks(_ context.Context, _ *database.A2ATaskFilter) ([]a2adomain.A2ATask, int, error) {
	return nil, 0, nil
}
func (m *mockStore) DeleteA2ATask(_ context.Context, _ string) error { return nil }
func (m *mockStore) CreateRemoteAgent(_ context.Context, _ *a2adomain.RemoteAgent) error {
	return nil
}
func (m *mockStore) GetRemoteAgent(_ context.Context, _ string) (*a2adomain.RemoteAgent, error) {
	return nil, domain.ErrNotFound
}
func (m *mockStore) ListRemoteAgents(_ context.Context, _ string, _ bool) ([]a2adomain.RemoteAgent, error) {
	return nil, nil
}
func (m *mockStore) UpdateRemoteAgent(_ context.Context, _ *a2adomain.RemoteAgent) error {
	return nil
}
func (m *mockStore) DeleteRemoteAgent(_ context.Context, _ string) error { return nil }
func (m *mockStore) CreateA2APushConfig(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}
func (m *mockStore) GetA2APushConfig(_ context.Context, _ string) (_, _, _ string, _ error) {
	return "", "", "", nil
}
func (m *mockStore) ListA2APushConfigs(_ context.Context, _ string) ([]database.A2APushConfig, error) {
	return nil, nil
}
func (m *mockStore) DeleteA2APushConfig(_ context.Context, _ string) error     { return nil }
func (m *mockStore) DeleteAllA2APushConfigs(_ context.Context, _ string) error { return nil }

// Project Goals
func (m *mockStore) CreateProjectGoal(_ context.Context, _ *goal.ProjectGoal) error { return nil }
func (m *mockStore) GetProjectGoal(_ context.Context, _ string) (*goal.ProjectGoal, error) {
	return nil, nil
}
func (m *mockStore) ListProjectGoals(_ context.Context, _ string) ([]goal.ProjectGoal, error) {
	return nil, nil
}
func (m *mockStore) ListEnabledGoals(_ context.Context, _ string) ([]goal.ProjectGoal, error) {
	return nil, nil
}
func (m *mockStore) UpdateProjectGoal(_ context.Context, _ *goal.ProjectGoal) error  { return nil }
func (m *mockStore) DeleteProjectGoal(_ context.Context, _ string) error             { return nil }
func (m *mockStore) DeleteProjectGoalsBySource(_ context.Context, _, _ string) error { return nil }

// LLM Key stubs
func (m *mockStore) CreateLLMKey(_ context.Context, _ *llmkey.LLMKey) error { return nil }
func (m *mockStore) ListLLMKeysByUser(_ context.Context, _ string) ([]llmkey.LLMKey, error) {
	return nil, nil
}
func (m *mockStore) GetLLMKeyByUserProvider(_ context.Context, _, _ string) (*llmkey.LLMKey, error) {
	return nil, nil
}
func (m *mockStore) DeleteLLMKey(_ context.Context, _, _ string) error { return nil }

// Channel stubs
func (m *mockStore) CreateChannel(_ context.Context, _ *channel.Channel) (*channel.Channel, error) {
	return nil, nil
}
func (m *mockStore) GetChannel(_ context.Context, _ string) (*channel.Channel, error) {
	return nil, nil
}
func (m *mockStore) ListChannels(_ context.Context, _ string) ([]channel.Channel, error) {
	return nil, nil
}
func (m *mockStore) DeleteChannel(_ context.Context, _ string) error { return nil }
func (m *mockStore) CreateChannelMessage(_ context.Context, _ *channel.Message) (*channel.Message, error) {
	return nil, nil
}
func (m *mockStore) ListChannelMessages(_ context.Context, _, _ string, _ int) ([]channel.Message, error) {
	return nil, nil
}
func (m *mockStore) AddChannelMember(_ context.Context, _ *channel.Member) error { return nil }
func (m *mockStore) UpdateChannelMemberNotify(_ context.Context, _, _ string, _ channel.NotifySetting) error {
	return nil
}

// Boundary stubs (Phase 31)
func (m *mockStore) GetProjectBoundaries(_ context.Context, _ string) (*boundary.ProjectBoundaryConfig, error) {
	return nil, domain.ErrNotFound
}
func (m *mockStore) UpsertProjectBoundaries(_ context.Context, _ *boundary.ProjectBoundaryConfig) error {
	return nil
}
func (m *mockStore) DeleteProjectBoundaries(_ context.Context, _ string) error { return nil }

// Review Trigger stubs (Phase 31)
func (m *mockStore) CreateReviewTrigger(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}
func (m *mockStore) FindRecentReviewTrigger(_ context.Context, _, _ string, _ time.Duration) (bool, error) {
	return false, nil
}
func (m *mockStore) InsertAuditEntry(_ context.Context, _ *database.AuditEntry) error {
	return nil
}
func (m *mockStore) ListAuditEntries(_ context.Context, _ string, _, _ int) ([]database.AuditEntry, error) {
	return nil, nil
}

// --- InitWorkspace tests ---

func TestProjectServiceInitWorkspace(t *testing.T) {
	wsRoot := t.TempDir()
	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "Empty Project"}},
	}
	svc := NewProjectService(store, wsRoot)

	p, err := svc.InitWorkspace(context.Background(), "p1", "test-tenant")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedPath := filepath.Join(wsRoot, "test-tenant", "p1")
	if p.WorkspacePath != expectedPath {
		t.Fatalf("expected workspace path %q, got %q", expectedPath, p.WorkspacePath)
	}

	// Verify directory exists.
	info, statErr := os.Stat(expectedPath)
	if statErr != nil {
		t.Fatalf("workspace directory does not exist: %v", statErr)
	}
	if !info.IsDir() {
		t.Fatal("workspace path is not a directory")
	}

	// Verify git init was run.
	gitDir := filepath.Join(expectedPath, ".git")
	if _, gitErr := os.Stat(gitDir); gitErr != nil {
		t.Fatalf(".git directory does not exist: %v", gitErr)
	}
}

func TestProjectServiceInitWorkspaceAlreadyHasWorkspace(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "Has WS", WorkspacePath: "/existing/path"}},
	}
	svc := NewProjectService(store, t.TempDir())

	_, err := svc.InitWorkspace(context.Background(), "p1", "tenant")
	if err == nil {
		t.Fatal("expected error for project with existing workspace")
	}
}

func TestProjectServiceInitWorkspaceNotFound(t *testing.T) {
	store := &mockStore{}
	svc := NewProjectService(store, t.TempDir())

	_, err := svc.InitWorkspace(context.Background(), "nonexistent", "tenant")
	if err == nil {
		t.Fatal("expected error for nonexistent project")
	}
}

func TestProjectServiceInitWorkspaceUpdateFails(t *testing.T) {
	store := &mockStore{
		projects:         []project.Project{{ID: "p1", Name: "Fail Update"}},
		updateProjectErr: errors.New("db write failed"),
	}
	svc := NewProjectService(store, t.TempDir())

	_, err := svc.InitWorkspace(context.Background(), "p1", "tenant")
	if err == nil {
		t.Fatal("expected error when UpdateProject fails")
	}
}

func TestSetPolicyProfile_Success(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "Test"}},
	}
	svc := NewProjectService(store, t.TempDir())

	err := svc.SetPolicyProfile(context.Background(), "p1", "my-custom-policy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify the project's PolicyProfile was updated in the store
	if store.projects[0].PolicyProfile != "my-custom-policy" {
		t.Errorf("expected PolicyProfile %q, got %q", "my-custom-policy", store.projects[0].PolicyProfile)
	}
}

func TestSetPolicyProfile_ProjectNotFound(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{},
	}
	svc := NewProjectService(store, t.TempDir())

	err := svc.SetPolicyProfile(context.Background(), "nonexistent", "my-custom-policy")
	if err == nil {
		t.Fatal("expected error for nonexistent project")
	}
}

func TestSetupProjectPersistsDetectedLanguages(t *testing.T) {
	// Create a temp workspace with a go.mod file so ScanWorkspace detects Go.
	wsDir := t.TempDir()
	goModPath := filepath.Join(wsDir, "go.mod")
	if err := os.WriteFile(goModPath, []byte("module example.com/test\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	store := &mockStore{
		projects: []project.Project{{
			ID:            "p1",
			Name:          "Go Project",
			WorkspacePath: wsDir,
		}},
	}
	svc := NewProjectService(store, t.TempDir())

	result, err := svc.SetupProject(context.Background(), "p1", "tenant", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.StackDetected {
		t.Fatal("expected StackDetected to be true")
	}

	// Verify the project's Config was updated with detected_languages.
	updated := store.projects[0]
	if updated.Config == nil {
		t.Fatal("expected Config to be non-nil after stack detection")
	}
	langJSON, ok := updated.Config["detected_languages"]
	if !ok {
		t.Fatal("expected detected_languages key in Config")
	}
	if !strings.Contains(langJSON, `"go"`) {
		t.Errorf("expected detected_languages to contain \"go\", got %s", langJSON)
	}

	// Verify it round-trips as valid JSON.
	var langs []project.Language
	if jsonErr := json.Unmarshal([]byte(langJSON), &langs); jsonErr != nil {
		t.Fatalf("detected_languages is not valid JSON: %v", jsonErr)
	}
	if len(langs) == 0 {
		t.Fatal("expected at least one language in detected_languages")
	}
	found := false
	for _, lang := range langs {
		if lang.Name == "go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected language 'go' in detected_languages, got %v", langs)
	}
}

func TestSetupProjectPersistsDetectedLanguagesUpdateFails(t *testing.T) {
	// Even when UpdateProject fails, SetupProject should still succeed
	// (the persist is best-effort, logged as warning).
	wsDir := t.TempDir()
	goModPath := filepath.Join(wsDir, "go.mod")
	if err := os.WriteFile(goModPath, []byte("module example.com/test\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	store := &mockStore{
		projects: []project.Project{{
			ID:            "p1",
			Name:          "Go Project",
			WorkspacePath: wsDir,
		}},
		updateProjectErr: errors.New("db write failed"),
	}
	svc := NewProjectService(store, t.TempDir())

	result, err := svc.SetupProject(context.Background(), "p1", "tenant", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Setup should still report stack detected even though persist failed.
	if !result.StackDetected {
		t.Fatal("expected StackDetected to be true despite update failure")
	}
}

func TestIsAllowedGiteaHost(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		allowed bool
	}{
		{"loopback v4", "127.0.0.1", false},
		{"loopback v6", "::1", false},
		{"private 10.x", "10.0.0.1", false},
		{"private 172.16.x", "172.16.0.1", false},
		{"private 192.168.x", "192.168.1.1", false},
		{"AWS metadata", "169.254.169.254", false},
		{"link-local", "169.254.1.1", false},
		{"localhost", "localhost", false},
		{"host with port", "127.0.0.1:3000", false},
		{"public IP allowed", "8.8.8.8", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAllowedGiteaHost(context.Background(), tt.host)
			if got != tt.allowed {
				t.Errorf("isAllowedGiteaHost(%q) = %v, want %v", tt.host, got, tt.allowed)
			}
		})
	}
}

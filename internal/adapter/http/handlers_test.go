package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	bp "github.com/Strob0t/CodeForge/internal/domain/branchprotection"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/cost"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/knowledgebase"
	"github.com/Strob0t/CodeForge/internal/domain/mcp"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/prompt"
	"github.com/Strob0t/CodeForge/internal/domain/resource"
	"github.com/Strob0t/CodeForge/internal/domain/review"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/settings"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/domain/tenant"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/domain/vcsaccount"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// mockStore implements database.Store for testing.
type mockStore struct {
	projects []project.Project
	agents   []agent.Agent
	tasks    []task.Task
	runs     []run.Run
}

func (m *mockStore) ListProjects(_ context.Context) ([]project.Project, error) {
	return m.projects, nil
}

func (m *mockStore) GetProject(_ context.Context, id string) (*project.Project, error) {
	for i := range m.projects {
		if m.projects[i].ID == id {
			return &m.projects[i], nil
		}
	}
	return nil, errNotFound
}

func (m *mockStore) CreateProject(_ context.Context, req *project.CreateRequest) (*project.Project, error) {
	p := project.Project{
		ID:       "test-id",
		Name:     req.Name,
		Provider: req.Provider,
	}
	m.projects = append(m.projects, p)
	return &p, nil
}

func (m *mockStore) UpdateProject(_ context.Context, p *project.Project) error {
	for i := range m.projects {
		if m.projects[i].ID == p.ID {
			m.projects[i] = *p
			return nil
		}
	}
	return errNotFound
}

func (m *mockStore) DeleteProject(_ context.Context, id string) error {
	for i := range m.projects {
		if m.projects[i].ID == id {
			m.projects = append(m.projects[:i], m.projects[i+1:]...)
			return nil
		}
	}
	return errNotFound
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
	return nil, errNotFound
}

func (m *mockStore) CreateAgent(_ context.Context, projectID, name, backend string, cfg map[string]string, limits *resource.Limits) (*agent.Agent, error) {
	a := agent.Agent{
		ID:             "agent-id",
		ProjectID:      projectID,
		Name:           name,
		Backend:        backend,
		Status:         agent.StatusIdle,
		Config:         cfg,
		ResourceLimits: limits,
	}
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
	return errNotFound
}

func (m *mockStore) DeleteAgent(_ context.Context, id string) error {
	for i := range m.agents {
		if m.agents[i].ID == id {
			m.agents = append(m.agents[:i], m.agents[i+1:]...)
			return nil
		}
	}
	return errNotFound
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
	return nil, errNotFound
}

func (m *mockStore) CreateTask(_ context.Context, req task.CreateRequest) (*task.Task, error) {
	t := task.Task{
		ID:        "task-id",
		ProjectID: req.ProjectID,
		Title:     req.Title,
		Status:    task.StatusPending,
	}
	m.tasks = append(m.tasks, t)
	return &t, nil
}

func (m *mockStore) UpdateTaskStatus(_ context.Context, _ string, _ task.Status) error {
	return nil
}

func (m *mockStore) UpdateTaskResult(_ context.Context, _ string, _ task.Result, _ float64) error {
	return nil
}

// --- Run methods ---

func (m *mockStore) CreateRun(_ context.Context, r *run.Run) error {
	if r.ID == "" {
		r.ID = "run-id"
	}
	m.runs = append(m.runs, *r)
	return nil
}

func (m *mockStore) GetRun(_ context.Context, id string) (*run.Run, error) {
	for i := range m.runs {
		if m.runs[i].ID == id {
			return &m.runs[i], nil
		}
	}
	return nil, errNotFound
}

func (m *mockStore) UpdateRunStatus(_ context.Context, id string, status run.Status, stepCount int, costUSD float64, tokensIn, tokensOut int64) error {
	for i := range m.runs {
		if m.runs[i].ID != id {
			continue
		}
		m.runs[i].Status = status
		m.runs[i].StepCount = stepCount
		m.runs[i].CostUSD = costUSD
		m.runs[i].TokensIn = tokensIn
		m.runs[i].TokensOut = tokensOut
		return nil
	}
	return errNotFound
}

func (m *mockStore) CompleteRun(_ context.Context, id string, status run.Status, output, errMsg string, costUSD float64, stepCount int, tokensIn, tokensOut int64, model string) error {
	for i := range m.runs {
		if m.runs[i].ID != id {
			continue
		}
		m.runs[i].Status = status
		m.runs[i].Output = output
		m.runs[i].Error = errMsg
		m.runs[i].CostUSD = costUSD
		m.runs[i].StepCount = stepCount
		m.runs[i].TokensIn = tokensIn
		m.runs[i].TokensOut = tokensOut
		m.runs[i].Model = model
		return nil
	}
	return errNotFound
}

func (m *mockStore) UpdateRunArtifact(_ context.Context, _, _ string, _ *bool, _ []string) error {
	return nil
}

func (m *mockStore) ListRunsByTask(_ context.Context, taskID string) ([]run.Run, error) {
	var result []run.Run
	for i := range m.runs {
		if m.runs[i].TaskID == taskID {
			result = append(result, m.runs[i])
		}
	}
	return result, nil
}

// --- Plan stub methods (satisfy database.Store interface) ---

func (m *mockStore) CreatePlan(_ context.Context, _ *plan.ExecutionPlan) error { return nil }
func (m *mockStore) GetPlan(_ context.Context, _ string) (*plan.ExecutionPlan, error) {
	return nil, errNotFound
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
	return nil, errNotFound
}
func (m *mockStore) UpdatePlanStepRound(_ context.Context, _ string, _ int) error { return nil }

// --- Agent Team stub methods (satisfy database.Store interface) ---

func (m *mockStore) CreateTeam(_ context.Context, _ agent.CreateTeamRequest) (*agent.Team, error) {
	return nil, nil
}
func (m *mockStore) GetTeam(_ context.Context, _ string) (*agent.Team, error) {
	return nil, errNotFound
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
	return nil, errNotFound
}
func (m *mockStore) GetRoadmapByProject(_ context.Context, _ string) (*roadmap.Roadmap, error) {
	return nil, errNotFound
}
func (m *mockStore) UpdateRoadmap(_ context.Context, _ *roadmap.Roadmap) error { return nil }
func (m *mockStore) DeleteRoadmap(_ context.Context, _ string) error           { return nil }
func (m *mockStore) CreateMilestone(_ context.Context, _ roadmap.CreateMilestoneRequest) (*roadmap.Milestone, error) {
	return &roadmap.Milestone{}, nil
}
func (m *mockStore) GetMilestone(_ context.Context, _ string) (*roadmap.Milestone, error) {
	return nil, errNotFound
}
func (m *mockStore) ListMilestones(_ context.Context, _ string) ([]roadmap.Milestone, error) {
	return nil, nil
}
func (m *mockStore) UpdateMilestone(_ context.Context, _ *roadmap.Milestone) error { return nil }
func (m *mockStore) DeleteMilestone(_ context.Context, _ string) error             { return nil }
func (m *mockStore) CreateFeature(_ context.Context, _ *roadmap.CreateFeatureRequest) (*roadmap.Feature, error) {
	return &roadmap.Feature{}, nil
}
func (m *mockStore) GetFeature(_ context.Context, _ string) (*roadmap.Feature, error) {
	return nil, errNotFound
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
func (m *mockStore) ListSessions(_ context.Context, _ string) ([]run.Session, error) {
	return nil, nil
}
func (m *mockStore) UpdateSessionStatus(_ context.Context, _ string, _ run.SessionStatus, _ string) error {
	return nil
}

// User stubs
func (m *mockStore) CreateUser(_ context.Context, _ *user.User) error        { return nil }
func (m *mockStore) GetUser(_ context.Context, _ string) (*user.User, error) { return nil, errNotFound }
func (m *mockStore) GetUserByEmail(_ context.Context, _, _ string) (*user.User, error) {
	return nil, errNotFound
}
func (m *mockStore) ListUsers(_ context.Context, _ string) ([]user.User, error) { return nil, nil }
func (m *mockStore) UpdateUser(_ context.Context, _ *user.User) error           { return nil }
func (m *mockStore) DeleteUser(_ context.Context, _ string) error               { return nil }

// RefreshToken stubs
func (m *mockStore) CreateRefreshToken(_ context.Context, _ *user.RefreshToken) error { return nil }
func (m *mockStore) GetRefreshTokenByHash(_ context.Context, _ string) (*user.RefreshToken, error) {
	return nil, errNotFound
}
func (m *mockStore) DeleteRefreshToken(_ context.Context, _ string) error        { return nil }
func (m *mockStore) DeleteRefreshTokensByUser(_ context.Context, _ string) error { return nil }

// APIKey stubs
func (m *mockStore) CreateAPIKey(_ context.Context, _ *user.APIKey) error { return nil }
func (m *mockStore) GetAPIKeyByHash(_ context.Context, _ string) (*user.APIKey, error) {
	return nil, errNotFound
}
func (m *mockStore) ListAPIKeysByUser(_ context.Context, _ string) ([]user.APIKey, error) {
	return nil, nil
}
func (m *mockStore) DeleteAPIKey(_ context.Context, _, _ string) error { return nil }

func (m *mockStore) RevokeToken(_ context.Context, _ string, _ time.Time) error { return nil }
func (m *mockStore) IsTokenRevoked(_ context.Context, _ string) (bool, error)   { return false, nil }
func (m *mockStore) PurgeExpiredTokens(_ context.Context) (int64, error)        { return 0, nil }
func (m *mockStore) RotateRefreshToken(_ context.Context, _ string, _ *user.RefreshToken) error {
	return nil
}

// Retrieval Scope stubs
func (m *mockStore) CreateScope(_ context.Context, _ cfcontext.CreateScopeRequest) (*cfcontext.RetrievalScope, error) {
	return nil, nil
}
func (m *mockStore) GetScope(_ context.Context, _ string) (*cfcontext.RetrievalScope, error) {
	return nil, errNotFound
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
func (m *mockStore) AddKnowledgeBaseToScope(_ context.Context, _, _ string) error { return nil }
func (m *mockStore) RemoveKnowledgeBaseFromScope(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockStore) ListKnowledgeBasesByScope(_ context.Context, _ string) ([]knowledgebase.KnowledgeBase, error) {
	return nil, nil
}

// Settings stubs
func (m *mockStore) ListSettings(_ context.Context) ([]settings.Setting, error) {
	return nil, nil
}
func (m *mockStore) GetSetting(_ context.Context, _ string) (*settings.Setting, error) {
	return nil, nil
}
func (m *mockStore) UpsertSetting(_ context.Context, _ string, _ json.RawMessage) error {
	return nil
}

// VCS Account stubs
func (m *mockStore) ListVCSAccounts(_ context.Context) ([]vcsaccount.VCSAccount, error) {
	return nil, nil
}
func (m *mockStore) GetVCSAccount(_ context.Context, _ string) (*vcsaccount.VCSAccount, error) {
	return nil, nil
}
func (m *mockStore) CreateVCSAccount(_ context.Context, _ *vcsaccount.VCSAccount) (*vcsaccount.VCSAccount, error) {
	return nil, nil
}
func (m *mockStore) DeleteVCSAccount(_ context.Context, _ string) error {
	return nil
}

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

// mockQueue implements messagequeue.Queue for testing.
type mockQueue struct{}

func (m *mockQueue) Publish(_ context.Context, _ string, _ []byte) error {
	return nil
}

func (m *mockQueue) Subscribe(_ context.Context, _ string, _ messagequeue.Handler) (func(), error) {
	return func() {}, nil
}

func (m *mockQueue) Drain() error      { return nil }
func (m *mockQueue) Close() error      { return nil }
func (m *mockQueue) IsConnected() bool { return true }

// mockBroadcaster implements broadcast.Broadcaster for testing.
type mockBroadcaster struct{}

func (m *mockBroadcaster) BroadcastEvent(_ context.Context, _ string, _ any) {}

// mockEventStore implements eventstore.Store for testing.
type mockEventStore struct{}

func (m *mockEventStore) Append(_ context.Context, _ *event.AgentEvent) error { return nil }
func (m *mockEventStore) LoadByTask(_ context.Context, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}
func (m *mockEventStore) LoadByAgent(_ context.Context, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}
func (m *mockEventStore) LoadByRun(_ context.Context, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}
func (m *mockEventStore) LoadTrajectory(_ context.Context, _ string, _ eventstore.TrajectoryFilter, _ string, _ int) (*eventstore.TrajectoryPage, error) {
	return &eventstore.TrajectoryPage{}, nil
}
func (m *mockEventStore) TrajectoryStats(_ context.Context, _ string) (*eventstore.TrajectorySummary, error) {
	return &eventstore.TrajectorySummary{}, nil
}
func (m *mockEventStore) LoadEventsRange(_ context.Context, _, _, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}
func (m *mockEventStore) ListCheckpoints(_ context.Context, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}
func (m *mockEventStore) AppendAudit(_ context.Context, _ *event.AuditEntry) error { return nil }
func (m *mockEventStore) LoadAudit(_ context.Context, _ *event.AuditFilter, _ string, _ int) (*event.AuditPage, error) {
	return nil, nil
}

var errNotFound = fmt.Errorf("mock: %w", domain.ErrNotFound)

func newTestRouter() chi.Router {
	store := &mockStore{}
	queue := &mockQueue{}
	bc := &mockBroadcaster{}
	es := &mockEventStore{}
	policySvc := service.NewPolicyService("headless-safe-sandbox", nil)
	runtimeSvc := service.NewRuntimeService(store, queue, bc, es, policySvc, &config.Runtime{})
	orchCfg := &config.Orchestrator{
		MaxParallel:       4,
		PingPongMaxRounds: 3,
		MaxTeamSize:       5,
	}
	orchSvc := service.NewOrchestratorService(store, bc, es, runtimeSvc, orchCfg)
	poolManagerSvc := service.NewPoolManagerService(store, bc, orchCfg)
	metaAgentSvc := service.NewMetaAgentService(store, litellm.NewClient("http://localhost:4000", ""), orchSvc, orchCfg)
	taskPlannerSvc := service.NewTaskPlannerService(metaAgentSvc, poolManagerSvc, store, orchCfg)
	contextOptSvc := service.NewContextOptimizerService(store, orchCfg)
	sharedCtxSvc := service.NewSharedContextService(store, bc, queue)
	modeSvc := service.NewModeService()
	repoMapSvc := service.NewRepoMapService(store, queue, bc, orchCfg)
	retrievalSvc := service.NewRetrievalService(store, queue, bc, orchCfg)
	costSvc := service.NewCostService(store)
	settingsSvc := service.NewSettingsService(store)
	vcsAccountSvc := service.NewVCSAccountService(store, []byte("test-encryption-key-32bytes!!!!!"))
	conversationSvc := service.NewConversationService(store, litellm.NewClient("http://localhost:4000", "test-key"), bc, "", nil)
	handlers := &cfhttp.Handlers{
		Projects:         service.NewProjectService(store, os.TempDir()),
		Tasks:            service.NewTaskService(store, queue),
		Agents:           service.NewAgentService(store, queue, bc),
		LiteLLM:          litellm.NewClient("http://localhost:4000", ""),
		Policies:         policySvc,
		Runtime:          runtimeSvc,
		Orchestrator:     orchSvc,
		MetaAgent:        metaAgentSvc,
		PoolManager:      poolManagerSvc,
		TaskPlanner:      taskPlannerSvc,
		ContextOptimizer: contextOptSvc,
		SharedContext:    sharedCtxSvc,
		Modes:            modeSvc,
		RepoMap:          repoMapSvc,
		Retrieval:        retrievalSvc,
		Events:           es,
		Cost:             costSvc,
		Settings:         settingsSvc,
		VCSAccounts:      vcsAccountSvc,
		Conversations:    conversationSvc,
	}

	r := chi.NewRouter()
	cfhttp.MountRoutes(r, handlers, config.Webhook{})
	return r
}

func TestListProjectsEmpty(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("GET", "/api/v1/projects", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var projects []project.Project
	if err := json.NewDecoder(w.Body).Decode(&projects); err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Fatalf("expected empty list, got %d", len(projects))
	}
}

func TestCreateAndGetProject(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(project.CreateRequest{Name: "My Project", Provider: "local"})
	req := httptest.NewRequest("POST", "/api/v1/projects", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var p project.Project
	if err := json.NewDecoder(w.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	if p.Name != "My Project" {
		t.Fatalf("expected 'My Project', got %q", p.Name)
	}

	// GET by ID
	req = httptest.NewRequest("GET", "/api/v1/projects/"+p.ID, http.NoBody)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestCreateProjectMissingName(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(project.CreateRequest{Provider: "local"})
	req := httptest.NewRequest("POST", "/api/v1/projects", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetProjectNotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/projects/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestVersionEndpoint(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["version"] != "0.1.0" {
		t.Fatalf("expected version 0.1.0, got %q", result["version"])
	}
}

// --- Delete Project ---

func TestDeleteProject(t *testing.T) {
	r := newTestRouter()

	// Create a project first
	body, _ := json.Marshal(project.CreateRequest{Name: "To Delete", Provider: "local"})
	req := httptest.NewRequest("POST", "/api/v1/projects", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var p project.Project
	_ = json.NewDecoder(w.Body).Decode(&p)

	// Delete it
	req = httptest.NewRequest("DELETE", "/api/v1/projects/"+p.ID, http.NoBody)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}

	// Verify it's gone
	req = httptest.NewRequest("GET", "/api/v1/projects/"+p.ID, http.NoBody)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", w.Code)
	}
}

func TestDeleteProjectNotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("DELETE", "/api/v1/projects/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// --- Task Endpoints ---

func TestCreateAndListTasks(t *testing.T) {
	r := newTestRouter()

	// Create project
	projBody, _ := json.Marshal(project.CreateRequest{Name: "Task Project", Provider: "local"})
	req := httptest.NewRequest("POST", "/api/v1/projects", bytes.NewReader(projBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var p project.Project
	_ = json.NewDecoder(w.Body).Decode(&p)

	// Create task
	taskBody, _ := json.Marshal(map[string]string{"title": "Fix bug", "prompt": "Find the null pointer"})
	req = httptest.NewRequest("POST", "/api/v1/projects/"+p.ID+"/tasks", bytes.NewReader(taskBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create task: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var createdTask task.Task
	_ = json.NewDecoder(w.Body).Decode(&createdTask)

	if createdTask.Title != "Fix bug" {
		t.Fatalf("expected title 'Fix bug', got %q", createdTask.Title)
	}

	// List tasks
	req = httptest.NewRequest("GET", "/api/v1/projects/"+p.ID+"/tasks", http.NoBody)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list tasks: expected 200, got %d", w.Code)
	}

	var tasks []task.Task
	_ = json.NewDecoder(w.Body).Decode(&tasks)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
}

func TestCreateTaskMissingTitle(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(map[string]string{"prompt": "no title"})
	req := httptest.NewRequest("POST", "/api/v1/projects/some-id/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateTaskInvalidBody(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("POST", "/api/v1/projects/some-id/tasks", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetTask(t *testing.T) {
	r := newTestRouter()

	// Create project + task
	projBody, _ := json.Marshal(project.CreateRequest{Name: "P", Provider: "local"})
	req := httptest.NewRequest("POST", "/api/v1/projects", bytes.NewReader(projBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var p project.Project
	_ = json.NewDecoder(w.Body).Decode(&p)

	taskBody, _ := json.Marshal(map[string]string{"title": "T1"})
	req = httptest.NewRequest("POST", "/api/v1/projects/"+p.ID+"/tasks", bytes.NewReader(taskBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var createdTask task.Task
	_ = json.NewDecoder(w.Body).Decode(&createdTask)

	// Get task by ID
	req = httptest.NewRequest("GET", "/api/v1/tasks/"+createdTask.ID, http.NoBody)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGetTaskNotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/tasks/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// --- Agent Endpoints ---

func TestListAgentsEmpty(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/projects/some-id/agents", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var agents []agent.Agent
	_ = json.NewDecoder(w.Body).Decode(&agents)
	if len(agents) != 0 {
		t.Fatalf("expected 0 agents, got %d", len(agents))
	}
}

func TestCreateAgentMissingName(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(map[string]string{"backend": "aider"})
	req := httptest.NewRequest("POST", "/api/v1/projects/p1/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateAgentMissingBackend(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(map[string]string{"name": "my-agent"})
	req := httptest.NewRequest("POST", "/api/v1/projects/p1/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateAgentInvalidBody(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("POST", "/api/v1/projects/p1/agents", bytes.NewReader([]byte("{")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetAgentNotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/agents/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDeleteAgentNotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("DELETE", "/api/v1/agents/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// --- Dispatch/Stop Endpoints ---

func TestDispatchTaskMissingTaskID(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest("POST", "/api/v1/agents/a1/dispatch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDispatchTaskInvalidBody(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("POST", "/api/v1/agents/a1/dispatch", bytes.NewReader([]byte("bad")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestStopAgentTaskMissingTaskID(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest("POST", "/api/v1/agents/a1/stop", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestStopAgentTaskInvalidBody(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("POST", "/api/v1/agents/a1/stop", bytes.NewReader([]byte("{")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- Task Events ---

func TestListTaskEventsEmpty(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/tasks/some-task/events", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// --- Run Events ---

func TestListRunEventsEmpty(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/runs/some-run/events", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var events []event.AgentEvent
	if err := json.NewDecoder(w.Body).Decode(&events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("expected empty list, got %d", len(events))
	}
}

// --- Provider Endpoints ---

func TestListGitProviders(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/providers/git", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string][]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["providers"] == nil {
		t.Fatal("expected 'providers' key in response")
	}
}

func TestListAgentBackends(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/providers/agent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string][]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["backends"] == nil {
		t.Fatal("expected 'backends' key in response")
	}
}

// --- Checkout Endpoint ---

func TestCheckoutBranchMissingBranch(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest("POST", "/api/v1/projects/p1/git/checkout", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCheckoutBranchInvalidBody(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("POST", "/api/v1/projects/p1/git/checkout", bytes.NewReader([]byte("bad")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- Create Project Invalid Body ---

func TestCreateProjectInvalidBody(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("POST", "/api/v1/projects", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- LLM Endpoints (require mock server) ---

func TestLLMHealthEndpoint(t *testing.T) {
	// LiteLLM client points to non-existent server, so health should be "unhealthy"
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/llm/health", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]string
	_ = json.NewDecoder(w.Body).Decode(&result)
	if result["status"] != "unhealthy" {
		t.Fatalf("expected 'unhealthy' (no server), got %q", result["status"])
	}
}

func TestAddLLMModelMissingName(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(map[string]string{"litellm_params": "{}"})
	req := httptest.NewRequest("POST", "/api/v1/llm/models", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAddLLMModelInvalidBody(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("POST", "/api/v1/llm/models", bytes.NewReader([]byte("bad")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDeleteLLMModelMissingID(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest("POST", "/api/v1/llm/models/delete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDeleteLLMModelInvalidBody(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("POST", "/api/v1/llm/models/delete", bytes.NewReader([]byte("{")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- Policy Endpoints ---

func TestListPolicyProfiles(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/policies", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string][]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	profiles := result["profiles"]
	if len(profiles) != 4 {
		t.Fatalf("expected 4 profiles (4 presets), got %d: %v", len(profiles), profiles)
	}
}

func TestGetPolicyProfile(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/policies/plan-readonly", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var p policy.PolicyProfile
	if err := json.NewDecoder(w.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	if p.Name != "plan-readonly" {
		t.Fatalf("expected name 'plan-readonly', got %q", p.Name)
	}
}

func TestGetPolicyProfileNotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/policies/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestEvaluatePolicy(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(policy.ToolCall{Tool: "Read", Path: "src/main.go"})
	req := httptest.NewRequest("POST", "/api/v1/policies/plan-readonly/evaluate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result policy.EvaluationResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Decision != policy.DecisionAllow {
		t.Fatalf("expected 'allow' for Read in plan-readonly, got %q", result.Decision)
	}
	if result.Profile != "plan-readonly" {
		t.Fatalf("expected profile 'plan-readonly', got %q", result.Profile)
	}
	if result.Reason == "" {
		t.Fatal("expected non-empty reason")
	}
}

func TestEvaluatePolicyUnknownProfile(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(policy.ToolCall{Tool: "Read"})
	req := httptest.NewRequest("POST", "/api/v1/policies/nonexistent/evaluate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestEvaluatePolicyMissingTool(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(map[string]string{"path": "file.go"})
	req := httptest.NewRequest("POST", "/api/v1/policies/plan-readonly/evaluate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestEvaluatePolicyInvalidBody(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("POST", "/api/v1/policies/plan-readonly/evaluate", bytes.NewReader([]byte("bad")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- Run Endpoints ---

func TestStartRunValidation(t *testing.T) {
	r := newTestRouter()

	// Missing task_id
	body, _ := json.Marshal(map[string]string{"agent_id": "a1", "project_id": "p1"})
	req := httptest.NewRequest("POST", "/api/v1/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing task_id, got %d: %s", w.Code, w.Body.String())
	}

	// Missing agent_id
	body, _ = json.Marshal(map[string]string{"task_id": "t1", "project_id": "p1"})
	req = httptest.NewRequest("POST", "/api/v1/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing agent_id, got %d: %s", w.Code, w.Body.String())
	}
}

func TestStartRunInvalidBody(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("POST", "/api/v1/runs", bytes.NewReader([]byte("bad")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetRunNotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/runs/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestListTaskRunsEmpty(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/tasks/some-task/runs", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var runs []run.Run
	_ = json.NewDecoder(w.Body).Decode(&runs)
	if len(runs) != 0 {
		t.Fatalf("expected empty list, got %d", len(runs))
	}
}

func TestCancelRunNotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("POST", "/api/v1/runs/nonexistent/cancel", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// CancelRun calls GetRun which returns not found → 500 (wrapped domain error)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for cancel of nonexistent run, got %d", w.Code)
	}
}

// --- RepoMap Endpoints ---

func TestGetRepoMapNotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/projects/some-id/repomap", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGenerateRepoMap(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{ID: "proj-1", Name: "Test", WorkspacePath: "/tmp/test"}},
	}
	queue := &mockQueue{}
	bc := &mockBroadcaster{}
	es := &mockEventStore{}
	policySvc := service.NewPolicyService("headless-safe-sandbox", nil)
	runtimeSvc := service.NewRuntimeService(store, queue, bc, es, policySvc, &config.Runtime{})
	orchCfg := &config.Orchestrator{
		MaxParallel:        4,
		PingPongMaxRounds:  3,
		MaxTeamSize:        5,
		RepoMapTokenBudget: 1024,
	}
	orchSvc := service.NewOrchestratorService(store, bc, es, runtimeSvc, orchCfg)
	poolManagerSvc := service.NewPoolManagerService(store, bc, orchCfg)
	metaAgentSvc := service.NewMetaAgentService(store, litellm.NewClient("http://localhost:4000", ""), orchSvc, orchCfg)
	taskPlannerSvc := service.NewTaskPlannerService(metaAgentSvc, poolManagerSvc, store, orchCfg)
	contextOptSvc := service.NewContextOptimizerService(store, orchCfg)
	sharedCtxSvc := service.NewSharedContextService(store, bc, queue)
	modeSvc := service.NewModeService()
	repoMapSvc := service.NewRepoMapService(store, queue, bc, orchCfg)
	retrievalSvc := service.NewRetrievalService(store, queue, bc, orchCfg)
	handlers := &cfhttp.Handlers{
		Projects:         service.NewProjectService(store, os.TempDir()),
		Tasks:            service.NewTaskService(store, queue),
		Agents:           service.NewAgentService(store, queue, bc),
		LiteLLM:          litellm.NewClient("http://localhost:4000", ""),
		Policies:         policySvc,
		Runtime:          runtimeSvc,
		Orchestrator:     orchSvc,
		MetaAgent:        metaAgentSvc,
		PoolManager:      poolManagerSvc,
		TaskPlanner:      taskPlannerSvc,
		ContextOptimizer: contextOptSvc,
		SharedContext:    sharedCtxSvc,
		Modes:            modeSvc,
		RepoMap:          repoMapSvc,
		Retrieval:        retrievalSvc,
		Cost:             service.NewCostService(store),
	}

	r := chi.NewRouter()
	cfhttp.MountRoutes(r, handlers, config.Webhook{})

	req := httptest.NewRequest("POST", "/api/v1/projects/proj-1/repomap", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Retrieval Endpoints ---

func TestIndexProject(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{ID: "proj-1", Name: "Test", WorkspacePath: "/tmp/test"}},
	}
	queue := &mockQueue{}
	bc := &mockBroadcaster{}
	es := &mockEventStore{}
	policySvc := service.NewPolicyService("headless-safe-sandbox", nil)
	runtimeSvc := service.NewRuntimeService(store, queue, bc, es, policySvc, &config.Runtime{})
	orchCfg := &config.Orchestrator{
		MaxParallel:           4,
		PingPongMaxRounds:     3,
		MaxTeamSize:           5,
		DefaultEmbeddingModel: "text-embedding-3-small",
	}
	orchSvc := service.NewOrchestratorService(store, bc, es, runtimeSvc, orchCfg)
	poolManagerSvc := service.NewPoolManagerService(store, bc, orchCfg)
	metaAgentSvc := service.NewMetaAgentService(store, litellm.NewClient("http://localhost:4000", ""), orchSvc, orchCfg)
	taskPlannerSvc := service.NewTaskPlannerService(metaAgentSvc, poolManagerSvc, store, orchCfg)
	contextOptSvc := service.NewContextOptimizerService(store, orchCfg)
	sharedCtxSvc := service.NewSharedContextService(store, bc, queue)
	modeSvc := service.NewModeService()
	repoMapSvc := service.NewRepoMapService(store, queue, bc, orchCfg)
	retrievalSvc := service.NewRetrievalService(store, queue, bc, orchCfg)
	handlers := &cfhttp.Handlers{
		Projects:         service.NewProjectService(store, os.TempDir()),
		Tasks:            service.NewTaskService(store, queue),
		Agents:           service.NewAgentService(store, queue, bc),
		LiteLLM:          litellm.NewClient("http://localhost:4000", ""),
		Policies:         policySvc,
		Runtime:          runtimeSvc,
		Orchestrator:     orchSvc,
		MetaAgent:        metaAgentSvc,
		PoolManager:      poolManagerSvc,
		TaskPlanner:      taskPlannerSvc,
		ContextOptimizer: contextOptSvc,
		SharedContext:    sharedCtxSvc,
		Modes:            modeSvc,
		RepoMap:          repoMapSvc,
		Retrieval:        retrievalSvc,
		Cost:             service.NewCostService(store),
	}

	r := chi.NewRouter()
	cfhttp.MountRoutes(r, handlers, config.Webhook{})

	req := httptest.NewRequest("POST", "/api/v1/projects/proj-1/index", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSearchProjectMissingQuery(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest("POST", "/api/v1/projects/proj-1/search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetIndexStatusNotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/projects/nonexistent/index", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Create / Delete Policy Endpoints ---

func TestCreatePolicyProfile(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(policy.PolicyProfile{
		Name: "test-custom",
		Mode: policy.ModeDefault,
		Rules: []policy.PermissionRule{
			{Specifier: policy.ToolSpecifier{Tool: "Read"}, Decision: policy.DecisionAllow},
		},
	})
	req := httptest.NewRequest("POST", "/api/v1/policies", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var p policy.PolicyProfile
	if err := json.NewDecoder(w.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	if p.Name != "test-custom" {
		t.Fatalf("expected name 'test-custom', got %q", p.Name)
	}

	// Verify it appears in list
	req = httptest.NewRequest("GET", "/api/v1/policies", http.NoBody)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var result map[string][]string
	_ = json.NewDecoder(w.Body).Decode(&result)
	if len(result["profiles"]) != 5 {
		t.Fatalf("expected 5 profiles (4 presets + 1 custom), got %d", len(result["profiles"]))
	}
}

func TestCreatePolicyProfileMissingName(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(map[string]string{"mode": "default"})
	req := httptest.NewRequest("POST", "/api/v1/policies", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreatePolicyProfileInvalidBody(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("POST", "/api/v1/policies", bytes.NewReader([]byte("bad")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDeletePolicyProfile(t *testing.T) {
	r := newTestRouter()

	// Create a custom policy first
	body, _ := json.Marshal(policy.PolicyProfile{
		Name: "to-delete",
		Mode: policy.ModeDefault,
	})
	req := httptest.NewRequest("POST", "/api/v1/policies", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", w.Code)
	}

	// Delete it
	req = httptest.NewRequest("DELETE", "/api/v1/policies/to-delete", http.NoBody)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify it's gone
	req = httptest.NewRequest("GET", "/api/v1/policies/to-delete", http.NoBody)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", w.Code)
	}
}

func TestDeletePolicyProfilePresetForbidden(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("DELETE", "/api/v1/policies/plan-readonly", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for preset deletion, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeletePolicyProfileNotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("DELETE", "/api/v1/policies/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Agent Search Endpoint Tests (Phase 6C) ---

func TestAgentSearchMissingQuery(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest("POST", "/api/v1/projects/proj-1/search/agent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAgentSearchQueryTooLong(t *testing.T) {
	r := newTestRouter()

	longQuery := strings.Repeat("x", 2001)
	body, _ := json.Marshal(map[string]string{"query": longQuery})
	req := httptest.NewRequest("POST", "/api/v1/projects/proj-1/search/agent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSearchProjectQueryTooLong(t *testing.T) {
	r := newTestRouter()

	longQuery := strings.Repeat("x", 2001)
	body, _ := json.Marshal(map[string]string{"query": longQuery})
	req := httptest.NewRequest("POST", "/api/v1/projects/proj-1/search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

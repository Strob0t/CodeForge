package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
	"github.com/Strob0t/CodeForge/internal/adapter/osfs"
	"github.com/Strob0t/CodeForge/internal/config"
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
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/experience"
	"github.com/Strob0t/CodeForge/internal/domain/feedback"
	"github.com/Strob0t/CodeForge/internal/domain/goal"
	"github.com/Strob0t/CodeForge/internal/domain/knowledgebase"
	"github.com/Strob0t/CodeForge/internal/domain/llmkey"
	"github.com/Strob0t/CodeForge/internal/domain/mcp"
	"github.com/Strob0t/CodeForge/internal/domain/memory"
	"github.com/Strob0t/CodeForge/internal/domain/microagent"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
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
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// mockStore implements database.Store for testing.
type mockStore struct {
	mu                  sync.Mutex
	projects            []project.Project
	agents              []agent.Agent
	tasks               []task.Task
	runs                []run.Run
	users               []user.User
	refreshTokens       []user.RefreshToken
	apiKeys             []user.APIKey
	revokedTokens       map[string]time.Time
	passwordResetTokens []user.PasswordResetToken
	// Roadmap fields
	roadmaps   []roadmap.Roadmap
	milestones []roadmap.Milestone
	features   []roadmap.Feature
	// Agent feature fields
	microagents []microagent.Microagent
	skills      []skill.Skill
	autoAgents  []autoagent.AutoAgent
	// Conversation fields
	convs    []conversation.Conversation
	messages []conversation.Message
	// Settings fields
	settings []settings.Setting
	// VCS Account fields
	vcsAccounts []vcsaccount.VCSAccount
	// Session fields
	sessions []run.Session
	// MCP fields
	mcpServers      []mcp.ServerDef
	mcpServerTools  []mcp.ServerTool
	mcpProjectLinks []struct{ ProjectID, ServerID string }
	// Knowledge Base fields
	knowledgeBases []knowledgebase.KnowledgeBase
	kbScopeLinks   []struct{ ScopeID, KBID string }
	// Routing fields (Phase 26)
	routingStats    []routing.ModelPerformanceStats
	routingOutcomes []routing.RoutingOutcome
	// Active Work fields (Phase 24)
	activeWork []task.ActiveWorkItem
	// Goal Discovery fields (Phase 28)
	goals []goal.ProjectGoal
	// Benchmark fields (Phase 26+)
	benchmarkRuns    []benchmark.Run
	benchmarkResults []benchmark.Result
}

func (m *mockStore) ListProjects(_ context.Context) ([]project.Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.projects, nil
}

func (m *mockStore) GetProject(_ context.Context, id string) (*project.Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.projects {
		if m.projects[i].ID == id {
			return &m.projects[i], nil
		}
	}
	return nil, errNotFound
}

func (m *mockStore) CreateProject(_ context.Context, req *project.CreateRequest) (*project.Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := project.Project{
		ID:       "test-id",
		Name:     req.Name,
		Provider: req.Provider,
	}
	m.projects = append(m.projects, p)
	return &p, nil
}

func (m *mockStore) UpdateProject(_ context.Context, p *project.Project) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.projects {
		if m.projects[i].ID == p.ID {
			m.projects[i] = *p
			return nil
		}
	}
	return errNotFound
}

func (m *mockStore) DeleteProject(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.projects {
		if m.projects[i].ID == id {
			m.projects = append(m.projects[:i], m.projects[i+1:]...)
			return nil
		}
	}
	return errNotFound
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
	return []cost.ProjectSummary{}, nil
}
func (m *mockStore) CostSummaryByProject(_ context.Context, _ string) (*cost.Summary, error) {
	return &cost.Summary{}, nil
}
func (m *mockStore) CostByModel(_ context.Context, _ string) ([]cost.ModelSummary, error) {
	return []cost.ModelSummary{}, nil
}
func (m *mockStore) CostTimeSeries(_ context.Context, _ string, _ int) ([]cost.DailyCost, error) {
	return []cost.DailyCost{}, nil
}
func (m *mockStore) RecentRunsWithCost(_ context.Context, _ string, _ int) ([]run.Run, error) {
	return []run.Run{}, nil
}
func (m *mockStore) CostByTool(_ context.Context, _ string) ([]cost.ToolSummary, error) {
	return []cost.ToolSummary{}, nil
}
func (m *mockStore) CostByToolForRun(_ context.Context, _ string) ([]cost.ToolSummary, error) {
	return []cost.ToolSummary{}, nil
}

// Dashboard Aggregation stubs
func (m *mockStore) DashboardStats(_ context.Context) (*dashboard.DashboardStats, error) {
	return &dashboard.DashboardStats{}, nil
}
func (m *mockStore) ProjectHealth(_ context.Context, _ string) (*dashboard.ProjectHealth, error) {
	return &dashboard.ProjectHealth{Sparkline: []float64{}}, nil
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

// --- Roadmap methods ---

func (m *mockStore) CreateRoadmap(_ context.Context, req roadmap.CreateRoadmapRequest) (*roadmap.Roadmap, error) {
	r := roadmap.Roadmap{
		ID:          fmt.Sprintf("roadmap-%d", len(m.roadmaps)+1),
		ProjectID:   req.ProjectID,
		Title:       req.Title,
		Description: req.Description,
		Status:      roadmap.StatusDraft,
		Version:     1,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	m.roadmaps = append(m.roadmaps, r)
	return &r, nil
}

func (m *mockStore) GetRoadmap(_ context.Context, id string) (*roadmap.Roadmap, error) {
	for i := range m.roadmaps {
		if m.roadmaps[i].ID == id {
			return &m.roadmaps[i], nil
		}
	}
	return nil, errNotFound
}

func (m *mockStore) GetRoadmapByProject(_ context.Context, projectID string) (*roadmap.Roadmap, error) {
	for i := range m.roadmaps {
		if m.roadmaps[i].ProjectID == projectID {
			return &m.roadmaps[i], nil
		}
	}
	return nil, errNotFound
}

func (m *mockStore) UpdateRoadmap(_ context.Context, r *roadmap.Roadmap) error {
	for i := range m.roadmaps {
		if m.roadmaps[i].ID == r.ID {
			r.UpdatedAt = time.Now().UTC()
			m.roadmaps[i] = *r
			return nil
		}
	}
	return errNotFound
}

func (m *mockStore) DeleteRoadmap(_ context.Context, id string) error {
	for i := range m.roadmaps {
		if m.roadmaps[i].ID == id {
			m.roadmaps = append(m.roadmaps[:i], m.roadmaps[i+1:]...)
			return nil
		}
	}
	return errNotFound
}

func (m *mockStore) CreateMilestone(_ context.Context, req roadmap.CreateMilestoneRequest) (*roadmap.Milestone, error) {
	ms := roadmap.Milestone{
		ID:          fmt.Sprintf("milestone-%d", len(m.milestones)+1),
		RoadmapID:   req.RoadmapID,
		Title:       req.Title,
		Description: req.Description,
		Status:      roadmap.StatusDraft,
		DueDate:     req.DueDate,
		Version:     1,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	m.milestones = append(m.milestones, ms)
	return &ms, nil
}

func (m *mockStore) GetMilestone(_ context.Context, id string) (*roadmap.Milestone, error) {
	for i := range m.milestones {
		if m.milestones[i].ID == id {
			return &m.milestones[i], nil
		}
	}
	return nil, errNotFound
}

func (m *mockStore) ListMilestones(_ context.Context, roadmapID string) ([]roadmap.Milestone, error) {
	var result []roadmap.Milestone
	for i := range m.milestones {
		if m.milestones[i].RoadmapID == roadmapID {
			result = append(result, m.milestones[i])
		}
	}
	return result, nil
}

func (m *mockStore) UpdateMilestone(_ context.Context, ms *roadmap.Milestone) error {
	for i := range m.milestones {
		if m.milestones[i].ID == ms.ID {
			ms.UpdatedAt = time.Now().UTC()
			m.milestones[i] = *ms
			return nil
		}
	}
	return errNotFound
}

func (m *mockStore) DeleteMilestone(_ context.Context, id string) error {
	for i := range m.milestones {
		if m.milestones[i].ID == id {
			m.milestones = append(m.milestones[:i], m.milestones[i+1:]...)
			return nil
		}
	}
	return errNotFound
}

func (m *mockStore) FindMilestoneByTitle(_ context.Context, roadmapID, title string) (*roadmap.Milestone, error) {
	for i := range m.milestones {
		if m.milestones[i].RoadmapID == roadmapID && m.milestones[i].Title == title {
			return &m.milestones[i], nil
		}
	}
	return nil, errNotFound
}

func (m *mockStore) CreateFeature(_ context.Context, req *roadmap.CreateFeatureRequest) (*roadmap.Feature, error) {
	f := roadmap.Feature{
		ID:          fmt.Sprintf("feature-%d", len(m.features)+1),
		MilestoneID: req.MilestoneID,
		Title:       req.Title,
		Description: req.Description,
		Status:      roadmap.FeatureBacklog,
		Labels:      req.Labels,
		SpecRef:     req.SpecRef,
		ExternalIDs: req.ExternalIDs,
		Version:     1,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	m.features = append(m.features, f)
	return &f, nil
}

func (m *mockStore) GetFeature(_ context.Context, id string) (*roadmap.Feature, error) {
	for i := range m.features {
		if m.features[i].ID == id {
			return &m.features[i], nil
		}
	}
	return nil, errNotFound
}

func (m *mockStore) FindFeatureBySpecRef(_ context.Context, milestoneID, specRef string) (*roadmap.Feature, error) {
	for i := range m.features {
		if m.features[i].MilestoneID == milestoneID && m.features[i].SpecRef == specRef {
			return &m.features[i], nil
		}
	}
	return nil, errNotFound
}

func (m *mockStore) ListFeatures(_ context.Context, milestoneID string) ([]roadmap.Feature, error) {
	var result []roadmap.Feature
	for i := range m.features {
		if m.features[i].MilestoneID == milestoneID {
			result = append(result, m.features[i])
		}
	}
	return result, nil
}

func (m *mockStore) ListFeaturesByRoadmap(_ context.Context, roadmapID string) ([]roadmap.Feature, error) {
	var result []roadmap.Feature
	for i := range m.features {
		if m.features[i].RoadmapID == roadmapID {
			result = append(result, m.features[i])
		}
	}
	return result, nil
}

func (m *mockStore) UpdateFeature(_ context.Context, f *roadmap.Feature) error {
	for i := range m.features {
		if m.features[i].ID == f.ID {
			f.UpdatedAt = time.Now().UTC()
			m.features[i] = *f
			return nil
		}
	}
	return errNotFound
}

func (m *mockStore) DeleteFeature(_ context.Context, id string) error {
	for i := range m.features {
		if m.features[i].ID == id {
			m.features = append(m.features[:i], m.features[i+1:]...)
			return nil
		}
	}
	return errNotFound
}

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

// Session methods
func (m *mockStore) CreateSession(_ context.Context, s *run.Session) error {
	if s.ID == "" {
		s.ID = fmt.Sprintf("sess-%d", len(m.sessions)+1)
	}
	s.CreatedAt = time.Now().UTC()
	s.UpdatedAt = s.CreatedAt
	m.sessions = append(m.sessions, *s)
	return nil
}
func (m *mockStore) GetSession(_ context.Context, id string) (*run.Session, error) {
	for i := range m.sessions {
		if m.sessions[i].ID == id {
			return &m.sessions[i], nil
		}
	}
	return nil, errNotFound
}
func (m *mockStore) GetSessionByConversation(_ context.Context, _ string) (*run.Session, error) {
	return nil, nil
}
func (m *mockStore) ListSessions(_ context.Context, projectID string) ([]run.Session, error) {
	var result []run.Session
	for i := range m.sessions {
		if m.sessions[i].ProjectID == projectID {
			result = append(result, m.sessions[i])
		}
	}
	return result, nil
}
func (m *mockStore) UpdateSessionStatus(_ context.Context, id string, status run.SessionStatus, metadata string) error {
	for i := range m.sessions {
		if m.sessions[i].ID == id {
			m.sessions[i].Status = status
			m.sessions[i].Metadata = metadata
			m.sessions[i].UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return errNotFound
}

// --- User methods ---

func (m *mockStore) CreateUser(_ context.Context, u *user.User) error {
	if u.ID == "" {
		u.ID = fmt.Sprintf("user-%d", len(m.users)+1)
	}
	now := time.Now().UTC()
	u.CreatedAt = now
	u.UpdatedAt = now
	m.users = append(m.users, *u)
	return nil
}

func (m *mockStore) CreateFirstUser(_ context.Context, u *user.User) error {
	if len(m.users) > 0 {
		return domain.ErrConflict
	}
	return m.CreateUser(context.Background(), u)
}

func (m *mockStore) GetUser(_ context.Context, id string) (*user.User, error) {
	for i := range m.users {
		if m.users[i].ID == id {
			return &m.users[i], nil
		}
	}
	return nil, errNotFound
}

func (m *mockStore) GetUserByEmail(_ context.Context, email, tenantID string) (*user.User, error) {
	for i := range m.users {
		if m.users[i].Email == email && (tenantID == "" || m.users[i].TenantID == tenantID) {
			return &m.users[i], nil
		}
	}
	return nil, errNotFound
}

func (m *mockStore) ListUsers(_ context.Context, tenantID string) ([]user.User, error) {
	if tenantID == "" {
		return m.users, nil
	}
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
			u.UpdatedAt = time.Now().UTC()
			m.users[i] = *u
			return nil
		}
	}
	return errNotFound
}

func (m *mockStore) DeleteUser(_ context.Context, id string) error {
	for i := range m.users {
		if m.users[i].ID == id {
			m.users = append(m.users[:i], m.users[i+1:]...)
			return nil
		}
	}
	return errNotFound
}

// --- RefreshToken methods ---

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
	return nil, errNotFound
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

// --- Token revocation ---

func (m *mockStore) RevokeToken(_ context.Context, jti string, expiresAt time.Time) error {
	if m.revokedTokens == nil {
		m.revokedTokens = make(map[string]time.Time)
	}
	m.revokedTokens[jti] = expiresAt
	return nil
}

func (m *mockStore) IsTokenRevoked(_ context.Context, jti string) (bool, error) {
	if m.revokedTokens == nil {
		return false, nil
	}
	_, ok := m.revokedTokens[jti]
	return ok, nil
}

func (m *mockStore) PurgeExpiredTokens(_ context.Context) (int64, error) { return 0, nil }

// --- APIKey methods ---

func (m *mockStore) CreateAPIKey(_ context.Context, key *user.APIKey) error {
	key.CreatedAt = time.Now().UTC()
	m.apiKeys = append(m.apiKeys, *key)
	return nil
}

func (m *mockStore) GetAPIKeyByHash(_ context.Context, hash string) (*user.APIKey, error) {
	for i := range m.apiKeys {
		if m.apiKeys[i].KeyHash == hash {
			return &m.apiKeys[i], nil
		}
	}
	return nil, errNotFound
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

func (m *mockStore) DeleteAPIKey(_ context.Context, id, userID string) error {
	for i := range m.apiKeys {
		if m.apiKeys[i].ID == id && m.apiKeys[i].UserID == userID {
			m.apiKeys = append(m.apiKeys[:i], m.apiKeys[i+1:]...)
			return nil
		}
	}
	return errNotFound
}

// --- Password Reset Token methods ---

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
	return nil
}

func (m *mockStore) DeleteExpiredPasswordResetTokens(_ context.Context) (int64, error) {
	return 0, nil
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
func (m *mockStore) GetScopesForProject(_ context.Context, _ string) ([]cfcontext.RetrievalScope, error) {
	return nil, nil
}
func (m *mockStore) AddProjectToScope(_ context.Context, _, _ string) error      { return nil }
func (m *mockStore) RemoveProjectFromScope(_ context.Context, _, _ string) error { return nil }

// Knowledge Base methods
func (m *mockStore) CreateKnowledgeBase(_ context.Context, req *knowledgebase.CreateRequest) (*knowledgebase.KnowledgeBase, error) {
	kb := knowledgebase.KnowledgeBase{
		ID:          fmt.Sprintf("kb-%d", len(m.knowledgeBases)+1),
		Name:        req.Name,
		Description: req.Description,
		Category:    req.Category,
		Tags:        req.Tags,
		ContentPath: req.ContentPath,
		Status:      knowledgebase.StatusPending,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	m.knowledgeBases = append(m.knowledgeBases, kb)
	return &kb, nil
}
func (m *mockStore) GetKnowledgeBase(_ context.Context, id string) (*knowledgebase.KnowledgeBase, error) {
	for i := range m.knowledgeBases {
		if m.knowledgeBases[i].ID == id {
			return &m.knowledgeBases[i], nil
		}
	}
	return nil, errNotFound
}
func (m *mockStore) ListKnowledgeBases(_ context.Context) ([]knowledgebase.KnowledgeBase, error) {
	return m.knowledgeBases, nil
}
func (m *mockStore) UpdateKnowledgeBase(_ context.Context, id string, req knowledgebase.UpdateRequest) (*knowledgebase.KnowledgeBase, error) {
	for i := range m.knowledgeBases {
		if m.knowledgeBases[i].ID != id {
			continue
		}
		if req.Name != nil {
			m.knowledgeBases[i].Name = *req.Name
		}
		if req.Description != nil {
			m.knowledgeBases[i].Description = *req.Description
		}
		if req.Tags != nil {
			m.knowledgeBases[i].Tags = req.Tags
		}
		m.knowledgeBases[i].UpdatedAt = time.Now().UTC()
		return &m.knowledgeBases[i], nil
	}
	return nil, errNotFound
}
func (m *mockStore) DeleteKnowledgeBase(_ context.Context, id string) error {
	for i := range m.knowledgeBases {
		if m.knowledgeBases[i].ID == id {
			m.knowledgeBases = append(m.knowledgeBases[:i], m.knowledgeBases[i+1:]...)
			return nil
		}
	}
	return errNotFound
}
func (m *mockStore) UpdateKnowledgeBaseStatus(_ context.Context, id, status string, chunkCount int) error {
	for i := range m.knowledgeBases {
		if m.knowledgeBases[i].ID == id {
			m.knowledgeBases[i].Status = knowledgebase.Status(status)
			m.knowledgeBases[i].ChunkCount = chunkCount
			return nil
		}
	}
	return errNotFound
}
func (m *mockStore) AddKnowledgeBaseToScope(_ context.Context, scopeID, kbID string) error {
	m.kbScopeLinks = append(m.kbScopeLinks, struct{ ScopeID, KBID string }{scopeID, kbID})
	return nil
}
func (m *mockStore) RemoveKnowledgeBaseFromScope(_ context.Context, scopeID, kbID string) error {
	for i := range m.kbScopeLinks {
		if m.kbScopeLinks[i].ScopeID == scopeID && m.kbScopeLinks[i].KBID == kbID {
			m.kbScopeLinks = append(m.kbScopeLinks[:i], m.kbScopeLinks[i+1:]...)
			return nil
		}
	}
	return nil
}
func (m *mockStore) ListKnowledgeBasesByScope(_ context.Context, scopeID string) ([]knowledgebase.KnowledgeBase, error) {
	var result []knowledgebase.KnowledgeBase
	for _, link := range m.kbScopeLinks {
		if link.ScopeID == scopeID {
			for i := range m.knowledgeBases {
				if m.knowledgeBases[i].ID == link.KBID {
					result = append(result, m.knowledgeBases[i])
				}
			}
		}
	}
	return result, nil
}

// Settings methods
func (m *mockStore) ListSettings(_ context.Context) ([]settings.Setting, error) {
	return m.settings, nil
}
func (m *mockStore) GetSetting(_ context.Context, key string) (*settings.Setting, error) {
	for i := range m.settings {
		if m.settings[i].Key == key {
			return &m.settings[i], nil
		}
	}
	return nil, errNotFound
}
func (m *mockStore) UpsertSetting(_ context.Context, key string, value json.RawMessage) error {
	for i := range m.settings {
		if m.settings[i].Key == key {
			m.settings[i].Value = value
			m.settings[i].UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	m.settings = append(m.settings, settings.Setting{Key: key, Value: value, UpdatedAt: time.Now().UTC()})
	return nil
}

// VCS Account methods
func (m *mockStore) ListVCSAccounts(_ context.Context) ([]vcsaccount.VCSAccount, error) {
	return m.vcsAccounts, nil
}
func (m *mockStore) GetVCSAccount(_ context.Context, id string) (*vcsaccount.VCSAccount, error) {
	for i := range m.vcsAccounts {
		if m.vcsAccounts[i].ID == id {
			return &m.vcsAccounts[i], nil
		}
	}
	return nil, errNotFound
}
func (m *mockStore) CreateVCSAccount(_ context.Context, a *vcsaccount.VCSAccount) (*vcsaccount.VCSAccount, error) {
	if a.ID == "" {
		a.ID = fmt.Sprintf("vcs-%d", len(m.vcsAccounts)+1)
	}
	m.vcsAccounts = append(m.vcsAccounts, *a)
	return a, nil
}
func (m *mockStore) DeleteVCSAccount(_ context.Context, id string) error {
	for i := range m.vcsAccounts {
		if m.vcsAccounts[i].ID == id {
			m.vcsAccounts = append(m.vcsAccounts[:i], m.vcsAccounts[i+1:]...)
			return nil
		}
	}
	return errNotFound
}

// OAuth State stubs
func (m *mockStore) CreateOAuthState(_ context.Context, _ *vcsaccount.OAuthState) error {
	return nil
}
func (m *mockStore) GetOAuthState(_ context.Context, _ string) (*vcsaccount.OAuthState, error) {
	return nil, errNotFound
}
func (m *mockStore) DeleteOAuthState(_ context.Context, _ string) error        { return nil }
func (m *mockStore) DeleteExpiredOAuthStates(_ context.Context) (int64, error) { return 0, nil }

// Conversation methods
func (m *mockStore) CreateConversation(_ context.Context, c *conversation.Conversation) (*conversation.Conversation, error) {
	if c.ID == "" {
		c.ID = fmt.Sprintf("conv-%d", len(m.convs)+1)
	}
	m.convs = append(m.convs, *c)
	return c, nil
}
func (m *mockStore) GetConversation(_ context.Context, id string) (*conversation.Conversation, error) {
	for i := range m.convs {
		if m.convs[i].ID == id {
			return &m.convs[i], nil
		}
	}
	return nil, errNotFound
}
func (m *mockStore) ListConversationsByProject(_ context.Context, projectID string) ([]conversation.Conversation, error) {
	var result []conversation.Conversation
	for i := range m.convs {
		if m.convs[i].ProjectID == projectID {
			result = append(result, m.convs[i])
		}
	}
	return result, nil
}
func (m *mockStore) DeleteConversation(_ context.Context, id string) error {
	for i := range m.convs {
		if m.convs[i].ID == id {
			m.convs = append(m.convs[:i], m.convs[i+1:]...)
			return nil
		}
	}
	return errNotFound
}
func (m *mockStore) CreateMessage(_ context.Context, msg *conversation.Message) (*conversation.Message, error) {
	if msg.ID == "" {
		msg.ID = fmt.Sprintf("msg-%d", len(m.messages)+1)
	}
	m.messages = append(m.messages, *msg)
	return msg, nil
}
func (m *mockStore) CreateToolMessages(_ context.Context, _ string, _ []conversation.Message) error {
	return nil
}
func (m *mockStore) ListMessages(_ context.Context, conversationID string) ([]conversation.Message, error) {
	var result []conversation.Message
	for i := range m.messages {
		if m.messages[i].ConversationID == conversationID {
			result = append(result, m.messages[i])
		}
	}
	return result, nil
}
func (m *mockStore) DeleteConversationMessages(_ context.Context, conversationID string) error {
	filtered := m.messages[:0]
	for i := range m.messages {
		if m.messages[i].ConversationID != conversationID {
			filtered = append(filtered, m.messages[i])
		}
	}
	m.messages = filtered
	return nil
}
func (m *mockStore) UpdateConversationMode(_ context.Context, _, _ string) error  { return nil }
func (m *mockStore) UpdateConversationModel(_ context.Context, _, _ string) error { return nil }
func (m *mockStore) SearchConversationMessages(_ context.Context, _ string, _ []string, _ int) ([]conversation.Message, error) {
	return nil, nil
}

// MCP Server methods
func (m *mockStore) CreateMCPServer(_ context.Context, s *mcp.ServerDef) error {
	m.mcpServers = append(m.mcpServers, *s)
	return nil
}
func (m *mockStore) GetMCPServer(_ context.Context, id string) (*mcp.ServerDef, error) {
	for i := range m.mcpServers {
		if m.mcpServers[i].ID == id {
			return &m.mcpServers[i], nil
		}
	}
	return nil, errNotFound
}
func (m *mockStore) ListMCPServers(_ context.Context) ([]mcp.ServerDef, error) {
	return m.mcpServers, nil
}
func (m *mockStore) UpdateMCPServer(_ context.Context, s *mcp.ServerDef) error {
	for i := range m.mcpServers {
		if m.mcpServers[i].ID == s.ID {
			m.mcpServers[i] = *s
			return nil
		}
	}
	return errNotFound
}
func (m *mockStore) DeleteMCPServer(_ context.Context, id string) error {
	for i := range m.mcpServers {
		if m.mcpServers[i].ID == id {
			m.mcpServers = append(m.mcpServers[:i], m.mcpServers[i+1:]...)
			return nil
		}
	}
	return errNotFound
}
func (m *mockStore) UpdateMCPServerStatus(_ context.Context, id string, status mcp.ServerStatus) error {
	for i := range m.mcpServers {
		if m.mcpServers[i].ID == id {
			m.mcpServers[i].Status = status
			return nil
		}
	}
	return errNotFound
}
func (m *mockStore) AssignMCPServerToProject(_ context.Context, projectID, serverID string) error {
	m.mcpProjectLinks = append(m.mcpProjectLinks, struct{ ProjectID, ServerID string }{projectID, serverID})
	return nil
}
func (m *mockStore) UnassignMCPServerFromProject(_ context.Context, projectID, serverID string) error {
	for i := range m.mcpProjectLinks {
		if m.mcpProjectLinks[i].ProjectID == projectID && m.mcpProjectLinks[i].ServerID == serverID {
			m.mcpProjectLinks = append(m.mcpProjectLinks[:i], m.mcpProjectLinks[i+1:]...)
			return nil
		}
	}
	return nil
}
func (m *mockStore) ListMCPServersByProject(_ context.Context, projectID string) ([]mcp.ServerDef, error) {
	var result []mcp.ServerDef
	for _, link := range m.mcpProjectLinks {
		if link.ProjectID == projectID {
			for i := range m.mcpServers {
				if m.mcpServers[i].ID == link.ServerID {
					result = append(result, m.mcpServers[i])
				}
			}
		}
	}
	return result, nil
}
func (m *mockStore) UpsertMCPServerTools(_ context.Context, serverID string, tools []mcp.ServerTool) error {
	n := 0
	for i := range m.mcpServerTools {
		if m.mcpServerTools[i].ServerID != serverID {
			m.mcpServerTools[n] = m.mcpServerTools[i]
			n++
		}
	}
	m.mcpServerTools = append(m.mcpServerTools[:n], tools...)
	return nil
}
func (m *mockStore) ListMCPServerTools(_ context.Context, serverID string) ([]mcp.ServerTool, error) {
	var result []mcp.ServerTool
	for i := range m.mcpServerTools {
		if m.mcpServerTools[i].ServerID == serverID {
			result = append(result, m.mcpServerTools[i])
		}
	}
	return result, nil
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
func (m *mockStore) CreateBenchmarkRun(_ context.Context, r *benchmark.Run) error {
	m.benchmarkRuns = append(m.benchmarkRuns, *r)
	return nil
}
func (m *mockStore) GetBenchmarkRun(_ context.Context, id string) (*benchmark.Run, error) {
	for i := range m.benchmarkRuns {
		if m.benchmarkRuns[i].ID == id {
			return &m.benchmarkRuns[i], nil
		}
	}
	return nil, fmt.Errorf("run not found: %s", id)
}
func (m *mockStore) ListBenchmarkRuns(_ context.Context) ([]benchmark.Run, error) {
	return m.benchmarkRuns, nil
}
func (m *mockStore) UpdateBenchmarkRun(_ context.Context, _ *benchmark.Run) error { return nil }
func (m *mockStore) DeleteBenchmarkRun(_ context.Context, _ string) error         { return nil }
func (m *mockStore) CreateBenchmarkResult(_ context.Context, _ *benchmark.Result) error {
	return nil
}
func (m *mockStore) ListBenchmarkResults(_ context.Context, runID string) ([]benchmark.Result, error) {
	var results []benchmark.Result
	for i := range m.benchmarkResults {
		if m.benchmarkResults[i].RunID == runID {
			results = append(results, m.benchmarkResults[i])
		}
	}
	return results, nil
}

// Experience Pool stubs
func (m *mockStore) CreateExperienceEntry(_ context.Context, _ *experience.Entry) error { return nil }
func (m *mockStore) GetExperienceEntry(_ context.Context, _ string) (*experience.Entry, error) {
	return nil, nil
}
func (m *mockStore) ListExperienceEntries(_ context.Context, _ string) ([]experience.Entry, error) {
	return nil, nil
}
func (m *mockStore) DeleteExperienceEntry(_ context.Context, _ string) error { return nil }
func (m *mockStore) UpdateExperienceHit(_ context.Context, _ string) error   { return nil }

// Agent Memory stubs
func (m *mockStore) CreateMemory(_ context.Context, _ *memory.Memory) error { return nil }
func (m *mockStore) ListMemories(_ context.Context, _ string) ([]memory.Memory, error) {
	return nil, nil
}

// --- Microagent methods ---

func (m *mockStore) CreateMicroagent(_ context.Context, ma *microagent.Microagent) error {
	if ma.ID == "" {
		ma.ID = fmt.Sprintf("microagent-%d", len(m.microagents)+1)
	}
	ma.CreatedAt = time.Now().UTC()
	ma.UpdatedAt = ma.CreatedAt
	m.microagents = append(m.microagents, *ma)
	return nil
}

func (m *mockStore) GetMicroagent(_ context.Context, id string) (*microagent.Microagent, error) {
	for i := range m.microagents {
		if m.microagents[i].ID == id {
			return &m.microagents[i], nil
		}
	}
	return nil, errNotFound
}

func (m *mockStore) ListMicroagents(_ context.Context, projectID string) ([]microagent.Microagent, error) {
	var result []microagent.Microagent
	for i := range m.microagents {
		if m.microagents[i].ProjectID == projectID || m.microagents[i].ProjectID == "" {
			result = append(result, m.microagents[i])
		}
	}
	return result, nil
}

func (m *mockStore) UpdateMicroagent(_ context.Context, ma *microagent.Microagent) error {
	for i := range m.microagents {
		if m.microagents[i].ID == ma.ID {
			ma.UpdatedAt = time.Now().UTC()
			m.microagents[i] = *ma
			return nil
		}
	}
	return errNotFound
}

func (m *mockStore) DeleteMicroagent(_ context.Context, id string) error {
	for i := range m.microagents {
		if m.microagents[i].ID == id {
			m.microagents = append(m.microagents[:i], m.microagents[i+1:]...)
			return nil
		}
	}
	return errNotFound
}

// --- Skill methods ---

func (m *mockStore) CreateSkill(_ context.Context, sk *skill.Skill) error {
	if sk.ID == "" {
		sk.ID = fmt.Sprintf("skill-%d", len(m.skills)+1)
	}
	sk.CreatedAt = time.Now().UTC()
	m.skills = append(m.skills, *sk)
	return nil
}

func (m *mockStore) GetSkill(_ context.Context, id string) (*skill.Skill, error) {
	for i := range m.skills {
		if m.skills[i].ID == id {
			return &m.skills[i], nil
		}
	}
	return nil, errNotFound
}

func (m *mockStore) ListSkills(_ context.Context, projectID string) ([]skill.Skill, error) {
	var result []skill.Skill
	for i := range m.skills {
		if m.skills[i].ProjectID == projectID || m.skills[i].ProjectID == "" {
			result = append(result, m.skills[i])
		}
	}
	return result, nil
}

func (m *mockStore) UpdateSkill(_ context.Context, sk *skill.Skill) error {
	for i := range m.skills {
		if m.skills[i].ID == sk.ID {
			m.skills[i] = *sk
			return nil
		}
	}
	return errNotFound
}

func (m *mockStore) DeleteSkill(_ context.Context, id string) error {
	for i := range m.skills {
		if m.skills[i].ID == id {
			m.skills = append(m.skills[:i], m.skills[i+1:]...)
			return nil
		}
	}
	return errNotFound
}

func (m *mockStore) IncrementSkillUsage(_ context.Context, _ string) error { return nil }
func (m *mockStore) ListActiveSkills(_ context.Context, _ string) ([]skill.Skill, error) {
	return nil, nil
}

// Feedback Audit stubs
func (m *mockStore) CreateFeedbackAudit(_ context.Context, _ *feedback.AuditEntry) error {
	return nil
}
func (m *mockStore) ListFeedbackByRun(_ context.Context, _ string) ([]feedback.AuditEntry, error) {
	return nil, nil
}

// --- Auto-Agent methods ---

func (m *mockStore) UpsertAutoAgent(_ context.Context, aa *autoagent.AutoAgent) error {
	for i := range m.autoAgents {
		if m.autoAgents[i].ProjectID == aa.ProjectID {
			m.autoAgents[i] = *aa
			return nil
		}
	}
	m.autoAgents = append(m.autoAgents, *aa)
	return nil
}

func (m *mockStore) GetAutoAgent(_ context.Context, projectID string) (*autoagent.AutoAgent, error) {
	for i := range m.autoAgents {
		if m.autoAgents[i].ProjectID == projectID {
			return &m.autoAgents[i], nil
		}
	}
	return nil, errNotFound
}

func (m *mockStore) UpdateAutoAgentStatus(_ context.Context, projectID string, status autoagent.Status, errMsg string) error {
	for i := range m.autoAgents {
		if m.autoAgents[i].ProjectID == projectID {
			m.autoAgents[i].Status = status
			m.autoAgents[i].Error = errMsg
			return nil
		}
	}
	return nil
}

func (m *mockStore) UpdateAutoAgentProgress(_ context.Context, aa *autoagent.AutoAgent) error {
	for i := range m.autoAgents {
		if m.autoAgents[i].ProjectID == aa.ProjectID {
			m.autoAgents[i] = *aa
			return nil
		}
	}
	return nil
}

func (m *mockStore) DeleteAutoAgent(_ context.Context, _ string) error { return nil }

// Quarantine (Phase 23B)
func (m *mockStore) QuarantineMessage(_ context.Context, _ *quarantine.Message) error { return nil }
func (m *mockStore) GetQuarantinedMessage(_ context.Context, _ string) (*quarantine.Message, error) {
	return nil, errNotFound
}
func (m *mockStore) ListQuarantinedMessages(_ context.Context, _ string, _ quarantine.Status, _, _ int) ([]*quarantine.Message, error) {
	return nil, nil
}
func (m *mockStore) UpdateQuarantineStatus(_ context.Context, _ string, _ quarantine.Status, _, _ string) error {
	return nil
}

// Agent Identity (Phase 23C)
func (m *mockStore) IncrementAgentStats(_ context.Context, _ string, _ float64, _ bool) error {
	return nil
}
func (m *mockStore) UpdateAgentState(_ context.Context, _ string, _ map[string]string) error {
	return nil
}
func (m *mockStore) SendAgentMessage(_ context.Context, _ *agent.InboxMessage) error { return nil }
func (m *mockStore) ListAgentInbox(_ context.Context, _ string, _ bool) ([]agent.InboxMessage, error) {
	return nil, nil
}
func (m *mockStore) MarkInboxRead(_ context.Context, _ string) error { return nil }

// Active Work Visibility (Phase 24)
func (m *mockStore) ListActiveWork(_ context.Context, _ string) ([]task.ActiveWorkItem, error) {
	return m.activeWork, nil
}
func (m *mockStore) ClaimTask(_ context.Context, taskID, agentID string, version int) (*task.ClaimResult, error) {
	for i := range m.tasks {
		if m.tasks[i].ID == taskID && m.tasks[i].Status == task.StatusPending && m.tasks[i].Version == version {
			m.tasks[i].AgentID = agentID
			m.tasks[i].Status = task.StatusQueued
			m.tasks[i].Version++
			return &task.ClaimResult{Task: &m.tasks[i], Claimed: true}, nil
		}
	}
	return &task.ClaimResult{Claimed: false, Reason: "task already claimed or version mismatch"}, nil
}
func (m *mockStore) ReleaseStaleWork(_ context.Context, _ time.Duration) ([]task.Task, error) {
	return nil, nil
}

// mockQueue implements messagequeue.Queue for testing.
type mockQueue struct{}

func (m *mockQueue) Publish(_ context.Context, _ string, _ []byte) error {
	return nil
}

func (m *mockQueue) PublishWithDedup(_ context.Context, _ string, _ []byte, _ string) error {
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
	return newTestRouterWithStore(&mockStore{})
}

func newTestRouterWithStore(store *mockStore) chi.Router {
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
	metaAgentSvc := service.NewMetaAgentService(store, litellm.NewClient("http://localhost:4000", ""), orchSvc, orchCfg, &config.Limits{})
	taskPlannerSvc := service.NewTaskPlannerService(metaAgentSvc, poolManagerSvc, store, orchCfg, &config.Limits{})
	contextOptSvc := service.NewContextOptimizerService(store, osfs.New(), orchCfg, &config.Limits{})
	sharedCtxSvc := service.NewSharedContextService(store, bc, queue)
	modeSvc := service.NewModeService()
	pipelineSvc := service.NewPipelineService(modeSvc)
	repoMapSvc := service.NewRepoMapService(store, queue, bc, orchCfg)
	retrievalSvc := service.NewRetrievalService(store, queue, bc, orchCfg, &config.Limits{})
	costSvc := service.NewCostService(store)
	settingsSvc := service.NewSettingsService(store)
	vcsAccountSvc := service.NewVCSAccountService(store, []byte("test-encryption-key-32bytes!!!!!"))
	conversationSvc := service.NewConversationService(store, bc, "", nil)
	conversationSvc.SetQueue(queue)
	authCfg := &config.Auth{
		Enabled:            true,
		JWTSecret:          "test-secret-key-32bytes-handler!",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		BcryptCost:         4,
	}
	authSvc := service.NewAuthService(store, authCfg)
	filesSvc := service.NewFileService(store, osfs.New())
	roadmapSvc := service.NewRoadmapService(store, bc, nil, nil)
	autoAgentSvc := service.NewAutoAgentService(store, bc, queue, conversationSvc)
	microagentSvc := service.NewMicroagentService(store)
	skillSvc := service.NewSkillService(store)
	memorySvc := service.NewMemoryService(store, queue)
	experiencePoolSvc := service.NewExperiencePoolService(store)
	kbSvc := service.NewKnowledgeBaseService(store)
	sessionSvc := service.NewSessionService(store, es)
	mcpSvc := service.NewMCPService(&config.MCP{}, &config.Limits{MCPTestTimeout: 10 * time.Second})
	mcpSvc.SetStore(store)
	handlers := &cfhttp.Handlers{
		Projects:         service.NewProjectService(store, os.TempDir()),
		Tasks:            service.NewTaskService(store, queue),
		Agents:           service.NewAgentService(store, queue, bc),
		LLM:              litellm.NewClient("http://localhost:4000", ""),
		Policies:         policySvc,
		Runtime:          runtimeSvc,
		Orchestrator:     orchSvc,
		MetaAgent:        metaAgentSvc,
		PoolManager:      poolManagerSvc,
		TaskPlanner:      taskPlannerSvc,
		ContextOptimizer: contextOptSvc,
		SharedContext:    sharedCtxSvc,
		Modes:            modeSvc,
		Pipelines:        pipelineSvc,
		RepoMap:          repoMapSvc,
		Retrieval:        retrievalSvc,
		Events:           es,
		Cost:             costSvc,
		Settings:         settingsSvc,
		VCSAccounts:      vcsAccountSvc,
		Conversations:    conversationSvc,
		Auth:             authSvc,
		Files:            filesSvc,
		Roadmap:          roadmapSvc,
		AutoAgent:        autoAgentSvc,
		Microagents:      microagentSvc,
		Skills:           skillSvc,
		Memory:           memorySvc,
		ExperiencePool:   experiencePoolSvc,
		KnowledgeBases:   kbSvc,
		Sessions:         sessionSvc,
		MCP:              mcpSvc,
		Scope:            service.NewScopeService(store),
		PromptSections:   service.NewPromptSectionService(store),
		Benchmarks: func() *service.BenchmarkService {
			suiteSvc := service.NewBenchmarkSuiteService(store, os.TempDir())
			runMgr := service.NewBenchmarkRunManager(store, suiteSvc)
			resultAgg := service.NewBenchmarkResultAggregator(store)
			watchdog := service.NewBenchmarkWatchdog(store)
			return service.NewBenchmarkService(suiteSvc, runMgr, resultAgg, watchdog)
		}(),
		ActiveWork:    service.NewActiveWorkService(store, bc),
		Routing:       service.NewRoutingService(store),
		GoalDiscovery: service.NewGoalDiscoveryService(store, osfs.New()),
		AppEnv:        os.Getenv("APP_ENV"),
		Limits: &config.Limits{
			MaxRequestBodySize: 1 << 20,
			MaxQueryLength:     2000,
			MaxFiles:           50,
			MaxFileSize:        32768,
			MaxInputLen:        10000,
			MaxEntries:         100,
		},
	}

	r := chi.NewRouter()
	// Inject a default admin user so RBAC-protected routes pass in tests.
	// Skip for /auth/ paths — those tests manage their own user context.
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
				if middleware.UserFromContext(r.Context()) == nil {
					r = r.WithContext(middleware.ContextWithTestUser(r.Context(), &user.User{
						ID:   "test-admin",
						Name: "Test Admin",
						Role: user.RoleAdmin,
					}))
				}
			}
			next.ServeHTTP(w, r)
		})
	})
	cfhttp.MountRoutes(r, handlers, config.Webhook{})
	return r
}

func newTestRouterWithModelAndStore(store *mockStore, model string) chi.Router {
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
	metaAgentSvc := service.NewMetaAgentService(store, litellm.NewClient("http://localhost:4000", ""), orchSvc, orchCfg, &config.Limits{})
	taskPlannerSvc := service.NewTaskPlannerService(metaAgentSvc, poolManagerSvc, store, orchCfg, &config.Limits{})
	contextOptSvc := service.NewContextOptimizerService(store, osfs.New(), orchCfg, &config.Limits{})
	sharedCtxSvc := service.NewSharedContextService(store, bc, queue)
	modeSvc := service.NewModeService()
	pipelineSvc := service.NewPipelineService(modeSvc)
	repoMapSvc := service.NewRepoMapService(store, queue, bc, orchCfg)
	retrievalSvc := service.NewRetrievalService(store, queue, bc, orchCfg, &config.Limits{})
	costSvc := service.NewCostService(store)
	settingsSvc := service.NewSettingsService(store)
	vcsAccountSvc := service.NewVCSAccountService(store, []byte("test-encryption-key-32bytes!!!!!"))
	conversationSvc := service.NewConversationService(store, bc, model, nil)
	conversationSvc.SetQueue(queue)
	authCfg := &config.Auth{
		Enabled:            true,
		JWTSecret:          "test-secret-key-32bytes-handler!",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		BcryptCost:         4,
	}
	authSvc := service.NewAuthService(store, authCfg)
	filesSvc := service.NewFileService(store, osfs.New())
	roadmapSvc := service.NewRoadmapService(store, bc, nil, nil)
	autoAgentSvc := service.NewAutoAgentService(store, bc, queue, conversationSvc)
	microagentSvc := service.NewMicroagentService(store)
	skillSvc := service.NewSkillService(store)
	memorySvc := service.NewMemoryService(store, queue)
	experiencePoolSvc := service.NewExperiencePoolService(store)
	kbSvc := service.NewKnowledgeBaseService(store)
	sessionSvc := service.NewSessionService(store, es)
	mcpSvc := service.NewMCPService(&config.MCP{}, &config.Limits{MCPTestTimeout: 10 * time.Second})
	mcpSvc.SetStore(store)
	handlers := &cfhttp.Handlers{
		Projects:         service.NewProjectService(store, os.TempDir()),
		Tasks:            service.NewTaskService(store, queue),
		Agents:           service.NewAgentService(store, queue, bc),
		LLM:              litellm.NewClient("http://localhost:4000", ""),
		Policies:         policySvc,
		Runtime:          runtimeSvc,
		Orchestrator:     orchSvc,
		MetaAgent:        metaAgentSvc,
		PoolManager:      poolManagerSvc,
		TaskPlanner:      taskPlannerSvc,
		ContextOptimizer: contextOptSvc,
		SharedContext:    sharedCtxSvc,
		Modes:            modeSvc,
		Pipelines:        pipelineSvc,
		RepoMap:          repoMapSvc,
		Retrieval:        retrievalSvc,
		Events:           es,
		Cost:             costSvc,
		Settings:         settingsSvc,
		VCSAccounts:      vcsAccountSvc,
		Conversations:    conversationSvc,
		Auth:             authSvc,
		Files:            filesSvc,
		Roadmap:          roadmapSvc,
		AutoAgent:        autoAgentSvc,
		Microagents:      microagentSvc,
		Skills:           skillSvc,
		Memory:           memorySvc,
		ExperiencePool:   experiencePoolSvc,
		KnowledgeBases:   kbSvc,
		Sessions:         sessionSvc,
		MCP:              mcpSvc,
		Scope:            service.NewScopeService(store),
		PromptSections:   service.NewPromptSectionService(store),
		Benchmarks: func() *service.BenchmarkService {
			suiteSvc := service.NewBenchmarkSuiteService(store, os.TempDir())
			runMgr := service.NewBenchmarkRunManager(store, suiteSvc)
			resultAgg := service.NewBenchmarkResultAggregator(store)
			watchdog := service.NewBenchmarkWatchdog(store)
			return service.NewBenchmarkService(suiteSvc, runMgr, resultAgg, watchdog)
		}(),
		ActiveWork:    service.NewActiveWorkService(store, bc),
		Routing:       service.NewRoutingService(store),
		GoalDiscovery: service.NewGoalDiscoveryService(store, osfs.New()),
		AppEnv:        os.Getenv("APP_ENV"),
		Limits: &config.Limits{
			MaxRequestBodySize: 1 << 20,
			MaxQueryLength:     2000,
			MaxFiles:           50,
			MaxFileSize:        32768,
			MaxInputLen:        10000,
			MaxEntries:         100,
		},
	}

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
				if middleware.UserFromContext(r.Context()) == nil {
					r = r.WithContext(middleware.ContextWithTestUser(r.Context(), &user.User{
						ID:   "test-admin",
						Name: "Test Admin",
						Role: user.RoleAdmin,
					}))
				}
			}
			next.ServeHTTP(w, r)
		})
	})
	cfhttp.MountRoutes(r, handlers, config.Webhook{})
	return r
}

func newTestRouterWithBackendHealth(bhSvc *service.BackendHealthService) chi.Router {
	r := newTestRouterWithStore(&mockStore{})
	// The default test router doesn't set BackendHealth; re-mount with it.
	// Instead of re-building, just use a minimal chi.Router with the handler.
	rr := chi.NewRouter()
	rr.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(middleware.ContextWithTestUser(r.Context(), &user.User{
				ID:   "test-admin",
				Name: "Test Admin",
				Role: user.RoleAdmin,
			})))
		})
	})
	h := &cfhttp.Handlers{
		BackendHealth: bhSvc,
	}
	rr.Get("/api/v1/backends/health", h.CheckBackendHealth)
	_ = r // suppress unused
	return rr
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
	if result["version"] == "" {
		t.Fatal("expected non-empty version string")
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

func TestHandler_CreateTaskInvalidBody(t *testing.T) {
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

func TestHandler_GetTaskNotFound(t *testing.T) {
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
	if len(profiles) != 5 {
		t.Fatalf("expected 5 profiles (5 presets), got %d: %v", len(profiles), profiles)
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

	// CancelRun calls GetRun which returns not found → 404 (mapped via writeDomainError)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cancel of nonexistent run, got %d", w.Code)
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
	metaAgentSvc := service.NewMetaAgentService(store, litellm.NewClient("http://localhost:4000", ""), orchSvc, orchCfg, &config.Limits{})
	taskPlannerSvc := service.NewTaskPlannerService(metaAgentSvc, poolManagerSvc, store, orchCfg, &config.Limits{})
	contextOptSvc := service.NewContextOptimizerService(store, osfs.New(), orchCfg, &config.Limits{})
	sharedCtxSvc := service.NewSharedContextService(store, bc, queue)
	modeSvc := service.NewModeService()
	repoMapSvc := service.NewRepoMapService(store, queue, bc, orchCfg)
	retrievalSvc := service.NewRetrievalService(store, queue, bc, orchCfg, &config.Limits{})
	handlers := &cfhttp.Handlers{
		Projects:         service.NewProjectService(store, os.TempDir()),
		Tasks:            service.NewTaskService(store, queue),
		Agents:           service.NewAgentService(store, queue, bc),
		LLM:              litellm.NewClient("http://localhost:4000", ""),
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
		Limits:           &config.Limits{MaxRequestBodySize: 1 << 20, MaxQueryLength: 2000},
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
	metaAgentSvc := service.NewMetaAgentService(store, litellm.NewClient("http://localhost:4000", ""), orchSvc, orchCfg, &config.Limits{})
	taskPlannerSvc := service.NewTaskPlannerService(metaAgentSvc, poolManagerSvc, store, orchCfg, &config.Limits{})
	contextOptSvc := service.NewContextOptimizerService(store, osfs.New(), orchCfg, &config.Limits{})
	sharedCtxSvc := service.NewSharedContextService(store, bc, queue)
	modeSvc := service.NewModeService()
	repoMapSvc := service.NewRepoMapService(store, queue, bc, orchCfg)
	retrievalSvc := service.NewRetrievalService(store, queue, bc, orchCfg, &config.Limits{})
	handlers := &cfhttp.Handlers{
		Projects:         service.NewProjectService(store, os.TempDir()),
		Tasks:            service.NewTaskService(store, queue),
		Agents:           service.NewAgentService(store, queue, bc),
		LLM:              litellm.NewClient("http://localhost:4000", ""),
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
		Limits:           &config.Limits{MaxRequestBodySize: 1 << 20, MaxQueryLength: 2000},
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
	if len(result["profiles"]) != 6 {
		t.Fatalf("expected 6 profiles (5 presets + 1 custom), got %d", len(result["profiles"]))
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

// --- Scope Endpoint Tests ---

func TestListScopesEmpty(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/scopes", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var scopes []cfcontext.RetrievalScope
	if err := json.NewDecoder(w.Body).Decode(&scopes); err != nil {
		t.Fatal(err)
	}
	if len(scopes) != 0 {
		t.Fatalf("expected empty slice, got %d items", len(scopes))
	}
}

func TestCreateScope(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(cfcontext.CreateScopeRequest{
		Name:       "cross-search",
		Type:       cfcontext.ScopeShared,
		ProjectIDs: []string{"proj-1"},
	})
	req := httptest.NewRequest("POST", "/api/v1/scopes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteScope(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("DELETE", "/api/v1/scopes/some-scope-id", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Benchmark Endpoint Tests ---

func TestListBenchmarkRunsDevMode(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/benchmarks/runs", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var runs []benchmark.Run
	if err := json.NewDecoder(w.Body).Decode(&runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected empty slice, got %d items", len(runs))
	}
}

func TestCreateBenchmarkRun(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	r := newTestRouter()

	body, _ := json.Marshal(benchmark.CreateRunRequest{
		Dataset: "swe-bench",
		Model:   "gpt-4",
		Metrics: []string{"llm_judge"},
	})
	req := httptest.NewRequest("POST", "/api/v1/benchmarks/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBenchmarksForbiddenWithoutDevMode(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/benchmarks/runs", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Prompt Section Endpoint Tests ---

func TestListPromptSectionsEmpty(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/prompt-sections", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var rows []prompt.SectionRow
	if err := json.NewDecoder(w.Body).Decode(&rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected empty slice, got %d items", len(rows))
	}
}

func TestUpsertPromptSectionMissingName(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(prompt.SectionRow{Content: "some content"})
	req := httptest.NewRequest("PUT", "/api/v1/prompt-sections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Git Status / Pull Endpoint Tests ---

func TestProjectGitStatusNotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/projects/nonexistent/git/status", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPullProjectNotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("POST", "/api/v1/projects/nonexistent/git/pull", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Phase 24: Active Work Visibility handler tests ---

func TestListActiveWorkEmpty(t *testing.T) {
	r := newTestRouterWithStore(&mockStore{
		projects: []project.Project{{ID: "p1", Name: "Test"}},
	})

	req := httptest.NewRequest("GET", "/api/v1/projects/p1/active-work", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var items []task.ActiveWorkItem
	if err := json.NewDecoder(w.Body).Decode(&items); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}

func TestListActiveWorkReturnsTasks(t *testing.T) {
	r := newTestRouterWithStore(&mockStore{
		projects: []project.Project{{ID: "p1", Name: "Test"}},
		activeWork: []task.ActiveWorkItem{
			{TaskID: "t1", TaskTitle: "Fix auth", TaskStatus: task.StatusRunning, ProjectID: "p1", AgentID: "a1", AgentName: "Coder"},
			{TaskID: "t2", TaskTitle: "Add tests", TaskStatus: task.StatusQueued, ProjectID: "p1", AgentID: "a2", AgentName: "Tester"},
		},
	})

	req := httptest.NewRequest("GET", "/api/v1/projects/p1/active-work", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var items []task.ActiveWorkItem
	if err := json.NewDecoder(w.Body).Decode(&items); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].AgentName != "Coder" {
		t.Errorf("items[0].agent_name = %q, want Coder", items[0].AgentName)
	}
}

func TestClaimTaskSuccess(t *testing.T) {
	r := newTestRouterWithStore(&mockStore{
		projects: []project.Project{{ID: "p1", Name: "Test"}},
		agents:   []agent.Agent{{ID: "a1", Name: "Coder", ProjectID: "p1"}},
		tasks:    []task.Task{{ID: "t1", ProjectID: "p1", Title: "Fix auth", Status: task.StatusPending, Version: 1}},
	})

	body := `{"agent_id":"a1"}`
	req := httptest.NewRequest("POST", "/api/v1/tasks/t1/claim", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result task.ClaimResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !result.Claimed {
		t.Fatalf("expected claimed=true, got false: %s", result.Reason)
	}
}

func TestClaimTaskAlreadyClaimed(t *testing.T) {
	r := newTestRouterWithStore(&mockStore{
		projects: []project.Project{{ID: "p1", Name: "Test"}},
		agents:   []agent.Agent{{ID: "a1", Name: "Coder", ProjectID: "p1"}},
		tasks:    []task.Task{{ID: "t1", ProjectID: "p1", Title: "Fix auth", Status: task.StatusRunning, Version: 2}},
	})

	body := `{"agent_id":"a1"}`
	req := httptest.NewRequest("POST", "/api/v1/tasks/t1/claim", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestClaimTaskMissingAgentID(t *testing.T) {
	r := newTestRouter()

	body := `{}`
	req := httptest.NewRequest("POST", "/api/v1/tasks/t1/claim", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestClaimTaskNotFound(t *testing.T) {
	r := newTestRouter()

	body := `{"agent_id":"a1"}`
	req := httptest.NewRequest("POST", "/api/v1/tasks/nonexistent/claim", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Routing store stubs (Phase 26) ---

func (m *mockStore) CreateRoutingOutcome(_ context.Context, o *routing.RoutingOutcome) error {
	m.routingOutcomes = append(m.routingOutcomes, *o)
	return nil
}
func (m *mockStore) ListRoutingStats(_ context.Context, taskType, tier string) ([]routing.ModelPerformanceStats, error) {
	if taskType == "" && tier == "" {
		return m.routingStats, nil
	}
	var filtered []routing.ModelPerformanceStats
	for i := range m.routingStats {
		if (taskType == "" || string(m.routingStats[i].TaskType) == taskType) &&
			(tier == "" || string(m.routingStats[i].ComplexityTier) == tier) {
			filtered = append(filtered, m.routingStats[i])
		}
	}
	return filtered, nil
}
func (m *mockStore) UpsertRoutingStats(_ context.Context, _ *routing.ModelPerformanceStats) error {
	return nil
}
func (m *mockStore) AggregateRoutingOutcomes(_ context.Context) error { return nil }
func (m *mockStore) ListRoutingOutcomes(_ context.Context, limit int) ([]routing.RoutingOutcome, error) {
	if limit <= 0 || limit > len(m.routingOutcomes) {
		return m.routingOutcomes, nil
	}
	return m.routingOutcomes[:limit], nil
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

// Project Goals (Phase 28)
func (m *mockStore) CreateProjectGoal(_ context.Context, g *goal.ProjectGoal) error {
	g.ID = fmt.Sprintf("goal-%d", len(m.goals)+1)
	g.Enabled = true
	m.goals = append(m.goals, *g)
	return nil
}
func (m *mockStore) GetProjectGoal(_ context.Context, id string) (*goal.ProjectGoal, error) {
	for i := range m.goals {
		if m.goals[i].ID == id {
			return &m.goals[i], nil
		}
	}
	return nil, errNotFound
}
func (m *mockStore) ListProjectGoals(_ context.Context, projectID string) ([]goal.ProjectGoal, error) {
	var result []goal.ProjectGoal
	for i := range m.goals {
		if m.goals[i].ProjectID == projectID {
			result = append(result, m.goals[i])
		}
	}
	return result, nil
}
func (m *mockStore) ListEnabledGoals(_ context.Context, projectID string) ([]goal.ProjectGoal, error) {
	var result []goal.ProjectGoal
	for i := range m.goals {
		if m.goals[i].ProjectID == projectID && m.goals[i].Enabled {
			result = append(result, m.goals[i])
		}
	}
	return result, nil
}
func (m *mockStore) UpdateProjectGoal(_ context.Context, g *goal.ProjectGoal) error {
	for i := range m.goals {
		if m.goals[i].ID == g.ID {
			m.goals[i] = *g
			return nil
		}
	}
	return errNotFound
}
func (m *mockStore) DeleteProjectGoal(_ context.Context, id string) error {
	for i := range m.goals {
		if m.goals[i].ID == id {
			m.goals = append(m.goals[:i], m.goals[i+1:]...)
			return nil
		}
	}
	return errNotFound
}
func (m *mockStore) DeleteProjectGoalsBySource(_ context.Context, projectID, source string) error {
	filtered := m.goals[:0]
	for i := range m.goals {
		if m.goals[i].ProjectID != projectID || m.goals[i].Source != source {
			filtered = append(filtered, m.goals[i])
		}
	}
	m.goals = filtered
	return nil
}

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
func (m *mockStore) ListAuditEntriesByAdmin(_ context.Context, _ string, _ int) ([]database.AuditEntry, error) {
	return nil, nil
}
func (m *mockStore) DeleteExpiredSessions(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}
func (m *mockStore) DeleteExpiredConversations(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}
func (m *mockStore) DeleteExpiredRuns(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}
func (m *mockStore) DeleteExpiredAuditEntries(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}
func (m *mockStore) AnonymizeAuditLogForUser(_ context.Context, _ string) (int64, error) {
	return 0, nil
}
func (m *mockStore) AnonymizeExpiredIPAddresses(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}

func TestListRemoteBranches_URLValidation(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		wantStatus int
		wantBody   string
	}{
		{"empty_url", "", http.StatusBadRequest, "url query parameter is required"},
		{"no_host", "just-a-path", http.StatusBadRequest, "invalid repository URL"},
		{"file_scheme", "file:///etc/passwd", http.StatusBadRequest, "invalid repository URL"},
		{"ftp_scheme", "ftp://example.com/repo", http.StatusBadRequest, "unsupported URL scheme"},
		{"javascript_scheme", "javascript:alert(1)", http.StatusBadRequest, "invalid repository URL"},
		{"https_valid_format", "https://github.com/user/repo", http.StatusBadGateway, "failed to list remote branches"},
		{"http_valid_format", "http://example.com/repo.git", http.StatusBadGateway, "failed to list remote branches"},
		{"ssh_valid_format", "ssh://git@github.com/user/repo", http.StatusBadGateway, "failed to list remote branches"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqURL := "/api/v1/repos/branches"
			if tt.url != "" {
				reqURL += "?url=" + neturl.QueryEscape(tt.url)
			}
			req := httptest.NewRequest(http.MethodGet, reqURL, http.NoBody)
			rr := httptest.NewRecorder()
			ph := &cfhttp.ProjectHandlers{}
			ph.ListRemoteBranches(rr, req)
			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
			if !strings.Contains(rr.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want substring %q", rr.Body.String(), tt.wantBody)
			}
		})
	}
}

// errorEventStore implements eventstore.Store and returns configurable errors for trajectory methods.
type errorEventStore struct {
	mockEventStore
	loadPage *eventstore.TrajectoryPage
	loadErr  error
	statsErr error
}

func (e *errorEventStore) LoadTrajectory(_ context.Context, _ string, _ eventstore.TrajectoryFilter, _ string, _ int) (*eventstore.TrajectoryPage, error) {
	return e.loadPage, e.loadErr
}

func (e *errorEventStore) TrajectoryStats(_ context.Context, _ string) (*eventstore.TrajectorySummary, error) {
	if e.statsErr != nil {
		return nil, e.statsErr
	}
	return &eventstore.TrajectorySummary{}, nil
}

func TestGetTrajectory_NoEvents_Returns200Empty(t *testing.T) {
	es := &errorEventStore{
		loadErr:  fmt.Errorf("no events found"),
		statsErr: fmt.Errorf("no events found"),
	}
	h := &cfhttp.Handlers{Events: es}

	req := httptest.NewRequest("GET", "/api/v1/runs/nonexistent-run/trajectory?limit=50", http.NoBody)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "nonexistent-run")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.GetTrajectory(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if _, ok := resp["events"]; !ok {
		t.Fatal("response missing 'events' key")
	}
	if _, ok := resp["stats"]; !ok {
		t.Fatal("response missing 'stats' key")
	}

	var events []json.RawMessage
	if err := json.Unmarshal(resp["events"], &events); err != nil {
		t.Fatalf("failed to decode events: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected empty events, got %d", len(events))
	}

	var hasMore bool
	if err := json.Unmarshal(resp["has_more"], &hasMore); err != nil {
		t.Fatalf("failed to decode has_more: %v", err)
	}
	if hasMore {
		t.Fatal("expected has_more=false")
	}
}

func TestGetTrajectory_LoadOK_StatsError_Returns200(t *testing.T) {
	es := &errorEventStore{
		loadPage: &eventstore.TrajectoryPage{},
		statsErr: fmt.Errorf("stats computation failed"),
	}
	h := &cfhttp.Handlers{Events: es}

	req := httptest.NewRequest("GET", "/api/v1/runs/some-run/trajectory", http.NoBody)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "some-run")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.GetTrajectory(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if _, ok := resp["stats"]; !ok {
		t.Fatal("response missing 'stats' key")
	}
}

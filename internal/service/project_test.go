package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
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

func (m *mockStore) CreateProject(_ context.Context, req project.CreateRequest) (*project.Project, error) {
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
func (m *mockStore) CreateFeature(_ context.Context, _ *roadmap.CreateFeatureRequest) (*roadmap.Feature, error) {
	return &roadmap.Feature{}, nil
}
func (m *mockStore) GetFeature(_ context.Context, _ string) (*roadmap.Feature, error) {
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
func (m *mockStore) ListSessions(_ context.Context, _ string) ([]run.Session, error) {
	return nil, nil
}
func (m *mockStore) UpdateSessionStatus(_ context.Context, _ string, _ run.SessionStatus, _ string) error {
	return nil
}

// --- User/Auth (in-memory implementation for auth tests) ---

func (m *mockStore) CreateUser(_ context.Context, u *user.User) error {
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

func (m *mockStore) DeleteAPIKey(_ context.Context, id string) error {
	for i := range m.apiKeys {
		if m.apiKeys[i].ID == id {
			m.apiKeys = append(m.apiKeys[:i], m.apiKeys[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

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
func (m *mockStore) AddProjectToScope(_ context.Context, _, _ string) error      { return nil }
func (m *mockStore) RemoveProjectFromScope(_ context.Context, _, _ string) error { return nil }

// --- ProjectService Tests ---

func TestProjectServiceList(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{
			{ID: "p1", Name: "Alpha"},
			{ID: "p2", Name: "Beta"},
		},
	}
	svc := NewProjectService(store)

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
	svc := NewProjectService(store)

	_, err := svc.List(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestProjectServiceGet(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "Alpha"}},
	}
	svc := NewProjectService(store)

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
	svc := NewProjectService(store)

	_, err := svc.Get(context.Background(), "nonexistent")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestProjectServiceCreate(t *testing.T) {
	store := &mockStore{}
	svc := NewProjectService(store)

	req := project.CreateRequest{Name: "New", Provider: "local"}
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
	svc := NewProjectService(store)

	_, err := svc.Create(context.Background(), project.CreateRequest{Name: "X"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestProjectServiceDelete(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "Alpha"}},
	}
	svc := NewProjectService(store)

	if err := svc.Delete(context.Background(), "p1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.projects) != 0 {
		t.Fatalf("expected 0 projects after delete, got %d", len(store.projects))
	}
}

func TestProjectServiceDeleteNotFound(t *testing.T) {
	store := &mockStore{}
	svc := NewProjectService(store)

	err := svc.Delete(context.Background(), "nonexistent")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestProjectServiceCloneNoRepoURL(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "No Repo", Provider: "local"}},
	}
	svc := NewProjectService(store)

	_, err := svc.Clone(context.Background(), "p1")
	if err == nil {
		t.Fatal("expected error for project without repo_url")
	}
	if got := err.Error(); got != "project p1 has no repo_url" {
		t.Fatalf("unexpected error message: %s", got)
	}
}

func TestProjectServiceCloneNotFound(t *testing.T) {
	store := &mockStore{}
	svc := NewProjectService(store)

	_, err := svc.Clone(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestProjectServiceStatusNoWorkspace(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "No WS"}},
	}
	svc := NewProjectService(store)

	_, err := svc.Status(context.Background(), "p1")
	if err == nil {
		t.Fatal("expected error for project without workspace")
	}
}

func TestProjectServicePullNoWorkspace(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "No WS"}},
	}
	svc := NewProjectService(store)

	err := svc.Pull(context.Background(), "p1")
	if err == nil {
		t.Fatal("expected error for project without workspace")
	}
}

func TestProjectServiceListBranchesNoWorkspace(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "No WS"}},
	}
	svc := NewProjectService(store)

	_, err := svc.ListBranches(context.Background(), "p1")
	if err == nil {
		t.Fatal("expected error for project without workspace")
	}
}

func TestProjectServiceCheckoutNoWorkspace(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{ID: "p1", Name: "No WS"}},
	}
	svc := NewProjectService(store)

	err := svc.Checkout(context.Background(), "p1", "main")
	if err == nil {
		t.Fatal("expected error for project without workspace")
	}
}

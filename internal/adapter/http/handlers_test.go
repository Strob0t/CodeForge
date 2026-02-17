package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/resource"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
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

func (m *mockStore) CreateProject(_ context.Context, req project.CreateRequest) (*project.Project, error) {
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

func (m *mockStore) UpdateRunStatus(_ context.Context, id string, status run.Status, stepCount int, costUSD float64) error {
	for i := range m.runs {
		if m.runs[i].ID == id {
			m.runs[i].Status = status
			m.runs[i].StepCount = stepCount
			m.runs[i].CostUSD = costUSD
			return nil
		}
	}
	return errNotFound
}

func (m *mockStore) CompleteRun(_ context.Context, id string, status run.Status, output, errMsg string, costUSD float64, stepCount int) error {
	for i := range m.runs {
		if m.runs[i].ID != id {
			continue
		}
		m.runs[i].Status = status
		m.runs[i].Output = output
		m.runs[i].Error = errMsg
		m.runs[i].CostUSD = costUSD
		m.runs[i].StepCount = stepCount
		return nil
	}
	return errNotFound
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
	handlers := &cfhttp.Handlers{
		Projects:         service.NewProjectService(store),
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
	}

	r := chi.NewRouter()
	cfhttp.MountRoutes(r, handlers)
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

	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["decision"] != "allow" {
		t.Fatalf("expected 'allow' for Read in plan-readonly, got %q", result["decision"])
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
		Projects:         service.NewProjectService(store),
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
	}

	r := chi.NewRouter()
	cfhttp.MountRoutes(r, handlers)

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
		Projects:         service.NewProjectService(store),
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
	}

	r := chi.NewRouter()
	cfhttp.MountRoutes(r, handlers)

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

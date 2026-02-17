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
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// mockStore implements database.Store for testing.
type mockStore struct {
	projects []project.Project
	agents   []agent.Agent
	tasks    []task.Task
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

func (m *mockStore) CreateAgent(_ context.Context, projectID, name, backend string, config map[string]string) (*agent.Agent, error) {
	a := agent.Agent{
		ID:        "agent-id",
		ProjectID: projectID,
		Name:      name,
		Backend:   backend,
		Status:    agent.StatusIdle,
		Config:    config,
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

var errNotFound = fmt.Errorf("mock: %w", domain.ErrNotFound)

func newTestRouter() chi.Router {
	store := &mockStore{}
	queue := &mockQueue{}
	bc := &mockBroadcaster{}
	handlers := &cfhttp.Handlers{
		Projects: service.NewProjectService(store),
		Tasks:    service.NewTaskService(store, queue),
		Agents:   service.NewAgentService(store, queue, bc),
		LiteLLM:  litellm.NewClient("http://localhost:4000", ""),
		Policies: service.NewPolicyService("headless-safe-sandbox", nil),
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

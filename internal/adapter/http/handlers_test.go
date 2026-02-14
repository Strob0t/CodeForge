package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// mockStore implements database.Store for testing.
type mockStore struct {
	projects []project.Project
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

func (m *mockStore) DeleteProject(_ context.Context, id string) error {
	for i := range m.projects {
		if m.projects[i].ID == id {
			m.projects = append(m.projects[:i], m.projects[i+1:]...)
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

func (m *mockQueue) Close() error { return nil }

var errNotFound = http.ErrNoCookie // reuse any error for tests

func newTestRouter() chi.Router {
	store := &mockStore{}
	queue := &mockQueue{}
	handlers := &cfhttp.Handlers{
		Projects: service.NewProjectService(store),
		Tasks:    service.NewTaskService(store, queue),
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

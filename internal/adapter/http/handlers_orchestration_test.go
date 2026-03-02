package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/mode"
	"github.com/Strob0t/CodeForge/internal/domain/pipeline"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
)

// --- Plan CRUD ---

func TestHandleCreatePlan(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	// Seed the store directly — avoids agent backend validation.
	createProject(t, r, "plan-project")
	createTask(t, r, "test-id", "plan-task")
	store.agents = append(store.agents, agent.Agent{ID: "agent-1", ProjectID: "test-id", Name: "test-agent"})

	body, _ := json.Marshal(plan.CreatePlanRequest{
		Name:     "test-plan",
		Protocol: plan.ProtocolSequential,
		Steps: []plan.CreateStepRequest{
			{TaskID: "task-id", AgentID: "agent-1"},
		},
	})
	req := httptest.NewRequest("POST", "/api/v1/projects/test-id/plans", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var p plan.ExecutionPlan
	if err := json.NewDecoder(w.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	if p.Name != "test-plan" {
		t.Fatalf("expected name 'test-plan', got %q", p.Name)
	}
}

func TestHandleListPlans_Empty(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)
	createProject(t, r, "plan-project")

	req := httptest.NewRequest("GET", "/api/v1/projects/test-id/plans", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var plans []plan.ExecutionPlan
	if err := json.NewDecoder(w.Body).Decode(&plans); err != nil {
		t.Fatal(err)
	}
	if len(plans) != 0 {
		t.Fatalf("expected empty list, got %d", len(plans))
	}
}

func TestHandleGetPlan_NotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/plans/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleStartPlan_NotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("POST", "/api/v1/plans/nonexistent/start", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCancelPlan_NotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("POST", "/api/v1/plans/nonexistent/cancel", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Plan Graph & Evaluate ---

func TestHandleGetPlanGraph_NotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/plans/nonexistent/graph", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleEvaluateStep_NoReviewRouter(t *testing.T) {
	// The default test router has nil ReviewRouter.
	r := newTestRouter()

	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest("POST", "/api/v1/plans/some-plan/steps/some-step/evaluate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Modes CRUD ---

func TestHandleListModes(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/modes", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var modes []mode.Mode
	if err := json.NewDecoder(w.Body).Decode(&modes); err != nil {
		t.Fatal(err)
	}
	// ModeService is initialized with built-in modes.
	if len(modes) == 0 {
		t.Fatal("expected built-in modes, got empty list")
	}
}

func TestHandleGetMode_NotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/modes/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateMode(t *testing.T) {
	r := newTestRouter()

	m := mode.Mode{
		ID:          "test-custom-mode",
		Name:        "Test Custom Mode",
		Description: "A test mode",
		Autonomy:    3,
		LLMScenario: "default",
		Tools:       []string{"read", "write"},
	}
	body, _ := json.Marshal(m)
	req := httptest.NewRequest("POST", "/api/v1/modes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var created mode.Mode
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ID != "test-custom-mode" {
		t.Fatalf("expected id 'test-custom-mode', got %q", created.ID)
	}
}

func TestHandleUpdateMode(t *testing.T) {
	r := newTestRouter()

	// First create a custom mode.
	m := mode.Mode{
		ID:          "update-mode",
		Name:        "Update Mode",
		Description: "Before",
		Autonomy:    2,
		LLMScenario: "default",
	}
	body, _ := json.Marshal(m)
	req := httptest.NewRequest("POST", "/api/v1/modes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create mode: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Update it.
	m.Description = "After"
	body, _ = json.Marshal(m)
	req = httptest.NewRequest("PUT", "/api/v1/modes/update-mode", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated mode.Mode
	if err := json.NewDecoder(w.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.Description != "After" {
		t.Fatalf("expected description 'After', got %q", updated.Description)
	}
}

func TestHandleDeleteMode(t *testing.T) {
	r := newTestRouter()

	// Create a custom mode.
	m := mode.Mode{
		ID:          "delete-mode",
		Name:        "Delete Mode",
		Description: "To be deleted",
		Autonomy:    1,
		LLMScenario: "default",
	}
	body, _ := json.Marshal(m)
	req := httptest.NewRequest("POST", "/api/v1/modes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create mode: expected 201, got %d", w.Code)
	}

	// Delete it.
	req = httptest.NewRequest("DELETE", "/api/v1/modes/delete-mode", http.NoBody)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify it's gone.
	req = httptest.NewRequest("GET", "/api/v1/modes/delete-mode", http.NoBody)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", w.Code)
	}
}

func TestHandleListScenarios(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/modes/scenarios", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var scenarios []string
	if err := json.NewDecoder(w.Body).Decode(&scenarios); err != nil {
		t.Fatal(err)
	}
	if len(scenarios) == 0 {
		t.Fatal("expected non-empty scenario list")
	}
}

func TestHandleListModeTools(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/modes/tools", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var tools []string
	if err := json.NewDecoder(w.Body).Decode(&tools); err != nil {
		t.Fatal(err)
	}
	if len(tools) == 0 {
		t.Fatal("expected non-empty tools list")
	}
	// Verify a known built-in tool is present.
	found := false
	for _, tool := range tools {
		if tool == "Read" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'Read' in tools list")
	}
}

func TestHandleListArtifactTypes(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/modes/artifact-types", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var types []string
	if err := json.NewDecoder(w.Body).Decode(&types); err != nil {
		t.Fatal(err)
	}
	if len(types) == 0 {
		t.Fatal("expected non-empty artifact types list")
	}
	// Verify a known type is present.
	found := false
	for _, at := range types {
		if at == "DIFF" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'DIFF' in artifact types list")
	}
}

// --- Pipelines ---

func TestHandleListPipelines_Empty(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/pipelines", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var templates []pipeline.Template
	if err := json.NewDecoder(w.Body).Decode(&templates); err != nil {
		t.Fatal(err)
	}
	// PipelineService is pre-loaded with built-in templates.
	// We just verify we get a valid response.
}

func TestHandleGetPipeline_NotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/pipelines/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRegisterPipeline(t *testing.T) {
	r := newTestRouter()

	// Register a custom mode that our pipeline will reference.
	m := mode.Mode{
		ID:          "pipe-mode",
		Name:        "Pipe Mode",
		Description: "Mode for pipeline test",
		Autonomy:    3,
		LLMScenario: "default",
	}
	mBody, _ := json.Marshal(m)
	req := httptest.NewRequest("POST", "/api/v1/modes", bytes.NewReader(mBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create mode: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	tmpl := pipeline.Template{
		ID:       "test-pipeline",
		Name:     "Test Pipeline",
		Protocol: plan.ProtocolSequential,
		Steps: []pipeline.Step{
			{Name: "step-1", ModeID: "pipe-mode"},
		},
	}
	body, _ := json.Marshal(tmpl)
	req = httptest.NewRequest("POST", "/api/v1/pipelines", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var created pipeline.Template
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ID != "test-pipeline" {
		t.Fatalf("expected id 'test-pipeline', got %q", created.ID)
	}
}

// --- Context Pack ---

func TestHandleGetContextPack_NotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/tasks/nonexistent/context", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleBuildContextPack_MissingProjectID(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest("POST", "/api/v1/tasks/some-task/context", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Helpers for creating test data via HTTP ---

func createProject(t *testing.T, router http.Handler, name string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"name": name, "provider": "local"})
	req := httptest.NewRequest("POST", "/api/v1/projects", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("createProject: expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func createTask(t *testing.T, router http.Handler, projectID, title string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"title": title})
	req := httptest.NewRequest("POST", "/api/v1/projects/"+projectID+"/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("createTask: expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

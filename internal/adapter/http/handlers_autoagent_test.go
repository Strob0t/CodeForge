package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/autoagent"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
)

// --- Auto-Agent Handler Tests ---

func TestHandleGetAutoAgentStatus_Idle(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	// Create a project so the route doesn't 404.
	store.projects = append(store.projects, project.Project{
		ID:            "proj-1",
		Name:          "test-project",
		WorkspacePath: "/tmp/test-ws",
	})

	req := httptest.NewRequest("GET", "/api/v1/projects/proj-1/auto-agent/status", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var aa autoagent.AutoAgent
	if err := json.NewDecoder(w.Body).Decode(&aa); err != nil {
		t.Fatal(err)
	}
	if aa.Status != autoagent.StatusIdle {
		t.Fatalf("expected idle status, got %q", aa.Status)
	}
	if aa.ProjectID != "proj-1" {
		t.Fatalf("expected project_id 'proj-1', got %q", aa.ProjectID)
	}
}

func TestHandleStopAutoAgent(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	store.projects = append(store.projects, project.Project{
		ID:   "proj-1",
		Name: "test-project",
	})

	req := httptest.NewRequest("POST", "/api/v1/projects/proj-1/auto-agent/stop", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "stopping" {
		t.Fatalf("expected status 'stopping', got %q", result["status"])
	}
}

func TestHandleStartAutoAgent_NoWorkspace(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	// Project exists but has no WorkspacePath.
	store.projects = append(store.projects, project.Project{
		ID:   "proj-1",
		Name: "test-project",
	})

	req := httptest.NewRequest("POST", "/api/v1/projects/proj-1/auto-agent/start", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleStartAutoAgent_NoFeatures(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	// Project with workspace but no roadmap features.
	store.projects = append(store.projects, project.Project{
		ID:            "proj-1",
		Name:          "test-project",
		WorkspacePath: "/tmp/test-ws",
	})

	// Seed a roadmap with a milestone but no backlog/planned features,
	// so pendingFeatures returns an empty slice → validation error (400).
	store.roadmaps = append(store.roadmaps, roadmap.Roadmap{
		ID:        "rm-1",
		ProjectID: "proj-1",
		Title:     "Empty Roadmap",
	})
	store.milestones = append(store.milestones, roadmap.Milestone{
		ID:        "ms-1",
		RoadmapID: "rm-1",
		Title:     "Milestone 1",
	})
	// Feature exists but is already done — not pending.
	store.features = append(store.features, roadmap.Feature{
		ID:          "feat-1",
		MilestoneID: "ms-1",
		RoadmapID:   "rm-1",
		Title:       "Done Feature",
		Status:      roadmap.FeatureDone,
	})

	req := httptest.NewRequest("POST", "/api/v1/projects/proj-1/auto-agent/start", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Should fail because no pending features exist.
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleStartAutoAgent_Success(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	store.projects = append(store.projects, project.Project{
		ID:            "proj-1",
		Name:          "test-project",
		WorkspacePath: "/tmp/test-ws",
	})

	// Seed roadmap with a pending feature.
	store.roadmaps = append(store.roadmaps, roadmap.Roadmap{
		ID:        "rm-1",
		ProjectID: "proj-1",
		Title:     "Test Roadmap",
	})
	store.milestones = append(store.milestones, roadmap.Milestone{
		ID:        "ms-1",
		RoadmapID: "rm-1",
		Title:     "Milestone 1",
	})
	store.features = append(store.features, roadmap.Feature{
		ID:          "feat-1",
		MilestoneID: "ms-1",
		RoadmapID:   "rm-1",
		Title:       "Feature 1",
		Status:      roadmap.FeatureBacklog,
	})

	req := httptest.NewRequest("POST", "/api/v1/projects/proj-1/auto-agent/start", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var aa autoagent.AutoAgent
	if err := json.NewDecoder(w.Body).Decode(&aa); err != nil {
		t.Fatal(err)
	}
	if aa.Status != autoagent.StatusRunning {
		t.Fatalf("expected running status, got %q", aa.Status)
	}
	if aa.FeaturesTotal != 1 {
		t.Fatalf("expected 1 total feature, got %d", aa.FeaturesTotal)
	}
}

func TestHandleStartAutoAgent_ProjectNotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("POST", "/api/v1/projects/nonexistent/auto-agent/start", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

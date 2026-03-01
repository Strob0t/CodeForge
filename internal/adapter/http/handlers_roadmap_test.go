package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
)

// --- Roadmap Handler Tests ---

func TestHandleCreateProjectRoadmap_Success(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	store.projects = append(store.projects, project.Project{ID: "proj-1", Name: "test"})

	body, _ := json.Marshal(roadmap.CreateRoadmapRequest{Title: "v1.0 Roadmap"})
	req := httptest.NewRequest("POST", "/api/v1/projects/proj-1/roadmap", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var rm roadmap.Roadmap
	if err := json.NewDecoder(w.Body).Decode(&rm); err != nil {
		t.Fatal(err)
	}
	if rm.Title != "v1.0 Roadmap" {
		t.Fatalf("expected title 'v1.0 Roadmap', got %q", rm.Title)
	}
	if rm.ProjectID != "proj-1" {
		t.Fatalf("expected project_id 'proj-1', got %q", rm.ProjectID)
	}
}

func TestHandleCreateProjectRoadmap_MissingTitle(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	store.projects = append(store.projects, project.Project{ID: "proj-1", Name: "test"})

	body, _ := json.Marshal(roadmap.CreateRoadmapRequest{})
	req := httptest.NewRequest("POST", "/api/v1/projects/proj-1/roadmap", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetProjectRoadmap_Success(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	store.projects = append(store.projects, project.Project{ID: "proj-1", Name: "test"})
	store.roadmaps = append(store.roadmaps, roadmap.Roadmap{
		ID:        "rm-1",
		ProjectID: "proj-1",
		Title:     "Test Roadmap",
		Status:    roadmap.StatusActive,
	})

	req := httptest.NewRequest("GET", "/api/v1/projects/proj-1/roadmap", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var rm roadmap.Roadmap
	if err := json.NewDecoder(w.Body).Decode(&rm); err != nil {
		t.Fatal(err)
	}
	if rm.Title != "Test Roadmap" {
		t.Fatalf("expected title 'Test Roadmap', got %q", rm.Title)
	}
}

func TestHandleGetProjectRoadmap_NotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/projects/nonexistent/roadmap", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeleteProjectRoadmap(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	store.projects = append(store.projects, project.Project{ID: "proj-1", Name: "test"})
	store.roadmaps = append(store.roadmaps, roadmap.Roadmap{
		ID:        "rm-1",
		ProjectID: "proj-1",
		Title:     "To Delete",
	})

	req := httptest.NewRequest("DELETE", "/api/v1/projects/proj-1/roadmap", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify it's gone.
	req = httptest.NewRequest("GET", "/api/v1/projects/proj-1/roadmap", http.NoBody)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", w.Code)
	}
}

func TestHandleCreateMilestone_Success(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	store.projects = append(store.projects, project.Project{ID: "proj-1", Name: "test"})
	store.roadmaps = append(store.roadmaps, roadmap.Roadmap{
		ID:        "rm-1",
		ProjectID: "proj-1",
		Title:     "Test Roadmap",
	})

	body, _ := json.Marshal(roadmap.CreateMilestoneRequest{Title: "Sprint 1"})
	req := httptest.NewRequest("POST", "/api/v1/projects/proj-1/roadmap/milestones", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var ms roadmap.Milestone
	if err := json.NewDecoder(w.Body).Decode(&ms); err != nil {
		t.Fatal(err)
	}
	if ms.Title != "Sprint 1" {
		t.Fatalf("expected title 'Sprint 1', got %q", ms.Title)
	}
	if ms.RoadmapID != "rm-1" {
		t.Fatalf("expected roadmap_id 'rm-1', got %q", ms.RoadmapID)
	}
}

func TestHandleCreateMilestone_MissingTitle(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	store.projects = append(store.projects, project.Project{ID: "proj-1", Name: "test"})
	store.roadmaps = append(store.roadmaps, roadmap.Roadmap{
		ID:        "rm-1",
		ProjectID: "proj-1",
		Title:     "Test Roadmap",
	})

	body, _ := json.Marshal(roadmap.CreateMilestoneRequest{})
	req := httptest.NewRequest("POST", "/api/v1/projects/proj-1/roadmap/milestones", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateFeature_Success(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	store.milestones = append(store.milestones, roadmap.Milestone{
		ID:        "ms-1",
		RoadmapID: "rm-1",
		Title:     "Sprint 1",
	})

	body, _ := json.Marshal(roadmap.CreateFeatureRequest{Title: "Add auth"})
	req := httptest.NewRequest("POST", "/api/v1/milestones/ms-1/features", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var f roadmap.Feature
	if err := json.NewDecoder(w.Body).Decode(&f); err != nil {
		t.Fatal(err)
	}
	if f.Title != "Add auth" {
		t.Fatalf("expected title 'Add auth', got %q", f.Title)
	}
	if f.MilestoneID != "ms-1" {
		t.Fatalf("expected milestone_id 'ms-1', got %q", f.MilestoneID)
	}
}

func TestHandleDeleteFeature(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	store.features = append(store.features, roadmap.Feature{
		ID:          "feat-1",
		MilestoneID: "ms-1",
		Title:       "To Delete",
	})

	req := httptest.NewRequest("DELETE", "/api/v1/features/feat-1", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetMilestone_NotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/milestones/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetFeature_NotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/features/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListSpecProviders(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/providers/spec", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Response should be a valid JSON array (may be empty if no providers registered).
	var providers []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&providers); err != nil {
		t.Fatal(err)
	}
}

func TestHandleListPMProviders(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/providers/pm", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var providers []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&providers); err != nil {
		t.Fatal(err)
	}
}

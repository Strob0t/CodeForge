package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/goal"
	"github.com/Strob0t/CodeForge/internal/domain/project"
)

func TestHandleListProjectGoals_Empty(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/projects/proj-1/goals", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var goals []goal.ProjectGoal
	if err := json.NewDecoder(w.Body).Decode(&goals); err != nil {
		t.Fatal(err)
	}
	if len(goals) != 0 {
		t.Fatalf("expected empty list, got %d", len(goals))
	}
}

func TestHandleListProjectGoals_Populated(t *testing.T) {
	store := &mockStore{
		goals: []goal.ProjectGoal{
			{ID: "g1", ProjectID: "proj-1", Kind: goal.KindVision, Title: "Vision", Content: "Build X"},
			{ID: "g2", ProjectID: "proj-1", Kind: goal.KindConstraint, Title: "Rules", Content: "Use Go"},
			{ID: "g3", ProjectID: "proj-2", Kind: goal.KindVision, Title: "Other", Content: "Different project"},
		},
	}
	r := newTestRouterWithStore(store)

	req := httptest.NewRequest("GET", "/api/v1/projects/proj-1/goals", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var goals []goal.ProjectGoal
	if err := json.NewDecoder(w.Body).Decode(&goals); err != nil {
		t.Fatal(err)
	}
	if len(goals) != 2 {
		t.Fatalf("expected 2 goals for proj-1, got %d", len(goals))
	}
}

func TestHandleCreateProjectGoal(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(goal.CreateRequest{
		Kind:    goal.KindVision,
		Title:   "Project Vision",
		Content: "We build amazing things.",
	})
	req := httptest.NewRequest("POST", "/api/v1/projects/proj-1/goals", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var g goal.ProjectGoal
	if err := json.NewDecoder(w.Body).Decode(&g); err != nil {
		t.Fatal(err)
	}
	if g.Title != "Project Vision" {
		t.Fatalf("expected title 'Project Vision', got %q", g.Title)
	}
	if g.ProjectID != "proj-1" {
		t.Fatalf("expected project_id 'proj-1', got %q", g.ProjectID)
	}
}

func TestHandleCreateProjectGoal_Validation(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(goal.CreateRequest{
		Kind:  "invalid",
		Title: "Test",
	})
	req := httptest.NewRequest("POST", "/api/v1/projects/proj-1/goals", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetProjectGoal(t *testing.T) {
	store := &mockStore{
		goals: []goal.ProjectGoal{
			{ID: "g1", ProjectID: "proj-1", Kind: goal.KindVision, Title: "Vision", Content: "Build X"},
		},
	}
	r := newTestRouterWithStore(store)

	req := httptest.NewRequest("GET", "/api/v1/goals/g1", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var g goal.ProjectGoal
	if err := json.NewDecoder(w.Body).Decode(&g); err != nil {
		t.Fatal(err)
	}
	if g.ID != "g1" {
		t.Fatalf("expected id 'g1', got %q", g.ID)
	}
}

func TestHandleUpdateProjectGoal(t *testing.T) {
	store := &mockStore{
		goals: []goal.ProjectGoal{
			{ID: "g1", ProjectID: "proj-1", Kind: goal.KindVision, Title: "Vision", Content: "Build X"},
		},
	}
	r := newTestRouterWithStore(store)

	newTitle := "Updated Vision"
	body, _ := json.Marshal(goal.UpdateRequest{Title: &newTitle})
	req := httptest.NewRequest("PUT", "/api/v1/goals/g1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var g goal.ProjectGoal
	if err := json.NewDecoder(w.Body).Decode(&g); err != nil {
		t.Fatal(err)
	}
	if g.Title != "Updated Vision" {
		t.Fatalf("expected title 'Updated Vision', got %q", g.Title)
	}
}

func TestHandleDeleteProjectGoal(t *testing.T) {
	store := &mockStore{
		goals: []goal.ProjectGoal{
			{ID: "g1", ProjectID: "proj-1", Kind: goal.KindVision, Title: "Vision", Content: "Build X"},
		},
	}
	r := newTestRouterWithStore(store)

	req := httptest.NewRequest("DELETE", "/api/v1/goals/g1", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDetectProjectGoals(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{
			{ID: "proj-1", Name: "Test", WorkspacePath: "/tmp/nonexistent-workspace"},
		},
	}
	r := newTestRouterWithStore(store)

	req := httptest.NewRequest("POST", "/api/v1/projects/proj-1/goals/detect", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

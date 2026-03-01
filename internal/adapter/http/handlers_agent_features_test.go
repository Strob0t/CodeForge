package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/experience"
	"github.com/Strob0t/CodeForge/internal/domain/memory"
	"github.com/Strob0t/CodeForge/internal/domain/microagent"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/skill"
)

// --- Microagent Handler Tests ---

func TestHandleCreateMicroagent(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(microagent.CreateRequest{
		Name:           "error-helper",
		Type:           microagent.TypeKnowledge,
		TriggerPattern: "error|exception",
		Description:    "Helps debug errors",
		Prompt:         "You are an error debugging assistant.",
	})
	req := httptest.NewRequest("POST", "/api/v1/projects/test-proj/microagents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var ma microagent.Microagent
	if err := json.NewDecoder(w.Body).Decode(&ma); err != nil {
		t.Fatal(err)
	}
	if ma.Name != "error-helper" {
		t.Fatalf("expected name 'error-helper', got %q", ma.Name)
	}
	if ma.ProjectID != "test-proj" {
		t.Fatalf("expected project_id 'test-proj', got %q", ma.ProjectID)
	}
	if !ma.Enabled {
		t.Fatal("expected microagent to be enabled by default")
	}
}

func TestHandleListMicroagents_Empty(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/projects/test-proj/microagents", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var agents []microagent.Microagent
	if err := json.NewDecoder(w.Body).Decode(&agents); err != nil {
		t.Fatal(err)
	}
	if len(agents) != 0 {
		t.Fatalf("expected empty list, got %d", len(agents))
	}
}

func TestHandleDeleteMicroagent(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	store.microagents = append(store.microagents, microagent.Microagent{
		ID:   "ma-1",
		Name: "to-delete",
	})

	req := httptest.NewRequest("DELETE", "/api/v1/microagents/ma-1", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Skill Handler Tests ---

func TestHandleCreateSkill(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(skill.CreateRequest{
		Name:        "git-helper",
		Description: "Git command skills",
		Language:    "python",
		Code:        "def git_status(): ...",
		Tags:        []string{"git", "vcs"},
	})
	req := httptest.NewRequest("POST", "/api/v1/projects/test-proj/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var sk skill.Skill
	if err := json.NewDecoder(w.Body).Decode(&sk); err != nil {
		t.Fatal(err)
	}
	if sk.Name != "git-helper" {
		t.Fatalf("expected name 'git-helper', got %q", sk.Name)
	}
	if sk.ProjectID != "test-proj" {
		t.Fatalf("expected project_id 'test-proj', got %q", sk.ProjectID)
	}
}

func TestHandleListSkills_Empty(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/projects/test-proj/skills", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var skills []skill.Skill
	if err := json.NewDecoder(w.Body).Decode(&skills); err != nil {
		t.Fatal(err)
	}
	if len(skills) != 0 {
		t.Fatalf("expected empty list, got %d", len(skills))
	}
}

func TestHandleDeleteSkill(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	store.skills = append(store.skills, skill.Skill{
		ID:   "sk-1",
		Name: "to-delete",
	})

	req := httptest.NewRequest("DELETE", "/api/v1/skills/sk-1", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Memory Handler Tests ---

func TestHandleListMemories_Empty(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/projects/test-proj/memories", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var memories []memory.Memory
	if err := json.NewDecoder(w.Body).Decode(&memories); err != nil {
		t.Fatal(err)
	}
	if len(memories) != 0 {
		t.Fatalf("expected empty list, got %d", len(memories))
	}
}

func TestHandleStoreMemory(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(memory.CreateRequest{
		Content:    "The user prefers Go over Python",
		Kind:       memory.KindObservation,
		Importance: 0.8,
	})
	req := httptest.NewRequest("POST", "/api/v1/projects/test-proj/memories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Memory store dispatches to NATS, returns 202.
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Experience Pool Handler Tests ---

func TestHandleListExperienceEntries_Empty(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/projects/test-proj/experience", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var entries []experience.Entry
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty list, got %d", len(entries))
	}
}

func TestHandleDeleteExperienceEntry(t *testing.T) {
	r := newTestRouter()

	// DeleteExperienceEntry uses the stub which returns nil — so this succeeds.
	req := httptest.NewRequest("DELETE", "/api/v1/experience/entry-1", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Policy Handler Tests ---

func TestHandleListPolicies(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/policies", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result struct {
		Profiles []string `json:"profiles"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	// PolicyService is initialized with built-in presets.
	if len(result.Profiles) == 0 {
		t.Fatal("expected built-in policy profiles, got empty list")
	}
}

func TestHandleGetPolicyProfile_NotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/policies/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleEvaluatePolicy(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(policy.ToolCall{
		Tool:    "read",
		Command: "",
		Path:    "/src/main.go",
	})
	req := httptest.NewRequest("POST", "/api/v1/policies/headless-safe-sandbox/evaluate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["decision"] == nil {
		t.Fatal("expected decision field in response")
	}
}

// --- Feedback Handler Tests ---

func TestHandleListFeedbackAudit_Empty(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/runs/run-1/feedback", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

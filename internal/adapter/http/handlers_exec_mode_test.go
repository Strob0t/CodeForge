package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/task"
)

// Sandbox and hybrid execution modes must fail at start with a 400 that tells the
// user why, because the worker would run every tool without isolation (KI-13).

func unavailableExecModeMessage(mode string) string {
	return mode + " execution mode is not available yet: tools would run without isolation (KI-13)"
}

func decodeErrorMessage(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	return body.Error
}

func newExecModeRunStore(projectConfig map[string]string) *mockStore {
	return &mockStore{
		projects: []project.Project{{ID: "proj-1", Name: "p", WorkspacePath: "/tmp/ws", Config: projectConfig}},
		agents:   []agent.Agent{{ID: "agent-1", ProjectID: "proj-1", Name: "a", Backend: "aider", Config: map[string]string{}}},
		tasks:    []task.Task{{ID: "task-1", ProjectID: "proj-1", Title: "t", Prompt: "do it"}},
	}
}

func TestStartRun_UnavailableExecModeReturns400(t *testing.T) {
	tests := []struct {
		name          string
		execMode      string
		projectConfig map[string]string
		wantMode      string
	}{
		{name: "explicit sandbox", execMode: "sandbox", wantMode: "sandbox"},
		{name: "explicit hybrid", execMode: "hybrid", wantMode: "hybrid"},
		{name: "project default sandbox", projectConfig: map[string]string{"execution_mode": "sandbox"}, wantMode: "sandbox"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newExecModeRunStore(tc.projectConfig)
			r := newTestRouterWithStore(store)

			body, _ := json.Marshal(map[string]string{
				"task_id": "task-1", "agent_id": "agent-1", "project_id": "proj-1", "exec_mode": tc.execMode,
			})
			req := httptest.NewRequest("POST", "/api/v1/runs", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
			if msg := decodeErrorMessage(t, w); !strings.Contains(msg, unavailableExecModeMessage(tc.wantMode)) {
				t.Fatalf("error message %q does not explain the unavailable exec mode", msg)
			}
			if len(store.runs) != 0 {
				t.Fatalf("expected no run to be created, got %d", len(store.runs))
			}
		})
	}
}

func TestStartRun_MountStillStarts(t *testing.T) {
	store := newExecModeRunStore(nil)
	r := newTestRouterWithStore(store)

	body, _ := json.Marshal(map[string]string{"task_id": "task-1", "agent_id": "agent-1", "project_id": "proj-1"})
	req := httptest.NewRequest("POST", "/api/v1/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		ExecMode string `json:"exec_mode"`
	}
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ExecMode != "mount" {
		t.Fatalf("expected exec_mode mount, got %q", created.ExecMode)
	}
}

func TestSendAgenticMessage_UnavailableExecModeReturns400(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{
			ID: "proj-1", Name: "p", WorkspacePath: t.TempDir(),
			Config: map[string]string{"execution_mode": "hybrid"},
		}},
		convs: []conversation.Conversation{{ID: "conv-1", ProjectID: "proj-1"}},
	}
	r := newTestRouterWithStore(store)

	agentic := true
	body, _ := json.Marshal(conversation.SendMessageRequest{Content: "run the tests", Agentic: &agentic})
	req := httptest.NewRequest("POST", "/api/v1/conversations/conv-1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if msg := decodeErrorMessage(t, w); !strings.Contains(msg, unavailableExecModeMessage("hybrid")) {
		t.Fatalf("error message %q does not explain the unavailable exec mode", msg)
	}
	if len(store.messages) != 0 {
		t.Fatalf("expected the rejected message not to be stored, got %d", len(store.messages))
	}
}

func TestAIDiscoverProjectGoals_UnavailableExecModeReturns400(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{
			ID: "proj-1", Name: "p", WorkspacePath: t.TempDir(),
			Config: map[string]string{"execution_mode": "sandbox"},
		}},
	}
	r := newTestRouterWithStore(store)

	req := httptest.NewRequest("POST", "/api/v1/projects/proj-1/goals/ai-discover", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if msg := decodeErrorMessage(t, w); !strings.Contains(msg, unavailableExecModeMessage("sandbox")) {
		t.Fatalf("error message %q does not explain the unavailable exec mode", msg)
	}
}

func TestCreateBenchmarkRun_UnavailableExecModeReturns400(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	for _, mode := range []benchmark.ExecMode{benchmark.ExecModeSandbox, benchmark.ExecModeHybrid} {
		t.Run(string(mode), func(t *testing.T) {
			r := newTestRouter()

			body, _ := json.Marshal(benchmark.CreateRunRequest{
				Dataset:       "swe-bench",
				Model:         "gpt-4",
				Metrics:       []string{"llm_judge"},
				BenchmarkType: benchmark.TypeAgent,
				ExecMode:      mode,
			})
			req := httptest.NewRequest("POST", "/api/v1/benchmarks/runs", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
			if msg := decodeErrorMessage(t, w); !strings.Contains(msg, unavailableExecModeMessage(string(mode))) {
				t.Fatalf("error message %q does not explain the unavailable exec mode", msg)
			}
		})
	}
}

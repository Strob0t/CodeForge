package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/run"
)

func TestListProjectSessions_Empty(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("GET", "/api/v1/projects/proj-1/sessions", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result []run.Session
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty list, got %d", len(result))
	}
}

func TestGetSession_NotFound(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("GET", "/api/v1/sessions/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestResumeRun_Success(t *testing.T) {
	store := &mockStore{
		runs: []run.Run{{ID: "run-1", ProjectID: "proj-1", TaskID: "task-1", AgentID: "agent-1", Status: run.StatusCompleted}},
	}
	r := newTestRouterWithStore(store)
	req := httptest.NewRequest("POST", "/api/v1/runs/run-1/resume", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var sess run.Session
	if err := json.NewDecoder(w.Body).Decode(&sess); err != nil {
		t.Fatal(err)
	}
	if sess.ParentRunID != "run-1" {
		t.Fatalf("expected ParentRunID=run-1, got %s", sess.ParentRunID)
	}
}

func TestResumeRun_NotFound(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("POST", "/api/v1/runs/nonexistent/resume", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestForkRun_Success(t *testing.T) {
	store := &mockStore{
		runs: []run.Run{{ID: "run-1", ProjectID: "proj-1", TaskID: "task-1", AgentID: "agent-1", Status: run.StatusCompleted}},
	}
	r := newTestRouterWithStore(store)
	req := httptest.NewRequest("POST", "/api/v1/runs/run-1/fork", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRewindRun_Success(t *testing.T) {
	store := &mockStore{
		runs: []run.Run{{ID: "run-1", ProjectID: "proj-1", TaskID: "task-1", AgentID: "agent-1", Status: run.StatusCompleted}},
	}
	r := newTestRouterWithStore(store)
	req := httptest.NewRequest("POST", "/api/v1/runs/run-1/rewind", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

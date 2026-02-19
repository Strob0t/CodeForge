package a2a

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newTestRouter() *chi.Mux {
	h := NewHandler("http://localhost:8080")
	r := chi.NewRouter()
	h.MountRoutes(r)
	return r
}

func TestAgentCard(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var card AgentCard
	if err := json.NewDecoder(w.Body).Decode(&card); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if card.Name != "CodeForge" {
		t.Fatalf("expected name CodeForge, got %s", card.Name)
	}
	if len(card.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(card.Skills))
	}
}

func TestCreateAndGetTask(t *testing.T) {
	r := newTestRouter()

	// Create task
	body := `{"id":"test-1","skill":"code-task","input":{"prompt":"write hello world"}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a/tasks", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp TaskResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "queued" {
		t.Fatalf("expected queued, got %s", resp.Status)
	}

	// Get task
	req2 := httptest.NewRequest(http.MethodGet, "/a2a/tasks/test-1", http.NoBody)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}
}

func TestGetTaskNotFound(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/a2a/tasks/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCreateTaskInvalidBody(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/a2a/tasks", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateTaskMissingID(t *testing.T) {
	r := newTestRouter()
	body := `{"skill":"code-task"}`
	req := httptest.NewRequest(http.MethodPost, "/a2a/tasks", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

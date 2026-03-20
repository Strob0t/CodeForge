package http_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// --- Batch Validation Unit Tests ---

func TestValidateBatchIDs(t *testing.T) {
	tests := []struct {
		name    string
		ids     []string
		wantErr string
	}{
		{
			name:    "empty list",
			ids:     []string{},
			wantErr: "ids list is empty",
		},
		{
			name:    "nil list",
			ids:     nil,
			wantErr: "ids list is empty",
		},
		{
			name:    "single valid ID",
			ids:     []string{"abc-123"},
			wantErr: "",
		},
		{
			name:    "multiple valid IDs",
			ids:     []string{"a", "b", "c"},
			wantErr: "",
		},
		{
			name:    "empty ID in list",
			ids:     []string{"a", "", "c"},
			wantErr: "empty ID in list",
		},
		{
			name:    "too many IDs",
			ids:     make51IDs(),
			wantErr: "too many IDs: maximum is 50",
		},
		{
			name:    "exactly 50 IDs",
			ids:     make50IDs(),
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string][]string{"ids": tt.ids})
			req := httptest.NewRequest("POST", "/api/v1/projects/batch/delete", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			r := newTestRouter()
			r.ServeHTTP(w, req)

			if tt.wantErr == "" {
				// No validation error expected; might get other errors (e.g., not found) but not 400
				if w.Code == http.StatusBadRequest {
					t.Fatalf("expected no validation error, got 400: %s", w.Body.String())
				}
			} else {
				if w.Code != http.StatusBadRequest {
					t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
				}
				if !strings.Contains(w.Body.String(), tt.wantErr) {
					t.Fatalf("expected error %q, got %s", tt.wantErr, w.Body.String())
				}
			}
		})
	}
}

// --- Batch Delete Tests ---

func TestBatchDelete_EmptyIDs(t *testing.T) {
	r := newTestRouter()
	body, _ := json.Marshal(map[string][]string{"ids": {}})
	req := httptest.NewRequest("POST", "/api/v1/projects/batch/delete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ids list is empty") {
		t.Fatalf("expected 'ids list is empty' error, got %s", w.Body.String())
	}
}

func TestBatchDelete_TooManyIDs(t *testing.T) {
	r := newTestRouter()
	ids := make51IDs()
	body, _ := json.Marshal(map[string][]string{"ids": ids})
	req := httptest.NewRequest("POST", "/api/v1/projects/batch/delete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "too many IDs") {
		t.Fatalf("expected 'too many IDs' error, got %s", w.Body.String())
	}
}

func TestBatchDelete_ValidRequest(t *testing.T) {
	r := newTestRouter()

	// Create a project first
	projBody, _ := json.Marshal(project.CreateRequest{Name: "Batch Delete Target", Provider: "local"})
	req := httptest.NewRequest("POST", "/api/v1/projects", bytes.NewReader(projBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var p project.Project
	_ = json.NewDecoder(w.Body).Decode(&p)

	// Batch delete it
	body, _ := json.Marshal(map[string][]string{"ids": {p.ID}})
	req = httptest.NewRequest("POST", "/api/v1/projects/batch/delete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var results []struct {
		ID    string `json:"id"`
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(w.Body).Decode(&results); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].OK {
		t.Fatalf("expected OK=true, got error: %s", results[0].Error)
	}
	if results[0].ID != p.ID {
		t.Fatalf("expected ID=%s, got %s", p.ID, results[0].ID)
	}
}

func TestBatchDelete_MixedResults(t *testing.T) {
	r := newTestRouter()

	// Create a project
	projBody, _ := json.Marshal(project.CreateRequest{Name: "Keep Me", Provider: "local"})
	req := httptest.NewRequest("POST", "/api/v1/projects", bytes.NewReader(projBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var p project.Project
	_ = json.NewDecoder(w.Body).Decode(&p)

	// Batch delete: one real, one nonexistent
	body, _ := json.Marshal(map[string][]string{"ids": {p.ID, "nonexistent-id"}})
	req = httptest.NewRequest("POST", "/api/v1/projects/batch/delete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var results []struct {
		ID    string `json:"id"`
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(w.Body).Decode(&results); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Find results by ID (order not guaranteed due to concurrency)
	resultMap := map[string]bool{}
	for _, r := range results {
		resultMap[r.ID] = r.OK
	}
	if !resultMap[p.ID] {
		t.Fatalf("expected OK=true for %s", p.ID)
	}
	if resultMap["nonexistent-id"] {
		t.Fatal("expected OK=false for nonexistent-id")
	}
}

// --- Batch Pull Tests ---

func TestBatchPull_EmptyIDInList(t *testing.T) {
	r := newTestRouter()
	body, _ := json.Marshal(map[string][]string{"ids": {"valid-id", ""}})
	req := httptest.NewRequest("POST", "/api/v1/projects/batch/pull", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "empty ID in list") {
		t.Fatalf("expected 'empty ID in list' error, got %s", w.Body.String())
	}
}

func TestBatchPull_InvalidBody(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("POST", "/api/v1/projects/batch/pull", strings.NewReader("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- Batch Status Tests ---

func TestBatchStatus_ValidRequest(t *testing.T) {
	r := newTestRouter()

	// Create a project
	projBody, _ := json.Marshal(project.CreateRequest{Name: "Status Target", Provider: "local"})
	req := httptest.NewRequest("POST", "/api/v1/projects", bytes.NewReader(projBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var p project.Project
	_ = json.NewDecoder(w.Body).Decode(&p)

	// Batch status — the project has no workspace, so Status will fail,
	// but the response structure should be correct.
	body, _ := json.Marshal(map[string][]string{"ids": {p.ID}})
	req = httptest.NewRequest("POST", "/api/v1/projects/batch/status", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var results []struct {
		ID     string `json:"id"`
		OK     bool   `json:"ok"`
		Error  string `json:"error,omitempty"`
		Status any    `json:"status,omitempty"`
	}
	if err := json.NewDecoder(w.Body).Decode(&results); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != p.ID {
		t.Fatalf("expected ID=%s, got %s", p.ID, results[0].ID)
	}
	// Status may fail (no workspace) — that's OK, we just validate the structure
}

func TestBatchStatus_EmptyIDs(t *testing.T) {
	r := newTestRouter()
	body, _ := json.Marshal(map[string][]string{"ids": {}})
	req := httptest.NewRequest("POST", "/api/v1/projects/batch/status", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- Helpers ---

func make50IDs() []string {
	ids := make([]string, 50)
	for i := range ids {
		ids[i] = fmt.Sprintf("id-%d", i)
	}
	return ids
}

func make51IDs() []string {
	ids := make([]string, 51)
	for i := range ids {
		ids[i] = fmt.Sprintf("id-%d", i)
	}
	return ids
}

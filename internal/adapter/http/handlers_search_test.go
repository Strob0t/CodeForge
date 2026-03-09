package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

func TestGlobalSearch_EmptyQuery(t *testing.T) {
	r := newTestRouter()
	body, _ := json.Marshal(map[string]string{"query": ""})
	req := httptest.NewRequest("POST", "/api/v1/search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "query is required") {
		t.Fatalf("expected 'query is required' error, got %s", w.Body.String())
	}
}

func TestGlobalSearch_MissingQuery(t *testing.T) {
	r := newTestRouter()
	body, _ := json.Marshal(map[string]int{"limit": 10})
	req := httptest.NewRequest("POST", "/api/v1/search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "query is required") {
		t.Fatalf("expected 'query is required' error, got %s", w.Body.String())
	}
}

func TestGlobalSearch_InvalidBody(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("POST", "/api/v1/search", strings.NewReader("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGlobalSearch_NoProjects(t *testing.T) {
	// With no projects in the store, searching all projects returns an empty result.
	r := newTestRouter()
	body, _ := json.Marshal(map[string]string{"query": "hello"})
	req := httptest.NewRequest("POST", "/api/v1/search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Query   string                                   `json:"query"`
		Total   int                                      `json:"total"`
		Results []messagequeue.RetrievalSearchHitPayload `json:"results"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Query != "hello" {
		t.Fatalf("expected query 'hello', got %q", resp.Query)
	}
	if resp.Total != 0 {
		t.Fatalf("expected 0 total results, got %d", resp.Total)
	}
	if len(resp.Results) != 0 {
		t.Fatalf("expected empty results, got %d", len(resp.Results))
	}
}

func TestGlobalSearch_DefaultLimit(t *testing.T) {
	// Verify a request with no limit set does not error (defaults to 20 internally).
	r := newTestRouter()
	body, _ := json.Marshal(map[string]string{"query": "test"})
	req := httptest.NewRequest("POST", "/api/v1/search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGlobalSearch_MaxLimit(t *testing.T) {
	// Verify a request with limit > 100 does not error (capped at 100 internally).
	r := newTestRouter()
	body, _ := json.Marshal(map[string]interface{}{"query": "test", "limit": 500})
	req := httptest.NewRequest("POST", "/api/v1/search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGlobalSearch_NegativeLimit(t *testing.T) {
	// Verify a request with negative limit defaults gracefully.
	r := newTestRouter()
	body, _ := json.Marshal(map[string]interface{}{"query": "test", "limit": -5})
	req := httptest.NewRequest("POST", "/api/v1/search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGlobalSearch_ResponseStructure(t *testing.T) {
	// Validate the response JSON has the expected fields.
	r := newTestRouter()
	body, _ := json.Marshal(map[string]string{"query": "struct"})
	req := httptest.NewRequest("POST", "/api/v1/search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Decode into raw map to verify field names exist.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	for _, field := range []string{"query", "total", "results"} {
		if _, ok := raw[field]; !ok {
			t.Fatalf("expected field %q in response, got keys: %v", field, raw)
		}
	}
}

package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAvailableLLMModels_NoRegistry(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("GET", "/api/v1/llm/available", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRefreshLLMModels_NoRegistry(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("POST", "/api/v1/llm/refresh", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCopilotExchange_NotEnabled(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("POST", "/api/v1/copilot/exchange", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListLLMModels_BadGateway(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("GET", "/api/v1/llm/models", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDiscoverLLMModels_BadGateway(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("GET", "/api/v1/llm/discover", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAddLLMModel_BadGateway(t *testing.T) {
	r := newTestRouter()
	body, _ := json.Marshal(map[string]any{
		"model_name":     "gpt-4",
		"litellm_params": map[string]string{"model": "gpt-4"},
	})
	req := httptest.NewRequest("POST", "/api/v1/llm/models", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", w.Code, w.Body.String())
	}
}

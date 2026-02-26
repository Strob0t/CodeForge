package litellm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
)

func TestHealthDetailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"healthy_endpoints": []map[string]string{
				{"model": "gpt-4o", "api_base": "https://api.openai.com"},
				{"model": "groq/llama-3.3-70b", "api_base": "https://api.groq.com"},
			},
			"unhealthy_endpoints": []map[string]string{
				{"model": "ollama/llama3.2", "error": "ConnectionError"},
			},
			"healthy_count":   2,
			"unhealthy_count": 1,
		})
	}))
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")
	report, err := client.HealthDetailed(context.Background())
	if err != nil {
		t.Fatalf("HealthDetailed failed: %v", err)
	}

	if report.HealthyCount != 2 {
		t.Errorf("expected 2 healthy, got %d", report.HealthyCount)
	}
	if report.UnhealthyCount != 1 {
		t.Errorf("expected 1 unhealthy, got %d", report.UnhealthyCount)
	}
	if len(report.HealthyEndpoints) != 2 {
		t.Fatalf("expected 2 healthy endpoints, got %d", len(report.HealthyEndpoints))
	}
	if report.HealthyEndpoints[0].Model != "gpt-4o" {
		t.Errorf("expected gpt-4o, got %q", report.HealthyEndpoints[0].Model)
	}
	if len(report.UnhealthyEndpoints) != 1 {
		t.Fatalf("expected 1 unhealthy endpoint, got %d", len(report.UnhealthyEndpoints))
	}
	if report.UnhealthyEndpoints[0].Model != "ollama/llama3.2" {
		t.Errorf("expected ollama/llama3.2, got %q", report.UnhealthyEndpoints[0].Model)
	}
}

func TestHealthDetailedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"down"}`))
	}))
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")
	_, err := client.HealthDetailed(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDiscoverModelsWithHealth(t *testing.T) {
	// Serve both /model/info, /v1/models, and /health.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/model/info":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{
						"model_name": "gpt-4o",
						"model_id":   "model-gpt-4o",
						"model_info": map[string]any{
							"max_tokens":            128000.0,
							"output_cost_per_token": 1.5e-5,
						},
						"litellm_params": map[string]any{
							"model": "openai/gpt-4o",
						},
					},
					{
						"model_name": "ollama/llama3.2",
						"model_id":   "model-ollama",
						"model_info": map[string]any{},
						"litellm_params": map[string]any{
							"model": "ollama/llama3.2",
						},
					},
				},
			})

		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{
					{"id": "gpt-4o"},
					{"id": "ollama/llama3.2"},
				},
			})

		case "/health":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"healthy_endpoints": []map[string]string{
					{"model": "gpt-4o"},
				},
				"unhealthy_endpoints": []map[string]string{
					{"model": "ollama/llama3.2", "error": "ConnectionError: host unreachable"},
				},
			})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")
	models, err := client.DiscoverModels(context.Background())
	if err != nil {
		t.Fatalf("DiscoverModels failed: %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	// gpt-4o should be reachable.
	var gpt4o, ollama *litellm.DiscoveredModel
	for i := range models {
		switch models[i].ModelName {
		case "gpt-4o":
			gpt4o = &models[i]
		case "ollama/llama3.2":
			ollama = &models[i]
		}
	}

	if gpt4o == nil {
		t.Fatal("gpt-4o not found")
	}
	if gpt4o.Status != "reachable" {
		t.Errorf("expected gpt-4o reachable, got %q", gpt4o.Status)
	}
	if gpt4o.ErrorDetail != "" {
		t.Errorf("expected empty error_detail for gpt-4o, got %q", gpt4o.ErrorDetail)
	}
	if gpt4o.Provider != "openai" {
		t.Errorf("expected provider 'openai', got %q", gpt4o.Provider)
	}
	if gpt4o.MaxTokens != 128000 {
		t.Errorf("expected max_tokens 128000, got %d", gpt4o.MaxTokens)
	}

	// ollama should be unreachable with error detail.
	if ollama == nil {
		t.Fatal("ollama/llama3.2 not found")
	}
	if ollama.Status != "unreachable" {
		t.Errorf("expected ollama/llama3.2 unreachable, got %q", ollama.Status)
	}
	if ollama.ErrorDetail == "" {
		t.Error("expected non-empty error_detail for ollama/llama3.2")
	}
}

func TestDiscoverModelsHealthFallback(t *testing.T) {
	// When /health fails, all models should still be marked reachable (graceful degradation).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/model/info":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"model_name": "gpt-4o", "model_id": "m1", "model_info": map[string]any{}},
				},
			})

		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "gpt-4o"}},
			})

		case "/health":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"broken"}`))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")
	models, err := client.DiscoverModels(context.Background())
	if err != nil {
		t.Fatalf("DiscoverModels should not fail when /health fails: %v", err)
	}

	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	// When health check fails, models should default to reachable.
	if models[0].Status != "reachable" {
		t.Errorf("expected reachable when health check fails, got %q", models[0].Status)
	}
}

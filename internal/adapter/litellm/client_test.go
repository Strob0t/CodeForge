package litellm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
)

func TestListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/model/info" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		resp := map[string][]litellm.Model{
			"data": {
				{ModelName: "gpt-4o", Provider: "openai"},
				{ModelName: "claude-sonnet-4-20250514", Provider: "anthropic"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")
	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ModelName != "gpt-4o" {
		t.Fatalf("expected gpt-4o, got %q", models[0].ModelName)
	}
}

func TestAddModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/model/new" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			t.Fatalf("unexpected auth: %q", auth)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"Model added successfully"}`))
	}))
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")
	err := client.AddModel(context.Background(), litellm.AddModelRequest{
		ModelName:     "test-model",
		LiteLLMParams: map[string]string{"model": "openai/gpt-4o"},
	})
	if err != nil {
		t.Fatalf("AddModel failed: %v", err)
	}
}

func TestDeleteModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/model/delete" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")
	err := client.DeleteModel(context.Background(), "model-123")
	if err != nil {
		t.Fatalf("DeleteModel failed: %v", err)
	}
}

func TestHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy"}`))
	}))
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")
	healthy, err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("Health failed: %v", err)
	}
	if !healthy {
		t.Fatal("expected healthy")
	}
}

func TestHealthUnhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"unhealthy"}`))
	}))
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")
	healthy, _ := client.Health(context.Background())
	if healthy {
		t.Fatal("expected unhealthy")
	}
}

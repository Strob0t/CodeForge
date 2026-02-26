package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/service"
)

// mockBroadcaster captures BroadcastEvent calls for verification.
type mockBroadcaster struct {
	mu     sync.Mutex
	events []mockEvent
}

type mockEvent struct {
	eventType string
	payload   any
}

func (m *mockBroadcaster) BroadcastEvent(_ context.Context, eventType string, payload any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, mockEvent{eventType: eventType, payload: payload})
}

func (m *mockBroadcaster) lastEvent() (mockEvent, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.events) == 0 {
		return mockEvent{}, false
	}
	return m.events[len(m.events)-1], true
}

func (m *mockBroadcaster) eventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

// newTestLiteLLMServer creates a mock LiteLLM server that serves /model/info, /v1/models, and /health.
func newTestLiteLLMServer(healthy, unhealthy []string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/model/info":
			data := make([]map[string]any, 0)
			for _, name := range append(healthy, unhealthy...) {
				data = append(data, map[string]any{
					"model_name": name,
					"model_id":   "id-" + name,
					"model_info": map[string]any{
						"max_tokens":            128000.0,
						"output_cost_per_token": 1.5e-5,
					},
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": data})

		case "/v1/models":
			data := make([]map[string]string, 0)
			for _, name := range append(healthy, unhealthy...) {
				data = append(data, map[string]string{"id": name})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": data})

		case "/health":
			he := make([]map[string]string, 0)
			for _, name := range healthy {
				he = append(he, map[string]string{"model": name})
			}
			ue := make([]map[string]string, 0)
			for _, name := range unhealthy {
				ue = append(ue, map[string]string{"model": name, "error": "unreachable"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"healthy_endpoints":   he,
				"unhealthy_endpoints": ue,
			})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestModelRegistryRefresh(t *testing.T) {
	srv := newTestLiteLLMServer([]string{"gpt-4o", "claude-3-opus"}, []string{"ollama/llama3.2"})
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")
	hub := &mockBroadcaster{}
	registry := service.NewModelRegistry(client, hub, 0) // 0 = no polling

	err := registry.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	models := registry.AvailableModels()
	if len(models) != 3 {
		t.Fatalf("expected 3 models, got %d", len(models))
	}

	// Verify BestModel selects from reachable models only.
	best := registry.BestModel()
	if best == "" {
		t.Fatal("expected non-empty best model")
	}
	// ollama/llama3.2 is unreachable, so it should NOT be best.
	if best == "ollama/llama3.2" {
		t.Error("best model should not be an unreachable model")
	}

	// Verify IsHealthy.
	if !registry.IsHealthy("gpt-4o") {
		t.Error("expected gpt-4o to be healthy")
	}
	if registry.IsHealthy("ollama/llama3.2") {
		t.Error("expected ollama/llama3.2 to be unhealthy")
	}
	if registry.IsHealthy("nonexistent") {
		t.Error("expected nonexistent model to be unhealthy")
	}

	// Verify WS event was broadcast.
	if hub.eventCount() == 0 {
		t.Fatal("expected at least one WS event broadcast")
	}
	evt, ok := hub.lastEvent()
	if !ok {
		t.Fatal("no events")
	}
	if evt.eventType != ws.EventModelHealth {
		t.Errorf("expected event type %q, got %q", ws.EventModelHealth, evt.eventType)
	}
	healthEvt, ok := evt.payload.(ws.ModelHealthEvent)
	if !ok {
		t.Fatalf("expected ModelHealthEvent payload, got %T", evt.payload)
	}
	if healthEvt.BestModel != best {
		t.Errorf("WS event best_model %q != registry best %q", healthEvt.BestModel, best)
	}
	if healthEvt.HealthyCount != 2 {
		t.Errorf("expected 2 healthy in event, got %d", healthEvt.HealthyCount)
	}
	if healthEvt.UnhealthyCount != 1 {
		t.Errorf("expected 1 unhealthy in event, got %d", healthEvt.UnhealthyCount)
	}

	// Verify LastRefresh is recent.
	if time.Since(registry.LastRefresh()) > 5*time.Second {
		t.Error("expected LastRefresh to be recent")
	}
}

func TestModelRegistryStart(t *testing.T) {
	srv := newTestLiteLLMServer([]string{"gpt-4o"}, nil)
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")
	hub := &mockBroadcaster{}
	registry := service.NewModelRegistry(client, hub, 0) // 0 = no periodic polling

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	registry.Start(ctx)

	// After Start, first refresh should have run synchronously.
	if registry.BestModel() == "" {
		t.Error("expected best model after Start")
	}
	models := registry.AvailableModels()
	if len(models) != 1 {
		t.Fatalf("expected 1 model after Start, got %d", len(models))
	}
}

func TestModelRegistryNoHub(t *testing.T) {
	srv := newTestLiteLLMServer([]string{"gpt-4o"}, nil)
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")
	// nil hub should not panic.
	registry := service.NewModelRegistry(client, nil, 0)

	err := registry.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh with nil hub failed: %v", err)
	}

	if registry.BestModel() != "gpt-4o" {
		t.Errorf("expected gpt-4o, got %q", registry.BestModel())
	}
}

func TestModelRegistryEmptyModels(t *testing.T) {
	srv := newTestLiteLLMServer(nil, nil)
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")
	hub := &mockBroadcaster{}
	registry := service.NewModelRegistry(client, hub, 0)

	err := registry.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	if registry.BestModel() != "" {
		t.Errorf("expected empty best model, got %q", registry.BestModel())
	}
	if len(registry.AvailableModels()) != 0 {
		t.Errorf("expected 0 models, got %d", len(registry.AvailableModels()))
	}
}

func TestModelRegistryAllUnhealthy(t *testing.T) {
	srv := newTestLiteLLMServer(nil, []string{"gpt-4o", "claude-3-opus"})
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")
	hub := &mockBroadcaster{}
	registry := service.NewModelRegistry(client, hub, 0)

	err := registry.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	// All models are unhealthy, so BestModel should be empty.
	if registry.BestModel() != "" {
		t.Errorf("expected empty best model when all unhealthy, got %q", registry.BestModel())
	}
	// But models should still be in the cache.
	if len(registry.AvailableModels()) != 2 {
		t.Errorf("expected 2 models in cache, got %d", len(registry.AvailableModels()))
	}
}

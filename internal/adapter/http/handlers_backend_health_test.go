package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

func TestCheckBackendHealth_NilService(t *testing.T) {
	// BackendHealth is nil in the default test router — should return 503.
	r := newTestRouter()
	req := httptest.NewRequest("GET", "/api/v1/backends/health", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "not initialized") {
		t.Fatalf("expected 'not initialized' error, got %s", w.Body.String())
	}
}

// mockHealthQueue captures Publish calls and immediately delivers a health result.
type mockHealthQueue struct {
	svc *service.BackendHealthService
}

func (m *mockHealthQueue) Publish(_ context.Context, subject string, data []byte) error {
	if subject == messagequeue.SubjectBackendHealthRequest {
		// Extract request_id from payload and deliver a mock result.
		var req struct {
			RequestID string `json:"request_id"`
		}
		if err := json.Unmarshal(data, &req); err != nil {
			return err
		}
		result, _ := json.Marshal(map[string]any{
			"request_id": req.RequestID,
			"backends": []map[string]any{
				{
					"name":         "aider",
					"display_name": "Aider",
					"available":    true,
					"capabilities": []string{"code_generation"},
				},
			},
		})
		// Deliver result synchronously (mimics Python worker response).
		return m.svc.HandleHealthResult(context.Background(), result)
	}
	return nil
}
func (m *mockHealthQueue) PublishWithDedup(_ context.Context, _ string, _ []byte, _ string) error {
	return nil
}
func (m *mockHealthQueue) Subscribe(_ context.Context, _ string, _ messagequeue.Handler) (func(), error) {
	return func() {}, nil
}
func (m *mockHealthQueue) Drain() error      { return nil }
func (m *mockHealthQueue) Close() error      { return nil }
func (m *mockHealthQueue) IsConnected() bool { return true }

func TestCheckBackendHealth_Success(t *testing.T) {
	// Create a BackendHealthService with a mock queue that delivers results.
	mq := &mockHealthQueue{}
	svc := service.NewBackendHealthService(mq)
	mq.svc = svc

	// Build a minimal router with the BackendHealth handler wired up.
	r := newTestRouterWithBackendHealth(svc)
	req := httptest.NewRequest("GET", "/api/v1/backends/health", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Backends []service.BackendHealthEntry `json:"backends"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(resp.Backends))
	}
	if resp.Backends[0].Name != "aider" {
		t.Fatalf("expected backend name 'aider', got %q", resp.Backends[0].Name)
	}
	if !resp.Backends[0].Available {
		t.Fatal("expected backend to be available")
	}
}

package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/routing"
)

func TestHandleListRoutingStatsEmpty(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("GET", "/api/v1/routing/stats", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListRoutingStatsWithData(t *testing.T) {
	store := &mockStore{}
	store.routingStats = []routing.ModelPerformanceStats{
		{ModelName: "openai/gpt-4o", TaskType: routing.TaskCode, ComplexityTier: routing.TierMedium, TrialCount: 10, AvgReward: 0.85},
		{ModelName: "anthropic/claude-sonnet-4", TaskType: routing.TaskReview, ComplexityTier: routing.TierComplex, TrialCount: 5, AvgReward: 0.9},
	}
	r := newTestRouterWithStore(store)

	req := httptest.NewRequest("GET", "/api/v1/routing/stats", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var stats []routing.ModelPerformanceStats
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("expected 2 stats, got %d", len(stats))
	}
}

func TestHandleListRoutingStatsFilterTaskType(t *testing.T) {
	store := &mockStore{}
	store.routingStats = []routing.ModelPerformanceStats{
		{ModelName: "openai/gpt-4o", TaskType: routing.TaskCode, ComplexityTier: routing.TierMedium},
		{ModelName: "anthropic/claude-sonnet-4", TaskType: routing.TaskReview, ComplexityTier: routing.TierComplex},
	}
	r := newTestRouterWithStore(store)

	req := httptest.NewRequest("GET", "/api/v1/routing/stats?task_type=code", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var stats []routing.ModelPerformanceStats
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 stat, got %d", len(stats))
	}
}

func TestHandleRefreshRoutingStats(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("POST", "/api/v1/routing/stats/refresh", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListRoutingOutcomesEmpty(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("GET", "/api/v1/routing/outcomes", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListRoutingOutcomesWithData(t *testing.T) {
	store := &mockStore{}
	store.routingOutcomes = []routing.RoutingOutcome{
		{ModelName: "openai/gpt-4o", TaskType: routing.TaskCode, Success: true},
		{ModelName: "anthropic/claude-sonnet-4", TaskType: routing.TaskReview, Success: false},
	}
	r := newTestRouterWithStore(store)

	req := httptest.NewRequest("GET", "/api/v1/routing/outcomes?limit=10", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var outcomes []routing.RoutingOutcome
	if err := json.NewDecoder(w.Body).Decode(&outcomes); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(outcomes) != 2 {
		t.Fatalf("expected 2 outcomes, got %d", len(outcomes))
	}
}

func TestHandleListRoutingOutcomesLimitParam(t *testing.T) {
	store := &mockStore{}
	store.routingOutcomes = []routing.RoutingOutcome{
		{ModelName: "a"},
		{ModelName: "b"},
		{ModelName: "c"},
	}
	r := newTestRouterWithStore(store)

	req := httptest.NewRequest("GET", "/api/v1/routing/outcomes?limit=2", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var outcomes []routing.RoutingOutcome
	if err := json.NewDecoder(w.Body).Decode(&outcomes); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(outcomes) != 2 {
		t.Fatalf("expected 2 outcomes (limit=2), got %d", len(outcomes))
	}
}

func TestHandleListRoutingOutcomesDefaultLimit(t *testing.T) {
	store := &mockStore{}
	for i := 0; i < 60; i++ {
		store.routingOutcomes = append(store.routingOutcomes, routing.RoutingOutcome{
			ModelName: "openai/gpt-4o",
		})
	}
	r := newTestRouterWithStore(store)

	req := httptest.NewRequest("GET", "/api/v1/routing/outcomes", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var outcomes []routing.RoutingOutcome
	if err := json.NewDecoder(w.Body).Decode(&outcomes); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(outcomes) != 50 {
		t.Fatalf("expected 50 outcomes (default limit), got %d", len(outcomes))
	}
}

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/routing"
)

// routingMockStore is a focused mock for RoutingService tests.
type routingMockStore struct {
	mockStore // embed the full mockStore for interface satisfaction

	outcomes        []routing.RoutingOutcome
	stats           []routing.ModelPerformanceStats
	createErr       error
	listStatsErr    error
	upsertErr       error
	aggregateErr    error
	listOutcomesErr error
}

func (m *routingMockStore) CreateRoutingOutcome(_ context.Context, o *routing.RoutingOutcome) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.outcomes = append(m.outcomes, *o)
	return nil
}

func (m *routingMockStore) ListRoutingStats(_ context.Context, taskType, tier string) ([]routing.ModelPerformanceStats, error) {
	if m.listStatsErr != nil {
		return nil, m.listStatsErr
	}
	if taskType == "" && tier == "" {
		return m.stats, nil
	}
	var filtered []routing.ModelPerformanceStats
	for i := range m.stats {
		if (taskType == "" || string(m.stats[i].TaskType) == taskType) &&
			(tier == "" || string(m.stats[i].ComplexityTier) == tier) {
			filtered = append(filtered, m.stats[i])
		}
	}
	return filtered, nil
}

func (m *routingMockStore) UpsertRoutingStats(_ context.Context, st *routing.ModelPerformanceStats) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	for i := range m.stats {
		if m.stats[i].ModelName == st.ModelName && m.stats[i].TaskType == st.TaskType && m.stats[i].ComplexityTier == st.ComplexityTier {
			m.stats[i] = *st
			return nil
		}
	}
	m.stats = append(m.stats, *st)
	return nil
}

func (m *routingMockStore) AggregateRoutingOutcomes(_ context.Context) error {
	return m.aggregateErr
}

func (m *routingMockStore) ListRoutingOutcomes(_ context.Context, limit int) ([]routing.RoutingOutcome, error) {
	if m.listOutcomesErr != nil {
		return nil, m.listOutcomesErr
	}
	if limit <= 0 || limit > len(m.outcomes) {
		return m.outcomes, nil
	}
	return m.outcomes[:limit], nil
}

// --- RecordOutcome tests ---

func TestRoutingServiceRecordOutcome(t *testing.T) {
	store := &routingMockStore{}
	svc := NewRoutingService(store)

	o := &routing.RoutingOutcome{
		ModelName:      "openai/gpt-4o",
		TaskType:       routing.TaskCode,
		ComplexityTier: routing.TierMedium,
		Success:        true,
		QualityScore:   0.85,
	}
	err := svc.RecordOutcome(context.Background(), o)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(store.outcomes))
	}
	if store.outcomes[0].ModelName != "openai/gpt-4o" {
		t.Errorf("expected model openai/gpt-4o, got %s", store.outcomes[0].ModelName)
	}
}

func TestRoutingServiceRecordOutcomeEmptyModel(t *testing.T) {
	store := &routingMockStore{}
	svc := NewRoutingService(store)

	o := &routing.RoutingOutcome{
		TaskType:       routing.TaskCode,
		ComplexityTier: routing.TierSimple,
	}
	err := svc.RecordOutcome(context.Background(), o)
	if err == nil {
		t.Fatal("expected error for empty model_name")
	}
	if len(store.outcomes) != 0 {
		t.Error("store should not have been called")
	}
}

func TestRoutingServiceRecordOutcomeInvalidTaskType(t *testing.T) {
	store := &routingMockStore{}
	svc := NewRoutingService(store)

	o := &routing.RoutingOutcome{
		ModelName:      "openai/gpt-4o",
		TaskType:       routing.TaskType("invalid"),
		ComplexityTier: routing.TierSimple,
	}
	err := svc.RecordOutcome(context.Background(), o)
	if err == nil {
		t.Fatal("expected error for invalid task_type")
	}
}

func TestRoutingServiceRecordOutcomeInvalidTier(t *testing.T) {
	store := &routingMockStore{}
	svc := NewRoutingService(store)

	o := &routing.RoutingOutcome{
		ModelName:      "openai/gpt-4o",
		TaskType:       routing.TaskCode,
		ComplexityTier: routing.ComplexityTier("bogus"),
	}
	err := svc.RecordOutcome(context.Background(), o)
	if err == nil {
		t.Fatal("expected error for invalid complexity_tier")
	}
}

func TestRoutingServiceRecordOutcomeStoreError(t *testing.T) {
	store := &routingMockStore{createErr: errors.New("db down")}
	svc := NewRoutingService(store)

	o := &routing.RoutingOutcome{
		ModelName:      "openai/gpt-4o",
		TaskType:       routing.TaskCode,
		ComplexityTier: routing.TierSimple,
	}
	err := svc.RecordOutcome(context.Background(), o)
	if err == nil {
		t.Fatal("expected error from store")
	}
}

// --- GetStats tests ---

func TestRoutingServiceGetStats(t *testing.T) {
	store := &routingMockStore{
		stats: []routing.ModelPerformanceStats{
			{ModelName: "openai/gpt-4o", TaskType: routing.TaskCode, ComplexityTier: routing.TierMedium, TrialCount: 10, AvgReward: 0.8},
			{ModelName: "anthropic/claude-sonnet-4", TaskType: routing.TaskReview, ComplexityTier: routing.TierComplex, TrialCount: 5, AvgReward: 0.9},
		},
	}
	svc := NewRoutingService(store)

	stats, err := svc.GetStats(context.Background(), "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("expected 2 stats, got %d", len(stats))
	}
}

func TestRoutingServiceGetStatsFiltered(t *testing.T) {
	store := &routingMockStore{
		stats: []routing.ModelPerformanceStats{
			{ModelName: "openai/gpt-4o", TaskType: routing.TaskCode, ComplexityTier: routing.TierMedium},
			{ModelName: "anthropic/claude-sonnet-4", TaskType: routing.TaskReview, ComplexityTier: routing.TierComplex},
		},
	}
	svc := NewRoutingService(store)

	stats, err := svc.GetStats(context.Background(), "code", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 stat, got %d", len(stats))
	}
	if stats[0].ModelName != "openai/gpt-4o" {
		t.Errorf("expected openai/gpt-4o, got %s", stats[0].ModelName)
	}
}

func TestRoutingServiceGetStatsError(t *testing.T) {
	store := &routingMockStore{listStatsErr: errors.New("db error")}
	svc := NewRoutingService(store)

	_, err := svc.GetStats(context.Background(), "", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRoutingServiceGetStatsEmpty(t *testing.T) {
	store := &routingMockStore{}
	svc := NewRoutingService(store)

	stats, err := svc.GetStats(context.Background(), "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats != nil {
		t.Errorf("expected nil stats, got %v", stats)
	}
}

// --- RefreshStats tests ---

func TestRoutingServiceRefreshStats(t *testing.T) {
	store := &routingMockStore{}
	svc := NewRoutingService(store)

	err := svc.RefreshStats(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRoutingServiceRefreshStatsError(t *testing.T) {
	store := &routingMockStore{aggregateErr: errors.New("aggregate failed")}
	svc := NewRoutingService(store)

	err := svc.RefreshStats(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- ListOutcomes tests ---

func TestRoutingServiceListOutcomes(t *testing.T) {
	store := &routingMockStore{
		outcomes: []routing.RoutingOutcome{
			{ModelName: "openai/gpt-4o", TaskType: routing.TaskCode},
			{ModelName: "anthropic/claude-sonnet-4", TaskType: routing.TaskReview},
		},
	}
	svc := NewRoutingService(store)

	outcomes, err := svc.ListOutcomes(context.Background(), 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(outcomes) != 2 {
		t.Fatalf("expected 2 outcomes, got %d", len(outcomes))
	}
}

func TestRoutingServiceListOutcomesLimit(t *testing.T) {
	store := &routingMockStore{
		outcomes: []routing.RoutingOutcome{
			{ModelName: "a"},
			{ModelName: "b"},
			{ModelName: "c"},
		},
	}
	svc := NewRoutingService(store)

	outcomes, err := svc.ListOutcomes(context.Background(), 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(outcomes) != 2 {
		t.Fatalf("expected 2 outcomes, got %d", len(outcomes))
	}
}

func TestRoutingServiceListOutcomesError(t *testing.T) {
	store := &routingMockStore{listOutcomesErr: errors.New("db error")}
	svc := NewRoutingService(store)

	_, err := svc.ListOutcomes(context.Background(), 10)
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- UpsertStats tests ---

func TestRoutingServiceUpsertStats(t *testing.T) {
	store := &routingMockStore{}
	svc := NewRoutingService(store)

	st := &routing.ModelPerformanceStats{
		ModelName:      "openai/gpt-4o",
		TaskType:       routing.TaskCode,
		ComplexityTier: routing.TierMedium,
		TrialCount:     10,
		AvgReward:      0.85,
	}
	err := svc.UpsertStats(context.Background(), st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.stats) != 1 {
		t.Fatalf("expected 1 stat, got %d", len(store.stats))
	}
}

func TestRoutingServiceUpsertStatsEmptyModel(t *testing.T) {
	store := &routingMockStore{}
	svc := NewRoutingService(store)

	st := &routing.ModelPerformanceStats{
		TaskType:       routing.TaskCode,
		ComplexityTier: routing.TierMedium,
	}
	err := svc.UpsertStats(context.Background(), st)
	if err == nil {
		t.Fatal("expected error for empty model_name")
	}
}

func TestRoutingServiceUpsertStatsError(t *testing.T) {
	store := &routingMockStore{upsertErr: errors.New("db error")}
	svc := NewRoutingService(store)

	st := &routing.ModelPerformanceStats{
		ModelName: "openai/gpt-4o",
	}
	err := svc.UpsertStats(context.Background(), st)
	if err == nil {
		t.Fatal("expected error from store")
	}
}

func TestRoutingServiceUpsertStatsUpdate(t *testing.T) {
	store := &routingMockStore{
		stats: []routing.ModelPerformanceStats{
			{ModelName: "openai/gpt-4o", TaskType: routing.TaskCode, ComplexityTier: routing.TierMedium, TrialCount: 5, AvgReward: 0.7},
		},
	}
	svc := NewRoutingService(store)

	st := &routing.ModelPerformanceStats{
		ModelName:      "openai/gpt-4o",
		TaskType:       routing.TaskCode,
		ComplexityTier: routing.TierMedium,
		TrialCount:     10,
		AvgReward:      0.85,
	}
	err := svc.UpsertStats(context.Background(), st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.stats) != 1 {
		t.Fatalf("expected 1 stat (updated), got %d", len(store.stats))
	}
	if store.stats[0].TrialCount != 10 {
		t.Errorf("expected trial_count=10, got %d", store.stats[0].TrialCount)
	}
}

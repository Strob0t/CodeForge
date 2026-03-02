package service_test

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/cost"
	"github.com/Strob0t/CodeForge/internal/service"
)

// costMockStore overrides cost-related methods on the base runtimeMockStore.
type costMockStore struct {
	runtimeMockStore
	globalSummary  []cost.ProjectSummary
	projectSummary *cost.Summary
	modelSummary   []cost.ModelSummary
}

func (m *costMockStore) CostSummaryGlobal(_ context.Context) ([]cost.ProjectSummary, error) {
	return m.globalSummary, nil
}

func (m *costMockStore) CostSummaryByProject(_ context.Context, _ string) (*cost.Summary, error) {
	if m.projectSummary == nil {
		return &cost.Summary{}, nil
	}
	return m.projectSummary, nil
}

func (m *costMockStore) CostByModel(_ context.Context, _ string) ([]cost.ModelSummary, error) {
	return m.modelSummary, nil
}

func TestCostService_GlobalSummary(t *testing.T) {
	store := &costMockStore{
		globalSummary: []cost.ProjectSummary{
			{ProjectID: "p1", ProjectName: "Alpha", Summary: cost.Summary{TotalCostUSD: 1.50, TotalTokensIn: 1000, TotalTokensOut: 500}},
			{ProjectID: "p2", ProjectName: "Beta", Summary: cost.Summary{TotalCostUSD: 3.25, TotalTokensIn: 2000, TotalTokensOut: 1000}},
		},
	}
	svc := service.NewCostService(store)
	ctx := context.Background()

	results, err := svc.GlobalSummary(ctx)
	if err != nil {
		t.Fatalf("GlobalSummary: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(results))
	}
	if results[0].ProjectID != "p1" {
		t.Errorf("expected project p1, got %s", results[0].ProjectID)
	}
	if results[1].TotalCostUSD != 3.25 {
		t.Errorf("expected cost 3.25, got %f", results[1].TotalCostUSD)
	}
}

func TestCostService_ProjectSummary(t *testing.T) {
	store := &costMockStore{
		projectSummary: &cost.Summary{
			TotalCostUSD:   5.75,
			TotalTokensIn:  3000,
			TotalTokensOut: 1500,
			RunCount:       10,
		},
	}
	svc := service.NewCostService(store)
	ctx := context.Background()

	summary, err := svc.ProjectSummary(ctx, "proj-1")
	if err != nil {
		t.Fatalf("ProjectSummary: %v", err)
	}
	if summary.TotalCostUSD != 5.75 {
		t.Errorf("expected cost 5.75, got %f", summary.TotalCostUSD)
	}
	if summary.RunCount != 10 {
		t.Errorf("expected 10 runs, got %d", summary.RunCount)
	}
}

func TestCostService_ByModel(t *testing.T) {
	store := &costMockStore{
		modelSummary: []cost.ModelSummary{
			{Model: "gpt-4o", Summary: cost.Summary{TotalCostUSD: 2.00, RunCount: 5}},
			{Model: "claude-3-5-sonnet", Summary: cost.Summary{TotalCostUSD: 3.50, RunCount: 8}},
		},
	}
	svc := service.NewCostService(store)
	ctx := context.Background()

	models, err := svc.ByModel(ctx, "proj-1")
	if err != nil {
		t.Fatalf("ByModel: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].Model != "gpt-4o" {
		t.Errorf("expected gpt-4o, got %s", models[0].Model)
	}
	if models[1].RunCount != 8 {
		t.Errorf("expected 8 calls, got %d", models[1].RunCount)
	}
}

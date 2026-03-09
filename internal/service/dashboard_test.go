package service

import (
	"context"
	"testing"
)

func TestDashboardService_Stats(t *testing.T) {
	store := &mockStore{}
	svc := NewDashboardService(store)
	ctx := context.Background()

	stats, err := svc.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats == nil {
		t.Fatal("expected non-nil stats")
	}
}

func TestDashboardService_ProjectHealth(t *testing.T) {
	store := &mockStore{}
	svc := NewDashboardService(store)
	ctx := context.Background()

	health, err := svc.ProjectHealth(ctx, "proj-1")
	if err != nil {
		t.Fatalf("ProjectHealth: %v", err)
	}
	if health == nil {
		t.Fatal("expected non-nil health")
	}
	if health.Level == "" {
		t.Error("expected non-empty level")
	}
}

func TestDashboardService_RunOutcomes(t *testing.T) {
	store := &mockStore{}
	svc := NewDashboardService(store)
	ctx := context.Background()

	outcomes, err := svc.RunOutcomes(ctx, 7)
	if err != nil {
		t.Fatalf("RunOutcomes: %v", err)
	}
	if outcomes == nil {
		t.Fatal("expected non-nil outcomes")
	}
}

func TestDashboardService_AgentPerformance(t *testing.T) {
	store := &mockStore{}
	svc := NewDashboardService(store)
	ctx := context.Background()

	agents, err := svc.AgentPerformance(ctx)
	if err != nil {
		t.Fatalf("AgentPerformance: %v", err)
	}
	if agents == nil {
		t.Fatal("expected non-nil agents")
	}
}

func TestDashboardService_ModelUsage(t *testing.T) {
	store := &mockStore{}
	svc := NewDashboardService(store)
	ctx := context.Background()

	models, err := svc.ModelUsage(ctx)
	if err != nil {
		t.Fatalf("ModelUsage: %v", err)
	}
	if models == nil {
		t.Fatal("expected non-nil models")
	}
}

func TestDashboardService_CostByProject(t *testing.T) {
	store := &mockStore{}
	svc := NewDashboardService(store)
	ctx := context.Background()

	costs, err := svc.CostByProject(ctx)
	if err != nil {
		t.Fatalf("CostByProject: %v", err)
	}
	if costs == nil {
		t.Fatal("expected non-nil costs")
	}
}

func TestDashboardService_CostTrend(t *testing.T) {
	store := &mockStore{}
	svc := NewDashboardService(store)
	ctx := context.Background()

	trend, err := svc.CostTrend(ctx, 30)
	if err != nil {
		t.Fatalf("CostTrend: %v", err)
	}
	if trend == nil {
		t.Fatal("expected non-nil trend")
	}
}

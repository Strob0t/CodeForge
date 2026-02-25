package service_test

import (
	"context"
	"testing"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/service"
)

func TestSharedContextService_InitForTeam(t *testing.T) {
	store := &runtimeMockStore{}
	svc := service.NewSharedContextService(store, nil, nil)

	ctx := context.Background()
	sc, err := svc.InitForTeam(ctx, "team-1", "proj-1")
	if err != nil {
		t.Fatalf("InitForTeam failed: %v", err)
	}
	if sc.ID == "" {
		t.Error("expected non-empty shared context ID")
	}
	if sc.TeamID != "team-1" {
		t.Errorf("expected team_id 'team-1', got %q", sc.TeamID)
	}
	if sc.Version != 1 {
		t.Errorf("expected version 1, got %d", sc.Version)
	}
}

func TestSharedContextService_AddItem(t *testing.T) {
	store := &runtimeMockStore{
		sharedContexts: []cfcontext.SharedContext{
			{ID: "sc-1", TeamID: "team-1", ProjectID: "proj-1", Version: 1},
		},
	}
	svc := service.NewSharedContextService(store, nil, nil)

	ctx := context.Background()
	item, err := svc.AddItem(ctx, cfcontext.AddSharedItemRequest{
		TeamID: "team-1",
		Key:    "step-output",
		Value:  "completed task successfully",
		Author: "550e8400-e29b-41d4-a716-446655440000",
	})
	if err != nil {
		t.Fatalf("AddItem failed: %v", err)
	}
	if item.Key != "step-output" {
		t.Errorf("expected key 'step-output', got %q", item.Key)
	}
	if item.Tokens == 0 {
		t.Error("expected non-zero token count")
	}
}

func TestSharedContextService_Get(t *testing.T) {
	store := &runtimeMockStore{
		sharedContexts: []cfcontext.SharedContext{
			{
				ID: "sc-1", TeamID: "team-1", ProjectID: "proj-1", Version: 2,
				Items: []cfcontext.SharedContextItem{
					{ID: "sci-1", SharedID: "sc-1", Key: "data", Value: "hello"},
				},
			},
		},
	}
	svc := service.NewSharedContextService(store, nil, nil)

	ctx := context.Background()
	sc, err := svc.Get(ctx, "team-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if len(sc.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(sc.Items))
	}
	if sc.Items[0].Key != "data" {
		t.Errorf("expected key 'data', got %q", sc.Items[0].Key)
	}
}

func TestSharedContextService_InitForTeam_InvalidInput(t *testing.T) {
	store := &runtimeMockStore{}
	svc := service.NewSharedContextService(store, nil, nil)

	ctx := context.Background()
	_, err := svc.InitForTeam(ctx, "", "proj-1")
	if err == nil {
		t.Error("expected error for empty team_id")
	}
}

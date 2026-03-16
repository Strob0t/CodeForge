package service

import (
	"context"
	"testing"
	"time"
)

type mockReviewTriggerStore struct {
	recentExists bool
	createdIDs   []string
}

func (m *mockReviewTriggerStore) FindRecentReviewTrigger(_ context.Context, _, _ string, _ time.Duration) (bool, error) {
	return m.recentExists, nil
}

func (m *mockReviewTriggerStore) CreateReviewTrigger(_ context.Context, _, _, _ string) (string, error) {
	id := "trigger-1"
	m.createdIDs = append(m.createdIDs, id)
	return id, nil
}

func TestReviewTriggerService_DedupSkipsRecentSHA(t *testing.T) {
	store := &mockReviewTriggerStore{recentExists: true}
	svc := NewReviewTriggerService(store, nil, 30*time.Minute)

	triggered, err := svc.TriggerReview(context.Background(), "proj-1", "abc123", "pipeline-completion")
	if err != nil {
		t.Fatal(err)
	}
	if triggered {
		t.Error("expected dedup to skip, but review was triggered")
	}
}

func TestReviewTriggerService_ManualBypassesDedup(t *testing.T) {
	store := &mockReviewTriggerStore{recentExists: true}
	svc := NewReviewTriggerService(store, nil, 30*time.Minute)

	triggered, err := svc.TriggerReview(context.Background(), "proj-1", "abc123", "manual")
	if err != nil {
		t.Fatal(err)
	}
	if !triggered {
		t.Error("manual trigger should bypass dedup")
	}
}

func TestReviewTriggerService_NewSHATriggersReview(t *testing.T) {
	store := &mockReviewTriggerStore{recentExists: false}
	svc := NewReviewTriggerService(store, nil, 30*time.Minute)

	triggered, err := svc.TriggerReview(context.Background(), "proj-1", "newsha", "branch-merge")
	if err != nil {
		t.Fatal(err)
	}
	if !triggered {
		t.Error("new SHA should trigger review")
	}
	if len(store.createdIDs) != 1 {
		t.Errorf("expected 1 trigger created, got %d", len(store.createdIDs))
	}
}

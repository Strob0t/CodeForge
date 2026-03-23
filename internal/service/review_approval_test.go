package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

func TestReviewApprovalService_PublishApprovalRequired(t *testing.T) {
	queue := &runtimeMockQueue{}
	hub := &runtimeMockBroadcaster{}
	svc := service.NewReviewApprovalService(queue, hub)

	stats := service.DiffStats{
		FilesChanged: 5,
		LinesAdded:   200,
		LinesRemoved: 50,
		CrossLayer:   true,
		Structural:   false,
	}

	svc.PublishApprovalRequired(context.Background(), "run-1", "proj-1", "tenant-1", service.ImpactHigh, stats)

	msg, ok := queue.lastMessage(messagequeue.SubjectReviewApprovalRequired)
	if !ok {
		t.Fatal("expected message published to review.approval.required")
	}

	var payload messagequeue.ReviewApprovalRequiredPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if payload.RunID != "run-1" {
		t.Errorf("RunID = %q, want %q", payload.RunID, "run-1")
	}
	if payload.ProjectID != "proj-1" {
		t.Errorf("ProjectID = %q, want %q", payload.ProjectID, "proj-1")
	}
	if payload.TenantID != "tenant-1" {
		t.Errorf("TenantID = %q, want %q", payload.TenantID, "tenant-1")
	}
	if payload.ImpactLevel != "high" {
		t.Errorf("ImpactLevel = %q, want %q", payload.ImpactLevel, "high")
	}
	if payload.DiffStats.FilesChanged != 5 {
		t.Errorf("FilesChanged = %d, want %d", payload.DiffStats.FilesChanged, 5)
	}
	if !payload.DiffStats.CrossLayer {
		t.Error("CrossLayer should be true")
	}
}

func TestReviewApprovalService_PublishApprovalRequired_NilQueue(t *testing.T) {
	hub := &runtimeMockBroadcaster{}
	svc := service.NewReviewApprovalService(nil, hub)

	// Should not panic with nil queue.
	svc.PublishApprovalRequired(context.Background(), "run-1", "proj-1", "t-1", service.ImpactHigh, service.DiffStats{})
}

func TestReviewApprovalService_HandleApprovalRequired(t *testing.T) {
	queue := &runtimeMockQueue{}
	hub := &runtimeMockBroadcaster{}
	svc := service.NewReviewApprovalService(queue, hub)

	payload := messagequeue.ReviewApprovalRequiredPayload{
		RunID:       "run-1",
		ProjectID:   "proj-1",
		TenantID:    "tenant-1",
		ImpactLevel: "high",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if err := svc.HandleApprovalRequired(context.Background(), "", data); err != nil {
		t.Fatalf("HandleApprovalRequired: %v", err)
	}

	hub.mu.Lock()
	eventCount := len(hub.events)
	hub.mu.Unlock()

	if eventCount != 1 {
		t.Fatalf("expected 1 broadcast event, got %d", eventCount)
	}
}

func TestReviewApprovalService_HandleApprovalRequired_InvalidJSON(t *testing.T) {
	queue := &runtimeMockQueue{}
	hub := &runtimeMockBroadcaster{}
	svc := service.NewReviewApprovalService(queue, hub)

	err := svc.HandleApprovalRequired(context.Background(), "", []byte("bad json"))
	if err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}

func TestReviewApprovalService_StartSubscriber_NilQueue(t *testing.T) {
	hub := &runtimeMockBroadcaster{}
	svc := service.NewReviewApprovalService(nil, hub)

	cancel, err := svc.StartSubscriber(context.Background())
	if err != nil {
		t.Fatalf("StartSubscriber: %v", err)
	}
	cancel() // should be a no-op
}

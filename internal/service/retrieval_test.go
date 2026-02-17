package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// captureQueue records the last published subject and data.
type captureQueue struct {
	subject string
	data    []byte
}

func (q *captureQueue) Publish(_ context.Context, subject string, data []byte) error {
	q.subject = subject
	q.data = data
	return nil
}
func (q *captureQueue) Subscribe(_ context.Context, _ string, _ messagequeue.Handler) (func(), error) {
	return func() {}, nil
}
func (q *captureQueue) Drain() error      { return nil }
func (q *captureQueue) Close() error      { return nil }
func (q *captureQueue) IsConnected() bool { return true }

func TestRetrievalService_RequestIndex(t *testing.T) {
	store := &runtimeMockStore{
		projects: []project.Project{
			{ID: "proj-1", Name: "test", WorkspacePath: "/tmp/test"},
		},
	}
	q := &captureQueue{}
	bc := &runtimeMockBroadcaster{}
	orchCfg := &config.Orchestrator{DefaultEmbeddingModel: "text-embedding-3-small"}
	svc := service.NewRetrievalService(store, q, bc, orchCfg)

	err := svc.RequestIndex(context.Background(), "proj-1", "")
	if err != nil {
		t.Fatalf("RequestIndex failed: %v", err)
	}

	if q.subject != messagequeue.SubjectRetrievalIndexRequest {
		t.Fatalf("expected subject %s, got %s", messagequeue.SubjectRetrievalIndexRequest, q.subject)
	}

	var payload messagequeue.RetrievalIndexRequestPayload
	if err := json.Unmarshal(q.data, &payload); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if payload.ProjectID != "proj-1" {
		t.Errorf("expected project_id proj-1, got %s", payload.ProjectID)
	}
	if payload.WorkspacePath != "/tmp/test" {
		t.Errorf("expected workspace_path /tmp/test, got %s", payload.WorkspacePath)
	}
	if payload.EmbeddingModel != "text-embedding-3-small" {
		t.Errorf("expected default embedding model, got %s", payload.EmbeddingModel)
	}
}

func TestRetrievalService_HandleIndexResult_Ready(t *testing.T) {
	store := &runtimeMockStore{}
	q := &captureQueue{}
	bc := &runtimeMockBroadcaster{}
	orchCfg := &config.Orchestrator{}
	svc := service.NewRetrievalService(store, q, bc, orchCfg)

	err := svc.HandleIndexResult(context.Background(), &messagequeue.RetrievalIndexResultPayload{
		ProjectID:      "proj-1",
		Status:         "ready",
		FileCount:      42,
		ChunkCount:     128,
		EmbeddingModel: "text-embedding-3-small",
	})
	if err != nil {
		t.Fatalf("HandleIndexResult failed: %v", err)
	}

	info := svc.GetIndexStatus("proj-1")
	if info == nil {
		t.Fatal("expected non-nil index info")
	}
	if info.Status != "ready" {
		t.Errorf("expected status ready, got %s", info.Status)
	}
	if info.FileCount != 42 {
		t.Errorf("expected 42 files, got %d", info.FileCount)
	}
	if info.ChunkCount != 128 {
		t.Errorf("expected 128 chunks, got %d", info.ChunkCount)
	}
}

func TestRetrievalService_HandleIndexResult_Error(t *testing.T) {
	store := &runtimeMockStore{}
	q := &captureQueue{}
	bc := &runtimeMockBroadcaster{}
	orchCfg := &config.Orchestrator{}
	svc := service.NewRetrievalService(store, q, bc, orchCfg)

	err := svc.HandleIndexResult(context.Background(), &messagequeue.RetrievalIndexResultPayload{
		ProjectID: "proj-1",
		Status:    "error",
		Error:     "embedding model not found",
	})
	if err != nil {
		t.Fatalf("HandleIndexResult failed: %v", err)
	}

	info := svc.GetIndexStatus("proj-1")
	if info == nil {
		t.Fatal("expected non-nil index info")
	}
	if info.Status != "error" {
		t.Errorf("expected status error, got %s", info.Status)
	}
	if info.Error != "embedding model not found" {
		t.Errorf("expected error message, got %s", info.Error)
	}
}

func TestRetrievalService_GetIndexStatus_NotFound(t *testing.T) {
	store := &runtimeMockStore{}
	q := &captureQueue{}
	bc := &runtimeMockBroadcaster{}
	orchCfg := &config.Orchestrator{}
	svc := service.NewRetrievalService(store, q, bc, orchCfg)

	info := svc.GetIndexStatus("nonexistent")
	if info != nil {
		t.Fatalf("expected nil for unknown project, got %+v", info)
	}
}

func TestRetrievalService_SearchSync_Timeout(t *testing.T) {
	store := &runtimeMockStore{}
	q := &captureQueue{}
	bc := &runtimeMockBroadcaster{}
	orchCfg := &config.Orchestrator{}
	svc := service.NewRetrievalService(store, q, bc, orchCfg)

	// Use a very short context deadline to trigger timeout quickly.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := svc.SearchSync(ctx, "proj-1", "test query", 10, 0.5, 0.5)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

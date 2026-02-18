package service_test

import (
	"context"
	"encoding/json"
	"strings"
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

func TestRetrievalService_SubAgentSearchSync_Publishes(t *testing.T) {
	store := &runtimeMockStore{}
	q := &captureQueue{}
	bc := &runtimeMockBroadcaster{}
	orchCfg := &config.Orchestrator{
		SubAgentModel:      "openai/gpt-4o-mini",
		SubAgentMaxQueries: 5,
		SubAgentRerank:     true,
	}
	svc := service.NewRetrievalService(store, q, bc, orchCfg)

	// Use a very short context deadline to trigger timeout quickly.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := svc.SubAgentSearchSync(ctx, "proj-1", "test query", 10, 5, "openai/gpt-4o-mini", true)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	// Verify the message was published to the correct subject.
	if q.subject != messagequeue.SubjectSubAgentSearchRequest {
		t.Fatalf("expected subject %s, got %s", messagequeue.SubjectSubAgentSearchRequest, q.subject)
	}

	var payload messagequeue.SubAgentSearchRequestPayload
	if err := json.Unmarshal(q.data, &payload); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if payload.ProjectID != "proj-1" {
		t.Errorf("expected project_id proj-1, got %s", payload.ProjectID)
	}
	if payload.Query != "test query" {
		t.Errorf("expected query 'test query', got %s", payload.Query)
	}
	if payload.TopK != 10 {
		t.Errorf("expected top_k 10, got %d", payload.TopK)
	}
	if payload.MaxQueries != 5 {
		t.Errorf("expected max_queries 5, got %d", payload.MaxQueries)
	}
	if !payload.Rerank {
		t.Error("expected rerank true")
	}
}

func TestRetrievalService_HandleSubAgentSearchResult(t *testing.T) {
	store := &runtimeMockStore{}
	q := &captureQueue{}
	bc := &runtimeMockBroadcaster{}
	orchCfg := &config.Orchestrator{}
	svc := service.NewRetrievalService(store, q, bc, orchCfg)

	// Start a goroutine that calls SubAgentSearchSync.
	resultCh := make(chan *messagequeue.SubAgentSearchResultPayload, 1)
	errCh := make(chan error, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		r, err := svc.SubAgentSearchSync(ctx, "proj-1", "handler", 10, 3, "test-model", false)
		resultCh <- r
		errCh <- err
	}()

	// Wait briefly for the request to be published.
	time.Sleep(50 * time.Millisecond)

	// Extract the request ID from the published payload.
	var reqPayload messagequeue.SubAgentSearchRequestPayload
	if err := json.Unmarshal(q.data, &reqPayload); err != nil {
		t.Fatalf("unmarshal request payload: %v", err)
	}

	// Deliver a result matching the request ID.
	svc.HandleSubAgentSearchResult(context.Background(), &messagequeue.SubAgentSearchResultPayload{
		ProjectID:       "proj-1",
		Query:           "handler",
		RequestID:       reqPayload.RequestID,
		Results:         []messagequeue.RetrievalSearchHitPayload{{Filepath: "a.go", Score: 0.9}},
		ExpandedQueries: []string{"handler function", "go handler"},
		TotalCandidates: 15,
	})

	result := <-resultCh
	err := <-errCh

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Results))
	}
	if result.Results[0].Filepath != "a.go" {
		t.Errorf("expected filepath a.go, got %s", result.Results[0].Filepath)
	}
	if len(result.ExpandedQueries) != 2 {
		t.Errorf("expected 2 expanded queries, got %d", len(result.ExpandedQueries))
	}
	if result.TotalCandidates != 15 {
		t.Errorf("expected 15 total candidates, got %d", result.TotalCandidates)
	}
}

func TestRetrievalService_HandleSubAgentSearchResult_NoWaiter(t *testing.T) {
	store := &runtimeMockStore{}
	q := &captureQueue{}
	bc := &runtimeMockBroadcaster{}
	orchCfg := &config.Orchestrator{}
	svc := service.NewRetrievalService(store, q, bc, orchCfg)

	// Delivering a result with no waiter should not panic.
	svc.HandleSubAgentSearchResult(context.Background(), &messagequeue.SubAgentSearchResultPayload{
		ProjectID: "proj-1",
		RequestID: "orphan-request-id",
	})
}

// --- Error-in-payload tests (code review #11) ---

func TestRetrievalService_SearchSync_ErrorInPayload(t *testing.T) {
	store := &runtimeMockStore{}
	q := &captureQueue{}
	bc := &runtimeMockBroadcaster{}
	orchCfg := &config.Orchestrator{}
	svc := service.NewRetrievalService(store, q, bc, orchCfg)

	resultCh := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_, err := svc.SearchSync(ctx, "proj-1", "query", 10, 0.5, 0.5)
		resultCh <- err
	}()

	// Wait for publish.
	time.Sleep(50 * time.Millisecond)

	var reqPayload messagequeue.RetrievalSearchRequestPayload
	if err := json.Unmarshal(q.data, &reqPayload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Deliver a result with an error field set.
	svc.HandleSearchResult(context.Background(), &messagequeue.RetrievalSearchResultPayload{
		ProjectID: "proj-1",
		RequestID: reqPayload.RequestID,
		Error:     "embedding service unavailable",
	})

	err := <-resultCh
	if err == nil {
		t.Fatal("expected error from SearchSync when result contains error field")
	}
	if !strings.Contains(err.Error(), "embedding service unavailable") {
		t.Errorf("expected error to contain 'embedding service unavailable', got: %s", err.Error())
	}
}

func TestRetrievalService_SubAgentSearchSync_ErrorInPayload(t *testing.T) {
	store := &runtimeMockStore{}
	q := &captureQueue{}
	bc := &runtimeMockBroadcaster{}
	orchCfg := &config.Orchestrator{SubAgentTimeout: 2 * time.Second}
	svc := service.NewRetrievalService(store, q, bc, orchCfg)

	resultCh := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_, err := svc.SubAgentSearchSync(ctx, "proj-1", "query", 10, 3, "model", true)
		resultCh <- err
	}()

	time.Sleep(50 * time.Millisecond)

	var reqPayload messagequeue.SubAgentSearchRequestPayload
	if err := json.Unmarshal(q.data, &reqPayload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Deliver a result with an error field set.
	svc.HandleSubAgentSearchResult(context.Background(), &messagequeue.SubAgentSearchResultPayload{
		ProjectID: "proj-1",
		RequestID: reqPayload.RequestID,
		Error:     "LLM quota exceeded",
	})

	err := <-resultCh
	if err == nil {
		t.Fatal("expected error from SubAgentSearchSync when result contains error field")
	}
	if !strings.Contains(err.Error(), "LLM quota exceeded") {
		t.Errorf("expected error to contain 'LLM quota exceeded', got: %s", err.Error())
	}
}

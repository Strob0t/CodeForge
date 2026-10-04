package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// captureQueue records the last published subject and data.
type captureQueue struct {
	mu      sync.Mutex
	subject string
	data    []byte
}

func (q *captureQueue) Publish(_ context.Context, subject string, data []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.subject = subject
	q.data = make([]byte, len(data))
	copy(q.data, data)
	return nil
}

func (q *captureQueue) PublishWithDedup(ctx context.Context, subject string, data []byte, _ string) error {
	return q.Publish(ctx, subject, data)
}

func (q *captureQueue) snapshot() (subject string, data []byte) {
	q.mu.Lock()
	defer q.mu.Unlock()
	data = make([]byte, len(q.data))
	copy(data, q.data)
	return q.subject, data
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
	svc := service.NewRetrievalService(store, q, bc, orchCfg, &config.Limits{SearchTimeout: 5 * time.Second})

	err := svc.RequestIndex(context.Background(), "proj-1", "", "")
	if err != nil {
		t.Fatalf("RequestIndex failed: %v", err)
	}

	subj, data := q.snapshot()
	if subj != messagequeue.SubjectRetrievalIndexRequest {
		t.Fatalf("expected subject %s, got %s", messagequeue.SubjectRetrievalIndexRequest, subj)
	}

	var payload messagequeue.RetrievalIndexRequestPayload
	if err := json.Unmarshal(data, &payload); err != nil {
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
	svc := service.NewRetrievalService(store, q, bc, orchCfg, &config.Limits{SearchTimeout: 5 * time.Second})

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

// A BM25-only index (the embedding model cannot be used, KI-130) is ready
// but ranks by keywords alone: the status says so, in the index status
// (snake_case like the frontend's RetrievalIndexStatus) and the WS event.
func TestRetrievalService_HandleIndexResult_BM25Only(t *testing.T) {
	bc := &runtimeMockBroadcaster{}
	svc := service.NewRetrievalService(&runtimeMockStore{}, &captureQueue{}, bc, &config.Orchestrator{}, &config.Limits{SearchTimeout: 5 * time.Second})

	err := svc.HandleIndexResult(context.Background(), &messagequeue.RetrievalIndexResultPayload{
		ProjectID: "proj-1", Status: "ready", ChunkCount: 3, EmbeddingModel: "text-embedding-3-small", BM25Only: true,
	})
	if err != nil {
		t.Fatalf("HandleIndexResult: %v", err)
	}

	info := svc.GetIndexStatus("proj-1")
	if info == nil || !info.BM25Only || info.Status != "ready" {
		t.Fatalf("index status = %+v, want ready and BM25-only", info)
	}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"bm25_only":true`, `"status":"ready"`, `"embedding_model":"text-embedding-3-small"`, `"chunk_count":3`} {
		if !strings.Contains(string(data), field) {
			t.Errorf("index status JSON %s lacks %s", data, field)
		}
	}
	events := bc.snapshot()
	if len(events) != 1 {
		t.Fatalf("events = %+v", events)
	}
	if ev, ok := events[0].Data.(event.RetrievalStatusEvent); !ok || !ev.BM25Only {
		t.Errorf("retrieval.status event = %+v, want bm25_only", events[0].Data)
	}
}

func TestRetrievalService_HandleIndexResult_Error(t *testing.T) {
	store := &runtimeMockStore{}
	q := &captureQueue{}
	bc := &runtimeMockBroadcaster{}
	orchCfg := &config.Orchestrator{}
	svc := service.NewRetrievalService(store, q, bc, orchCfg, &config.Limits{SearchTimeout: 5 * time.Second})

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
	svc := service.NewRetrievalService(store, q, bc, orchCfg, &config.Limits{SearchTimeout: 5 * time.Second})

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
	svc := service.NewRetrievalService(store, q, bc, orchCfg, &config.Limits{SearchTimeout: 5 * time.Second})

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
	svc := service.NewRetrievalService(store, q, bc, orchCfg, &config.Limits{SearchTimeout: 5 * time.Second})

	// Use a very short context deadline to trigger timeout quickly.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := svc.SubAgentSearchSync(ctx, "proj-1", "test query", 10, 5, "openai/gpt-4o-mini", true)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	// Verify the message was published to the correct subject.
	subj, data := q.snapshot()
	if subj != messagequeue.SubjectSubAgentSearchRequest {
		t.Fatalf("expected subject %s, got %s", messagequeue.SubjectSubAgentSearchRequest, subj)
	}

	var payload messagequeue.SubAgentSearchRequestPayload
	if err := json.Unmarshal(data, &payload); err != nil {
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
	svc := service.NewRetrievalService(store, q, bc, orchCfg, &config.Limits{SearchTimeout: 5 * time.Second})

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
	_, data := q.snapshot()
	var reqPayload messagequeue.SubAgentSearchRequestPayload
	if err := json.Unmarshal(data, &reqPayload); err != nil {
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
	svc := service.NewRetrievalService(store, q, bc, orchCfg, &config.Limits{SearchTimeout: 5 * time.Second})

	// Delivering a result with no waiter should not panic.
	svc.HandleSubAgentSearchResult(context.Background(), &messagequeue.SubAgentSearchResultPayload{
		ProjectID: "proj-1",
		RequestID: "orphan-request-id",
	})
}

// --- GlobalSearch tests (Task 4.2) ---

// autoReplyQueue intercepts search requests and delivers mock results automatically.
type autoReplyQueue struct {
	svc    *service.RetrievalService
	mu     sync.Mutex
	hits   map[string][]messagequeue.RetrievalSearchHitPayload // projectID -> hits
	callCt int
}

func (q *autoReplyQueue) Publish(_ context.Context, subject string, data []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.callCt++

	if subject == messagequeue.SubjectRetrievalSearchRequest {
		var req messagequeue.RetrievalSearchRequestPayload
		if err := json.Unmarshal(data, &req); err != nil {
			return err
		}
		hits := q.hits[req.ProjectID]
		go q.svc.HandleSearchResult(context.Background(), &messagequeue.RetrievalSearchResultPayload{
			ProjectID: req.ProjectID,
			Query:     req.Query,
			RequestID: req.RequestID,
			Results:   hits,
		})
	}
	return nil
}
func (q *autoReplyQueue) PublishWithDedup(ctx context.Context, subj string, data []byte, _ string) error {
	return q.Publish(ctx, subj, data)
}
func (q *autoReplyQueue) Subscribe(_ context.Context, _ string, _ messagequeue.Handler) (func(), error) {
	return func() {}, nil
}
func (q *autoReplyQueue) Drain() error      { return nil }
func (q *autoReplyQueue) Close() error      { return nil }
func (q *autoReplyQueue) IsConnected() bool { return true }

func TestGlobalSearch_MultipleProjects(t *testing.T) {
	store := &runtimeMockStore{
		projects: []project.Project{
			{ID: "p1", Name: "Alpha"},
			{ID: "p2", Name: "Beta"},
		},
	}
	arq := &autoReplyQueue{
		hits: map[string][]messagequeue.RetrievalSearchHitPayload{
			"p1": {{Filepath: "a.go", Score: 0.8, Content: "func a()"}},
			"p2": {{Filepath: "b.go", Score: 0.95, Content: "func b()"}},
		},
	}
	bc := &runtimeMockBroadcaster{}
	orchCfg := &config.Orchestrator{}
	svc := service.NewRetrievalService(store, arq, bc, orchCfg, &config.Limits{SearchTimeout: 5 * time.Second})
	arq.svc = svc

	results, err := svc.GlobalSearch(context.Background(), "func", nil, 20)
	if err != nil {
		t.Fatalf("GlobalSearch error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Should be sorted by score descending: Beta (0.95) first, Alpha (0.8) second.
	if results[0].ProjectID != "p2" {
		t.Errorf("expected first result from p2, got %s", results[0].ProjectID)
	}
	if results[1].ProjectID != "p1" {
		t.Errorf("expected second result from p1, got %s", results[1].ProjectID)
	}
	if results[0].Score != 0.95 {
		t.Errorf("expected score 0.95, got %f", results[0].Score)
	}
}

func TestGlobalSearch_ExplicitProjectIDs(t *testing.T) {
	store := &runtimeMockStore{
		projects: []project.Project{
			{ID: "p1", Name: "Alpha"},
			{ID: "p2", Name: "Beta"},
			{ID: "p3", Name: "Gamma"},
		},
	}
	arq := &autoReplyQueue{
		hits: map[string][]messagequeue.RetrievalSearchHitPayload{
			"p1": {{Filepath: "a.go", Score: 0.8, Content: "func a()"}},
			"p3": {{Filepath: "c.go", Score: 0.7, Content: "func c()"}},
		},
	}
	bc := &runtimeMockBroadcaster{}
	orchCfg := &config.Orchestrator{}
	svc := service.NewRetrievalService(store, arq, bc, orchCfg, &config.Limits{SearchTimeout: 5 * time.Second})
	arq.svc = svc

	// Only search p1 and p3, not p2.
	results, err := svc.GlobalSearch(context.Background(), "func", []string{"p1", "p3"}, 20)
	if err != nil {
		t.Fatalf("GlobalSearch error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.ProjectID == "p2" {
			t.Fatal("p2 should not be in results when filtering by [p1, p3]")
		}
	}
}

// An explicit project ID must name a project of the caller's tenant (the
// store scopes GetProject): the worker searches an index by project ID only.
func TestGlobalSearch_UnknownProjectIsNotFound(t *testing.T) {
	store := &runtimeMockStore{projects: []project.Project{{ID: "p1", Name: "Alpha"}}}
	arq := &autoReplyQueue{}
	svc := service.NewRetrievalService(store, arq, &runtimeMockBroadcaster{}, &config.Orchestrator{}, &config.Limits{SearchTimeout: 5 * time.Second})
	arq.svc = svc

	_, err := svc.GlobalSearch(context.Background(), "func", []string{"p1", "other-tenant-project"}, 20)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GlobalSearch error = %v, want not found", err)
	}
	if arq.callCt != 0 {
		t.Fatalf("published %d search requests", arq.callCt)
	}
}

func TestGlobalSearch_LimitRespected(t *testing.T) {
	// Before S7-F an explicit ID was searched without loading its project.
	store := &runtimeMockStore{projects: []project.Project{{ID: "p1", Name: "Alpha"}}}
	hits := make([]messagequeue.RetrievalSearchHitPayload, 10)
	for i := range hits {
		hits[i] = messagequeue.RetrievalSearchHitPayload{
			Filepath: "file.go",
			Score:    float64(10-i) / 10.0,
			Content:  "line",
		}
	}
	arq := &autoReplyQueue{
		hits: map[string][]messagequeue.RetrievalSearchHitPayload{"p1": hits},
	}
	bc := &runtimeMockBroadcaster{}
	orchCfg := &config.Orchestrator{}
	svc := service.NewRetrievalService(store, arq, bc, orchCfg, &config.Limits{SearchTimeout: 5 * time.Second})
	arq.svc = svc

	results, err := svc.GlobalSearch(context.Background(), "query", []string{"p1"}, 3)
	if err != nil {
		t.Fatalf("GlobalSearch error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results (limit), got %d", len(results))
	}
}

func TestGlobalSearch_EmptyProjects(t *testing.T) {
	store := &runtimeMockStore{projects: []project.Project{}}
	arq := &autoReplyQueue{hits: map[string][]messagequeue.RetrievalSearchHitPayload{}}
	bc := &runtimeMockBroadcaster{}
	orchCfg := &config.Orchestrator{}
	svc := service.NewRetrievalService(store, arq, bc, orchCfg, &config.Limits{SearchTimeout: 5 * time.Second})
	arq.svc = svc

	results, err := svc.GlobalSearch(context.Background(), "query", nil, 20)
	if err != nil {
		t.Fatalf("GlobalSearch error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for empty projects, got %d", len(results))
	}
}

// --- Error-in-payload tests (code review #11) ---

func TestRetrievalService_SearchSync_ErrorInPayload(t *testing.T) {
	store := &runtimeMockStore{}
	q := &captureQueue{}
	bc := &runtimeMockBroadcaster{}
	orchCfg := &config.Orchestrator{}
	svc := service.NewRetrievalService(store, q, bc, orchCfg, &config.Limits{SearchTimeout: 5 * time.Second})

	resultCh := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_, err := svc.SearchSync(ctx, "proj-1", "query", 10, 0.5, 0.5)
		resultCh <- err
	}()

	// Wait for publish.
	time.Sleep(50 * time.Millisecond)

	_, data := q.snapshot()
	var reqPayload messagequeue.RetrievalSearchRequestPayload
	if err := json.Unmarshal(data, &reqPayload); err != nil {
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
	svc := service.NewRetrievalService(store, q, bc, orchCfg, &config.Limits{SearchTimeout: 5 * time.Second})

	resultCh := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_, err := svc.SubAgentSearchSync(ctx, "proj-1", "query", 10, 3, "model", true)
		resultCh <- err
	}()

	time.Sleep(50 * time.Millisecond)

	_, data := q.snapshot()
	var reqPayload messagequeue.SubAgentSearchRequestPayload
	if err := json.Unmarshal(data, &reqPayload); err != nil {
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

package service_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/memory"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// memoryMockStore implements the subset of database.Store needed by MemoryService.
// It embeds runtimeMockStore for the full interface and overrides ListMemories.
type memoryMockStore struct {
	runtimeMockStore
	memories []memory.Memory
}

func (m *memoryMockStore) ListMemories(_ context.Context, _ string) ([]memory.Memory, error) {
	return m.memories, nil
}

// --- Store validation tests ---

func TestMemoryService_Store_Validation(t *testing.T) {
	q := &captureQueue{}
	svc := service.NewMemoryService(&memoryMockStore{}, q)

	tests := []struct {
		name    string
		req     *memory.CreateRequest
		wantErr string
	}{
		{
			name:    "missing project_id",
			req:     &memory.CreateRequest{Content: "hello", Kind: memory.KindObservation, Importance: 0.5},
			wantErr: "project_id is required",
		},
		{
			name:    "missing content",
			req:     &memory.CreateRequest{ProjectID: "proj-1", Kind: memory.KindObservation, Importance: 0.5},
			wantErr: "content is required",
		},
		{
			name:    "invalid kind",
			req:     &memory.CreateRequest{ProjectID: "proj-1", Content: "hello", Kind: "bogus", Importance: 0.5},
			wantErr: "invalid kind",
		},
		{
			name:    "importance out of range (too high)",
			req:     &memory.CreateRequest{ProjectID: "proj-1", Content: "hello", Kind: memory.KindDecision, Importance: 1.5},
			wantErr: "importance must be between 0 and 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.Store(context.Background(), tt.req)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestMemoryService_Store_PublishesNATS(t *testing.T) {
	q := &captureQueue{}
	svc := service.NewMemoryService(&memoryMockStore{}, q)

	req := &memory.CreateRequest{
		ProjectID:  "proj-1",
		Content:    "discovered a bug in handler",
		Kind:       memory.KindObservation,
		Importance: 0.8,
	}

	if err := svc.Store(context.Background(), req); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	subj, data := q.snapshot()
	if subj != messagequeue.SubjectMemoryStore {
		t.Fatalf("expected subject %s, got %s", messagequeue.SubjectMemoryStore, subj)
	}

	var payload memory.CreateRequest
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal published payload: %v", err)
	}
	if payload.ProjectID != "proj-1" {
		t.Errorf("expected project_id 'proj-1', got %q", payload.ProjectID)
	}
	if payload.Content != "discovered a bug in handler" {
		t.Errorf("unexpected content: %q", payload.Content)
	}
	if payload.Kind != memory.KindObservation {
		t.Errorf("expected kind 'observation', got %q", payload.Kind)
	}
}

func TestMemoryService_Store_QueueError(t *testing.T) {
	q := &failQueue{err: errMockNotFound}
	svc := service.NewMemoryService(&memoryMockStore{}, q)

	req := &memory.CreateRequest{
		ProjectID:  "proj-1",
		Content:    "something",
		Kind:       memory.KindInsight,
		Importance: 0.3,
	}

	err := svc.Store(context.Background(), req)
	if err == nil {
		t.Fatal("expected publish error, got nil")
	}
	if !strings.Contains(err.Error(), "publish memory store") {
		t.Errorf("expected error containing 'publish memory store', got %q", err.Error())
	}
}

// --- RecallSync tests ---

func TestMemoryService_RecallSync_HappyPath(t *testing.T) {
	q := &captureQueue{}
	store := &memoryMockStore{}
	svc := service.NewMemoryService(store, q)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	type recallResult struct {
		result *memory.RecallResult
		err    error
	}
	ch := make(chan recallResult, 1)

	go func() {
		r, err := svc.RecallSync(ctx, &memory.RecallRequest{
			ProjectID: "proj-1",
			Query:     "bug in handler",
			TopK:      5,
		})
		ch <- recallResult{result: r, err: err}
	}()

	// Wait for publish.
	time.Sleep(50 * time.Millisecond)

	// Extract request_id from published payload.
	subj, data := q.snapshot()
	if subj != messagequeue.SubjectMemoryRecall {
		t.Fatalf("expected subject %s, got %s", messagequeue.SubjectMemoryRecall, subj)
	}

	var reqPayload memory.RecallRequest
	if err := json.Unmarshal(data, &reqPayload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Deliver result.
	svc.HandleRecallResult(context.Background(), &memory.RecallResult{
		RequestID: reqPayload.RequestID,
		ProjectID: "proj-1",
		Query:     "bug in handler",
		Results: []memory.ScoredResult{
			{ID: "mem-1", Content: "found a nil pointer", Kind: "error", Score: 0.95},
			{ID: "mem-2", Content: "handler refactored", Kind: "observation", Score: 0.82},
		},
	})

	res := <-ch
	if res.err != nil {
		t.Fatalf("unexpected error: %v", res.err)
	}
	if res.result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(res.result.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res.result.Results))
	}
	if res.result.Results[0].ID != "mem-1" {
		t.Errorf("expected first result ID 'mem-1', got %q", res.result.Results[0].ID)
	}
	if res.result.Results[0].Score != 0.95 {
		t.Errorf("expected score 0.95, got %f", res.result.Results[0].Score)
	}
}

func TestMemoryService_RecallSync_Timeout(t *testing.T) {
	q := &captureQueue{}
	svc := service.NewMemoryService(&memoryMockStore{}, q)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	result, err := svc.RecallSync(ctx, &memory.RecallRequest{
		ProjectID: "proj-1",
		Query:     "something",
		TopK:      5,
	})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected error containing 'timeout', got %q", err.Error())
	}
	if result != nil {
		t.Errorf("expected nil result on timeout, got %+v", result)
	}
}

func TestMemoryService_RecallSync_ErrorInResult(t *testing.T) {
	q := &captureQueue{}
	svc := service.NewMemoryService(&memoryMockStore{}, q)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errCh := make(chan error, 1)

	go func() {
		_, err := svc.RecallSync(ctx, &memory.RecallRequest{
			ProjectID: "proj-1",
			Query:     "test query",
			TopK:      5,
		})
		errCh <- err
	}()

	// Wait for publish.
	time.Sleep(50 * time.Millisecond)

	_, data := q.snapshot()
	var reqPayload memory.RecallRequest
	if err := json.Unmarshal(data, &reqPayload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Deliver a result with an error field.
	svc.HandleRecallResult(context.Background(), &memory.RecallResult{
		RequestID: reqPayload.RequestID,
		ProjectID: "proj-1",
		Error:     "embedding model unavailable",
	})

	err := <-errCh
	if err == nil {
		t.Fatal("expected error from RecallSync when result has error field")
	}
	if !strings.Contains(err.Error(), "embedding model unavailable") {
		t.Errorf("expected error containing 'embedding model unavailable', got %q", err.Error())
	}
}

func TestMemoryService_RecallSync_MissingFields(t *testing.T) {
	q := &captureQueue{}
	svc := service.NewMemoryService(&memoryMockStore{}, q)

	tests := []struct {
		name    string
		req     *memory.RecallRequest
		wantErr string
	}{
		{
			name:    "missing project_id",
			req:     &memory.RecallRequest{Query: "test", TopK: 5},
			wantErr: "project_id is required",
		},
		{
			name:    "missing query",
			req:     &memory.RecallRequest{ProjectID: "proj-1", TopK: 5},
			wantErr: "query is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.RecallSync(context.Background(), tt.req)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestMemoryService_RecallSync_DefaultTopK(t *testing.T) {
	q := &captureQueue{}
	svc := service.NewMemoryService(&memoryMockStore{}, q)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// TopK=0 should be defaulted to 10. We verify by checking the published payload.
	_, _ = svc.RecallSync(ctx, &memory.RecallRequest{
		ProjectID: "proj-1",
		Query:     "test",
		TopK:      0,
	})

	_, data := q.snapshot()
	var payload memory.RecallRequest
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.TopK != 10 {
		t.Errorf("expected TopK to be defaulted to 10, got %d", payload.TopK)
	}
}

// --- ListByProject tests ---

func TestMemoryService_ListByProject(t *testing.T) {
	t.Run("returns memories", func(t *testing.T) {
		store := &memoryMockStore{
			memories: []memory.Memory{
				{ID: "mem-1", ProjectID: "proj-1", Content: "first", Kind: memory.KindObservation},
				{ID: "mem-2", ProjectID: "proj-1", Content: "second", Kind: memory.KindDecision},
			},
		}
		q := &captureQueue{}
		svc := service.NewMemoryService(store, q)

		memories, err := svc.ListByProject(context.Background(), "proj-1")
		if err != nil {
			t.Fatalf("ListByProject failed: %v", err)
		}
		if len(memories) != 2 {
			t.Fatalf("expected 2 memories, got %d", len(memories))
		}
		if memories[0].ID != "mem-1" {
			t.Errorf("expected first memory ID 'mem-1', got %q", memories[0].ID)
		}
		if memories[1].Kind != memory.KindDecision {
			t.Errorf("expected second memory kind 'decision', got %q", memories[1].Kind)
		}
	})

	t.Run("empty project returns empty", func(t *testing.T) {
		store := &memoryMockStore{memories: nil}
		q := &captureQueue{}
		svc := service.NewMemoryService(store, q)

		memories, err := svc.ListByProject(context.Background(), "empty-proj")
		if err != nil {
			t.Fatalf("ListByProject failed: %v", err)
		}
		if len(memories) != 0 {
			t.Fatalf("expected 0 memories for empty project, got %d", len(memories))
		}
	})
}

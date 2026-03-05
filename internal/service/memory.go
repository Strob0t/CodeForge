package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/memory"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// defaultRecallTimeout is the fallback when the caller's context has no deadline.
const defaultRecallTimeout = 30 * time.Second

// MemoryService manages persistent agent memories. Storage and recall of
// embedding-scored memories is delegated to the Python worker via NATS;
// listing is served directly from the database.
type MemoryService struct {
	db           database.Store
	queue        messagequeue.Queue
	recallWaiter *syncWaiter[memory.RecallResult]
}

// NewMemoryService creates a new MemoryService.
func NewMemoryService(db database.Store, queue messagequeue.Queue) *MemoryService {
	return &MemoryService{
		db:           db,
		queue:        queue,
		recallWaiter: newSyncWaiter[memory.RecallResult]("memory-recall"),
	}
}

// Store publishes a memory store request to the Python worker (which computes
// embeddings and importance) and persists it.
func (s *MemoryService) Store(ctx context.Context, req *memory.CreateRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal memory store: %w", err)
	}

	if err := s.queue.Publish(ctx, messagequeue.SubjectMemoryStore, data); err != nil {
		return fmt.Errorf("publish memory store: %w", err)
	}

	slog.Debug("memory store dispatched", "project_id", req.ProjectID, "kind", req.Kind)
	return nil
}

// RecallSync publishes a memory recall request and waits for the Python worker
// to score and return the top-k results. Uses a syncWaiter for correlation.
func (s *MemoryService) RecallSync(ctx context.Context, req *memory.RecallRequest) (*memory.RecallResult, error) {
	if req.ProjectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	if req.Query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if req.TopK <= 0 {
		req.TopK = 10
	}

	requestID, err := generateRequestID()
	if err != nil {
		return nil, err
	}
	req.RequestID = requestID

	ch := s.recallWaiter.register(requestID)
	defer s.recallWaiter.unregister(requestID)

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal memory recall: %w", err)
	}

	if err := s.queue.Publish(ctx, messagequeue.SubjectMemoryRecall, data); err != nil {
		return nil, fmt.Errorf("publish memory recall: %w", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, defaultRecallTimeout)
	defer cancel()

	select {
	case result := <-ch:
		if result.Error != "" {
			return nil, fmt.Errorf("memory recall error: %s", result.Error)
		}
		return result, nil
	case <-timeoutCtx.Done():
		return nil, fmt.Errorf("memory recall timeout for project %s", req.ProjectID)
	}
}

// HandleRecallResult delivers a recall result from the Python worker to the
// waiting caller identified by RequestID.
func (s *MemoryService) HandleRecallResult(_ context.Context, result *memory.RecallResult) {
	s.recallWaiter.deliver(result.RequestID, result)
}

// ListByProject returns all memories for a project, directly from the database.
func (s *MemoryService) ListByProject(ctx context.Context, projectID string) ([]memory.Memory, error) {
	return s.db.ListMemories(ctx, projectID)
}

// StartSubscribers subscribes to memory result subjects and returns cancel funcs.
func (s *MemoryService) StartSubscribers(ctx context.Context) ([]func(), error) {
	cancelRecall, err := s.queue.Subscribe(ctx, messagequeue.SubjectMemoryRecallResult, func(msgCtx context.Context, _ string, data []byte) error {
		var result memory.RecallResult
		if err := json.Unmarshal(data, &result); err != nil {
			return fmt.Errorf("unmarshal memory recall result: %w", err)
		}
		s.HandleRecallResult(msgCtx, &result)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe memory recall result: %w", err)
	}

	return []func(){cancelRecall}, nil
}

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/domain/memory"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// NATS subjects for memory operations.
const (
	SubjectMemoryStore  = "memory.store"
	SubjectMemoryRecall = "memory.recall"
)

// MemoryService manages persistent agent memories. Storage and recall of
// embedding-scored memories is delegated to the Python worker via NATS;
// listing is served directly from the database.
type MemoryService struct {
	db    database.Store
	queue messagequeue.Queue
}

// NewMemoryService creates a new MemoryService.
func NewMemoryService(db database.Store, queue messagequeue.Queue) *MemoryService {
	return &MemoryService{db: db, queue: queue}
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

	if err := s.queue.Publish(ctx, SubjectMemoryStore, data); err != nil {
		return fmt.Errorf("publish memory store: %w", err)
	}

	slog.Debug("memory store dispatched", "project_id", req.ProjectID, "kind", req.Kind)
	return nil
}

// Recall publishes a memory recall request to the Python worker (which runs
// composite scoring) and returns asynchronously via NATS.
func (s *MemoryService) Recall(ctx context.Context, req memory.RecallRequest) error {
	if req.ProjectID == "" {
		return fmt.Errorf("project_id is required")
	}
	if req.Query == "" {
		return fmt.Errorf("query is required")
	}
	if req.TopK <= 0 {
		req.TopK = 10
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal memory recall: %w", err)
	}

	if err := s.queue.Publish(ctx, SubjectMemoryRecall, data); err != nil {
		return fmt.Errorf("publish memory recall: %w", err)
	}

	return nil
}

// ListByProject returns all memories for a project, directly from the database.
func (s *MemoryService) ListByProject(ctx context.Context, projectID string) ([]memory.Memory, error) {
	return s.db.ListMemories(ctx, projectID)
}

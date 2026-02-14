package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// TaskService handles task business logic including NATS dispatch.
type TaskService struct {
	store database.Store
	queue messagequeue.Queue
}

// NewTaskService creates a new TaskService.
func NewTaskService(store database.Store, queue messagequeue.Queue) *TaskService {
	return &TaskService{store: store, queue: queue}
}

// List returns all tasks for a project.
func (s *TaskService) List(ctx context.Context, projectID string) ([]task.Task, error) {
	return s.store.ListTasks(ctx, projectID)
}

// Get returns a task by ID.
func (s *TaskService) Get(ctx context.Context, id string) (*task.Task, error) {
	return s.store.GetTask(ctx, id)
}

// Create creates a task, saves it to DB, and publishes it to NATS.
func (s *TaskService) Create(ctx context.Context, req task.CreateRequest) (*task.Task, error) {
	t, err := s.store.CreateTask(ctx, req)
	if err != nil {
		return nil, err
	}

	// Publish to NATS for worker pickup
	data, err := json.Marshal(t)
	if err != nil {
		return t, fmt.Errorf("marshal task for queue: %w", err)
	}

	if err := s.queue.Publish(ctx, messagequeue.SubjectTaskCreated, data); err != nil {
		slog.Error("failed to publish task to queue", "task_id", t.ID, "error", err)
		// Task is saved in DB, so we return it even if queue publish fails.
		// The task can be retried later.
	}

	return t, nil
}

package service

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// TaskService handles task business logic.
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

// Create creates a task and saves it to DB.
// Task execution is triggered later via runs.start when a run is created.
func (s *TaskService) Create(ctx context.Context, req task.CreateRequest) (*task.Task, error) {
	return s.store.CreateTask(ctx, req)
}

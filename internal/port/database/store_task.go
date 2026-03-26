package database

import (
	"context"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/task"
)

// TaskStore defines database operations for tasks and active work visibility.
type TaskStore interface {
	// Tasks
	ListTasks(ctx context.Context, projectID string) ([]task.Task, error)
	GetTask(ctx context.Context, id string) (*task.Task, error)
	CreateTask(ctx context.Context, req task.CreateRequest) (*task.Task, error)
	UpdateTaskStatus(ctx context.Context, id string, status task.Status) error
	UpdateTaskResult(ctx context.Context, id string, result task.Result, costUSD float64) error

	// Active Work Visibility (Phase 24)
	ListActiveWork(ctx context.Context, projectID string) ([]task.ActiveWorkItem, error)
	ClaimTask(ctx context.Context, taskID, agentID string, version int) (*task.ClaimResult, error)
	ReleaseStaleWork(ctx context.Context, threshold time.Duration) ([]task.Task, error)
}

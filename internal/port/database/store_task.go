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
	// QueueTask moves a task that is neither queued nor running to queued
	// and assigns it to agentID; domain.ErrConflict when it is (a dispatch
	// is under way).
	QueueTask(ctx context.Context, id, agentID string) error
	// UpdateTaskResult stores a task's result and cost together with the status
	// the result leaves it in.
	UpdateTaskResult(ctx context.Context, id string, status task.Status, result task.Result, costUSD float64) error

	// Active Work Visibility (Phase 24)
	ListActiveWork(ctx context.Context, projectID string) ([]task.ActiveWorkItem, error)
	ClaimTask(ctx context.Context, taskID, agentID string, version int) (*task.ClaimResult, error)
	// TouchTaskHeartbeat records a worker heartbeat of a queued or running
	// task (KI-65).
	TouchTaskHeartbeat(ctx context.Context, id string) error
	// ListTasksWithStaleHeartbeat returns up to limit queued or running tasks
	// whose last heartbeat is older than idleFor, across all tenants (watchdog
	// use). Tasks without a heartbeat since their last change (not accepted by
	// a worker yet) are not listed.
	ListTasksWithStaleHeartbeat(ctx context.Context, idleFor time.Duration, limit int) ([]task.Task, error)
}

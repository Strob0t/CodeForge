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
	// QueueTask moves a task that is neither queued nor running to queued,
	// assigns it to agentID and records the dispatch (its ID and time);
	// domain.ErrConflict when it is (a dispatch is under way).
	QueueTask(ctx context.Context, id, agentID, dispatchID string) error
	// UpdateTaskResult stores a task's result and cost together with the status
	// the result leaves it in.
	UpdateTaskResult(ctx context.Context, id string, status task.Status, result task.Result, costUSD float64) error
	// EndTaskDispatch ends the task's dispatch dispatchID with status and
	// result, but only while it is the task's current dispatch and the task
	// is queued or running: domain.ErrConflict otherwise (a result arrived
	// or the task was dispatched again), domain.ErrNotFound for an unknown
	// task or one of another tenant.
	EndTaskDispatch(ctx context.Context, id, dispatchID string, status task.Status, result task.Result) error
	// RecordTaskResult records a worker's result of the task's dispatch
	// dispatchID. The result of the task's current dispatch (the task queued
	// or running) ends it with status, result and cost and reports current;
	// a result of another dispatch (ended, or replaced by a newer one) only
	// adds its cost, once per dispatch, and reports not current.
	// domain.ErrNotFound for an unknown task or one of another tenant.
	RecordTaskResult(ctx context.Context, id, dispatchID string, status task.Status, result task.Result, costUSD float64) (current bool, err error)

	// Active Work Visibility (Phase 24)
	ListActiveWork(ctx context.Context, projectID string) ([]task.ActiveWorkItem, error)
	ClaimTask(ctx context.Context, taskID, agentID string, version int) (*task.ClaimResult, error)
	// TouchTaskHeartbeat records a worker heartbeat of a queued or running
	// task (KI-65) for its dispatch dispatchID; a heartbeat of another
	// dispatch is ignored.
	TouchTaskHeartbeat(ctx context.Context, id, dispatchID string) error
	// ListTasksWithStaleHeartbeat returns up to limit queued or running tasks
	// whose last heartbeat for their current dispatch is older than idleFor,
	// across all tenants (watchdog use), with their dispatch. Dispatches
	// without a heartbeat (not accepted by a worker yet) are not listed.
	ListTasksWithStaleHeartbeat(ctx context.Context, idleFor time.Duration, limit int) ([]task.Task, error)
	// ListTasksNeverAccepted returns up to limit queued tasks dispatched more
	// than olderThan ago whose dispatch has no heartbeat (no worker accepted
	// it), across all tenants (watchdog use), with their dispatch.
	ListTasksNeverAccepted(ctx context.Context, olderThan time.Duration, limit int) ([]task.Task, error)
}

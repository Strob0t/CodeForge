package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// ActiveWorkService provides visibility into running/queued tasks and
// supports atomic task claiming to prevent redundant parallel execution.
type ActiveWorkService struct {
	store database.Store
	hub   broadcast.Broadcaster
}

// NewActiveWorkService creates a new ActiveWorkService.
func NewActiveWorkService(store database.Store, hub broadcast.Broadcaster) *ActiveWorkService {
	return &ActiveWorkService{store: store, hub: hub}
}

// ListActiveWork returns all running/queued tasks for a project with
// their assigned agent and latest run metadata.
func (s *ActiveWorkService) ListActiveWork(ctx context.Context, projectID string) ([]task.ActiveWorkItem, error) {
	items, err := s.store.ListActiveWork(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list active work: %w", err)
	}
	return items, nil
}

// ClaimTask atomically claims a pending task for an agent. It reads the
// task's current version and uses optimistic locking to prevent races.
// On success it broadcasts an EventActiveWorkClaimed event.
func (s *ActiveWorkService) ClaimTask(ctx context.Context, taskID, agentID string) (*task.ClaimResult, error) {
	t, err := s.store.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("get task for claim: %w", err)
	}

	if t.Status != task.StatusPending {
		return &task.ClaimResult{Claimed: false, Reason: "task not pending"}, nil
	}

	ag, err := s.store.GetAgent(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("get agent for claim: %w", err)
	}

	result, err := s.store.ClaimTask(ctx, taskID, agentID, t.Version)
	if err != nil {
		return nil, fmt.Errorf("claim task: %w", err)
	}

	if result.Claimed {
		s.hub.BroadcastEvent(ctx, ws.EventActiveWorkClaimed, ws.ActiveWorkClaimedEvent{
			TaskID:    taskID,
			TaskTitle: t.Title,
			ProjectID: t.ProjectID,
			AgentID:   agentID,
			AgentName: ag.Name,
		})
		slog.Info("task claimed", "task_id", taskID, "agent_id", agentID)
	}

	return result, nil
}

// ReleaseStaleWork finds tasks stuck in running/queued status longer than
// the given threshold and resets them to pending. Broadcasts an
// EventActiveWorkReleased event per released task.
func (s *ActiveWorkService) ReleaseStaleWork(ctx context.Context, threshold time.Duration) ([]task.Task, error) {
	released, err := s.store.ReleaseStaleWork(ctx, threshold)
	if err != nil {
		return nil, fmt.Errorf("release stale work: %w", err)
	}

	for i := range released {
		s.hub.BroadcastEvent(ctx, ws.EventActiveWorkReleased, ws.ActiveWorkReleasedEvent{
			TaskID:    released[i].ID,
			ProjectID: released[i].ProjectID,
			Reason:    "stale task released after timeout",
		})
	}

	if len(released) > 0 {
		slog.Info("released stale work", "count", len(released), "threshold", threshold)
	}

	return released, nil
}

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// HandleTaskHeartbeat records the heartbeat a worker sends while it executes
// a backend task, in the task's tenant (KI-65). A heartbeat that cannot be
// recorded is logged and dropped: the next one follows.
func (s *AgentService) HandleTaskHeartbeat(ctx context.Context, hb *messagequeue.TaskHeartbeatPayload) error {
	ctx = withPayloadTenant(ctx, hb.TenantID)
	logBestEffort(ctx, s.store.TouchTaskHeartbeat(ctx, hb.TaskID), "TouchTaskHeartbeat", slog.String("task_id", hb.TaskID))
	return nil
}

// StartHeartbeatSubscriber subscribes to the workers' task heartbeats.
func (s *AgentService) StartHeartbeatSubscriber(ctx context.Context) (cancel func(), err error) {
	return s.queue.Subscribe(ctx, messagequeue.SubjectTaskHeartbeat, func(msgCtx context.Context, _ string, data []byte) error {
		var hb messagequeue.TaskHeartbeatPayload
		if err := json.Unmarshal(data, &hb); err != nil {
			return fmt.Errorf("unmarshal task heartbeat: %w", err)
		}
		return s.HandleTaskHeartbeat(msgCtx, &hb)
	})
}

// FailTasksWithLostWorker fails the queued or running backend tasks whose
// worker sent no heartbeat for lostAfter (KI-65; see LostWorkerAfter): the
// worker is told to stop the task (it may only have lost its connection),
// the task is failed through the task result path (which resets its agent to
// idle), in the task's tenant. Tasks a worker has not accepted yet have
// no heartbeat and are not failed; lostAfter 0 disables the check. It returns
// how many lost tasks it failed.
func (s *AgentService) FailTasksWithLostWorker(ctx context.Context, lostAfter time.Duration) (int, error) {
	if lostAfter <= 0 {
		return 0, nil
	}
	lost, err := s.store.ListTasksWithStaleHeartbeat(ctx, lostAfter, staleRunBatch)
	if err != nil {
		return 0, fmt.Errorf("list tasks with lost worker: %w", err)
	}
	reason := lostWorkerReason(lostAfter)
	handled := 0
	var errs []error
	for i := range lost {
		t := &lost[i]
		taskCtx := withEntityTenant(ctx, t.TenantID)
		slog.WarnContext(taskCtx, "task worker heartbeat lost, failing the task", "task_id", t.ID, "after", lostAfter)
		s.tellWorkerToStopTask(taskCtx, t.ID)
		if err := s.recordResult(taskCtx, task.StatusFailed, task.Result{Error: reason}, t.ID, t.ProjectID, 0); err != nil {
			errs = append(errs, fmt.Errorf("task %s: %w", t.ID, err))
			continue
		}
		s.hub.BroadcastEvent(taskCtx, event.EventActiveWorkReleased, event.ActiveWorkReleasedEvent{
			TaskID:    t.ID,
			ProjectID: t.ProjectID,
			Reason:    reason,
		})
		handled++
	}
	return handled, errors.Join(errs...)
}

// tellWorkerToStopTask publishes tasks.cancel for a task; best effort.
func (s *AgentService) tellWorkerToStopTask(ctx context.Context, taskID string) {
	data, err := json.Marshal(messagequeue.TaskCancelPayload{TaskID: taskID})
	if err == nil {
		err = s.queue.Publish(ctx, messagequeue.SubjectTaskCancel, data)
	}
	logBestEffort(ctx, err, "publish tasks.cancel", slog.String("task_id", taskID))
}

// resetAgent sets an agent whose task ended back to idle; best effort.
func (s *AgentService) resetAgent(ctx context.Context, agentID, projectID string) {
	if err := s.store.UpdateAgentStatus(ctx, agentID, agent.StatusIdle); err != nil {
		logBestEffort(ctx, err, "UpdateAgentStatus", slog.String("agent_id", agentID))
		return
	}
	s.hub.BroadcastEvent(ctx, event.EventAgentStatus, event.AgentStatusEvent{
		AgentID:   agentID,
		ProjectID: projectID,
		Status:    string(agent.StatusIdle),
	})
}

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
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
	logBestEffort(ctx, s.store.TouchTaskHeartbeat(ctx, hb.TaskID, hb.DispatchID), "TouchTaskHeartbeat",
		slog.String("task_id", hb.TaskID), slog.String("dispatch_id", hb.DispatchID))
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
// task's dispatch is failed (announced like a worker's result, which resets
// its agent to idle), in the task's tenant, and its worker is told to stop
// (it may only have lost its connection). A task whose result arrived or
// that was dispatched again since it was listed is skipped. Tasks a worker
// has not accepted yet have no heartbeat and are not failed; lostAfter 0
// disables the check. It returns how many lost tasks it failed.
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
		slog.WarnContext(taskCtx, "task worker heartbeat lost, failing the task", "task_id", t.ID, "dispatch_id", t.DispatchID, "after", lostAfter)
		if err := s.failTaskDispatch(taskCtx, t, reason); err != nil {
			if errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrNotFound) {
				slog.InfoContext(taskCtx, "lost task ended or was dispatched again meanwhile, skipped", "task_id", t.ID)
				continue
			}
			errs = append(errs, fmt.Errorf("task %s: %w", t.ID, err))
			continue
		}
		s.tellWorkerToStopTask(taskCtx, t)
		s.hub.BroadcastEvent(taskCtx, event.EventActiveWorkReleased, event.ActiveWorkReleasedEvent{
			TaskID:    t.ID,
			ProjectID: t.ProjectID,
			Reason:    reason,
		})
		handled++
	}
	return handled, errors.Join(errs...)
}

// FailTasksNeverAccepted fails the backend tasks whose dispatch no worker
// accepted within acceptAfter (runtime.task_accept_timeout; 0 disables the
// check): the dispatch got no heartbeat because its message still waits in
// NATS (every worker busy) or was lost. The dispatch is failed (announced
// like a worker's result, which resets its agent to idle) in the task's
// tenant, and a tasks.cancel is published so a worker that picks the
// message up later skips it. A task whose dispatch ended meanwhile is
// skipped. It returns how many tasks it failed.
func (s *AgentService) FailTasksNeverAccepted(ctx context.Context, acceptAfter time.Duration) (int, error) {
	if acceptAfter <= 0 {
		return 0, nil
	}
	waiting, err := s.store.ListTasksNeverAccepted(ctx, acceptAfter, staleRunBatch)
	if err != nil {
		return 0, fmt.Errorf("list tasks never accepted: %w", err)
	}
	reason := fmt.Sprintf("no worker accepted the task within %s (task_accept_timeout)", acceptAfter)
	handled := 0
	var errs []error
	for i := range waiting {
		t := &waiting[i]
		taskCtx := withEntityTenant(ctx, t.TenantID)
		slog.WarnContext(taskCtx, "no worker accepted the task, failing it", "task_id", t.ID, "dispatch_id", t.DispatchID, "after", acceptAfter)
		if err := s.failTaskDispatch(taskCtx, t, reason); err != nil {
			if errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrNotFound) {
				slog.InfoContext(taskCtx, "unaccepted task ended or was dispatched again meanwhile, skipped", "task_id", t.ID)
				continue
			}
			errs = append(errs, fmt.Errorf("task %s: %w", t.ID, err))
			continue
		}
		s.tellWorkerToStopTask(taskCtx, t)
		s.hub.BroadcastEvent(taskCtx, event.EventActiveWorkReleased, event.ActiveWorkReleasedEvent{
			TaskID:    t.ID,
			ProjectID: t.ProjectID,
			Reason:    reason,
		})
		handled++
	}
	return handled, errors.Join(errs...)
}

// deadLetteredTaskError is the error of a task whose dispatch was dead-lettered.
const deadLetteredTaskError = "the task's dispatch could not be delivered to a worker (dead-lettered)"

// HandleDeadLetteredTaskDispatch fails the task whose dispatch a worker
// dead-lettered (rejected as invalid, or not accepted within its
// deliveries), in the dispatch's tenant: no worker runs it and it sends no
// heartbeat. Only the task's current dispatch of a task still queued or
// running is failed; a dispatch that cannot be read, or of a task that ended
// or was dispatched again, is ignored.
func (s *AgentService) HandleDeadLetteredTaskDispatch(ctx context.Context, data []byte) error {
	var dispatch messagequeue.TaskAgentPayload
	if err := json.Unmarshal(data, &dispatch); err != nil || dispatch.TaskID == "" {
		slog.Warn("dead-lettered task dispatch without a task, ignored", "error", err)
		return nil
	}
	ctx = withPayloadTenant(ctx, dispatch.TenantID)
	slog.WarnContext(ctx, "task dispatch dead-lettered, failing the task", "task_id", dispatch.TaskID, "dispatch_id", dispatch.DispatchID)
	t := &task.Task{ID: dispatch.TaskID, ProjectID: dispatch.ProjectID, DispatchID: dispatch.DispatchID}
	err := s.failTaskDispatch(ctx, t, deadLetteredTaskError)
	if errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrNotFound) {
		slog.InfoContext(ctx, "dead-lettered dispatch of a task that ended or was dispatched again, ignored", "task_id", dispatch.TaskID)
		return nil
	}
	return err
}

// StartDeadLetterSubscriber subscribes to the dead-lettered task dispatches
// of every backend (tasks.agent.*.dlq).
func (s *AgentService) StartDeadLetterSubscriber(ctx context.Context) (cancel func(), err error) {
	return s.queue.Subscribe(ctx, messagequeue.SubjectTaskAgent+".*"+deadLetterSuffix, func(msgCtx context.Context, _ string, data []byte) error {
		return s.HandleDeadLetteredTaskDispatch(msgCtx, data)
	})
}

// tellWorkerToStopTask tells the worker executing a task to stop it through
// the backend of the task's agent, as StopTask does; best effort. A task that
// names no agent (dispatched before tasks recorded their agent) cannot be
// routed to a backend and is only logged.
func (s *AgentService) tellWorkerToStopTask(ctx context.Context, t *task.Task) {
	if t.AgentID == "" {
		slog.WarnContext(ctx, "task names no agent, its worker is not told to stop", "task_id", t.ID)
		return
	}
	ag, err := s.store.GetAgent(ctx, t.AgentID)
	if err != nil {
		logBestEffort(ctx, err, "GetAgent", slog.String("agent_id", t.AgentID), slog.String("task_id", t.ID))
		return
	}
	logBestEffort(ctx, stopOnBackend(ctx, ag, t.ID), "stop task on its backend", slog.String("task_id", t.ID))
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

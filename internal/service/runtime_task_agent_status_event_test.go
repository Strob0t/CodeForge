package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const statusEventTenant = "00000000-0000-0000-0000-0000000000a7"

// statusEvents returns the task.status and agent.status payloads broadcast so
// far, and fails when one was sent without the run's tenant.
func statusEvents(t *testing.T, bc *runtimeMockBroadcaster) ([]event.TaskStatusEvent, []event.AgentStatusEvent) {
	t.Helper()
	var tasks []event.TaskStatusEvent
	var agents []event.AgentStatusEvent
	for _, ev := range bc.snapshot() {
		switch payload := ev.Data.(type) {
		case event.TaskStatusEvent:
			if ev.EventType != event.EventTaskStatus {
				t.Fatalf("TaskStatusEvent sent as %q", ev.EventType)
			}
			tasks = append(tasks, payload)
		case event.AgentStatusEvent:
			if ev.EventType != event.EventAgentStatus {
				t.Fatalf("AgentStatusEvent sent as %q", ev.EventType)
			}
			agents = append(agents, payload)
		default:
			continue
		}
		if ev.Tenant != statusEventTenant {
			t.Fatalf("%s broadcast in tenant %q, want %q", ev.EventType, ev.Tenant, statusEventTenant)
		}
	}
	return tasks, agents
}

// KI-74: the runtime moves the task and the agent on run start and end; the
// project page follows them through task.status / agent.status instead of
// refetching both on every run.status.
func TestRunLifecycle_BroadcastsTaskAndAgentStatus(t *testing.T) {
	ctx := tenantctx.WithTenant(context.Background(), statusEventTenant)

	t.Run("run start", func(t *testing.T) {
		svc, _, _, bc := newRuntimeTestEnv()
		if _, err := svc.StartRun(ctx, &run.StartRequest{TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1"}); err != nil {
			t.Fatalf("StartRun: %v", err)
		}
		tasks, agents := statusEvents(t, bc)
		if len(tasks) != 1 || tasks[0] != (event.TaskStatusEvent{TaskID: "task-1", ProjectID: "proj-1", Status: "running", AgentID: "agent-1"}) {
			t.Fatalf("task.status events = %+v, want task-1 running by agent-1", tasks)
		}
		if len(agents) != 1 || agents[0] != (event.AgentStatusEvent{AgentID: "agent-1", ProjectID: "proj-1", Status: "running"}) {
			t.Fatalf("agent.status events = %+v, want agent-1 running", agents)
		}
	})

	for _, tc := range []struct {
		runStatus  string
		taskStatus string
	}{
		{"completed", "completed"},
		{"failed", "failed"},
		{"cancelled", "cancelled"},
		{"timeout", "failed"},
	} {
		t.Run("run end "+tc.runStatus, func(t *testing.T) {
			svc, store, _, bc := newRuntimeTestEnv()
			store.mu.Lock()
			store.runs = append(store.runs, run.Run{
				ID: "run-s1", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
				PolicyProfile: "plan-readonly", Status: run.StatusRunning, StartedAt: time.Now(),
			})
			store.mu.Unlock()

			if err := svc.HandleRunComplete(ctx, &messagequeue.RunCompletePayload{
				RunID: "run-s1", TaskID: "task-1", ProjectID: "proj-1", Status: tc.runStatus,
			}); err != nil {
				t.Fatalf("HandleRunComplete: %v", err)
			}
			tasks, agents := statusEvents(t, bc)
			if len(tasks) != 1 || tasks[0] != (event.TaskStatusEvent{TaskID: "task-1", ProjectID: "proj-1", Status: tc.taskStatus, AgentID: "agent-1"}) {
				t.Fatalf("task.status events = %+v, want task-1 %s", tasks, tc.taskStatus)
			}
			if len(agents) != 1 || agents[0] != (event.AgentStatusEvent{AgentID: "agent-1", ProjectID: "proj-1", Status: "idle"}) {
				t.Fatalf("agent.status events = %+v, want agent-1 idle", agents)
			}
		})
	}

	// A completion that loses the run's end to another path announces nothing.
	t.Run("lost end", func(t *testing.T) {
		svc, store, _, bc := newRuntimeTestEnv()
		store.mu.Lock()
		store.runs = append(store.runs, run.Run{
			ID: "run-s2", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
			PolicyProfile: "plan-readonly", Status: run.StatusCancelled, StartedAt: time.Now(),
		})
		store.mu.Unlock()

		_ = svc.HandleRunComplete(ctx, &messagequeue.RunCompletePayload{
			RunID: "run-s2", TaskID: "task-1", ProjectID: "proj-1", Status: "completed",
		})
		if tasks, agents := statusEvents(t, bc); len(tasks)+len(agents) != 0 {
			t.Fatalf("lost end broadcast task %+v / agent %+v", tasks, agents)
		}
	})
}

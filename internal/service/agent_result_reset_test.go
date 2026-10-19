package service

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/task"
)

// TestHandleResult_ResetsTheAgent: Dispatch marks the agent running; the
// task's result left it running for good (S2 follow-up). Every result -
// completed, failed, cancelled - sets the task's agent back to idle.
func TestHandleResult_ResetsTheAgent(t *testing.T) {
	for _, reported := range []string{"completed", "failed", "cancelled"} {
		t.Run(reported, func(t *testing.T) {
			store := &mockStore{
				agents: []agent.Agent{{ID: "agent-1", ProjectID: "p1", Name: "a", Backend: "aider", Status: agent.StatusRunning}},
				tasks:  []task.Task{{ID: "t1", ProjectID: "p1", AgentID: "agent-1", Status: task.StatusQueued}},
			}
			hub := &mockBroadcaster{}
			svc := NewAgentService(store, &mockQueue{}, hub)

			if err := svc.HandleResult(context.Background(), reported, task.Result{Output: "done"}, "t1", "p1", "", 0.01); err != nil {
				t.Fatalf("HandleResult: %v", err)
			}

			if store.agents[0].Status != agent.StatusIdle {
				t.Errorf("agent status = %q, want idle", store.agents[0].Status)
			}
			idle := false
			for _, ev := range hub.events {
				if st, ok := ev.payload.(event.AgentStatusEvent); ok && ev.eventType == event.EventAgentStatus && st.AgentID == "agent-1" && st.Status == string(agent.StatusIdle) {
					idle = true
				}
			}
			if !idle {
				t.Error("no agent.status idle event was broadcast")
			}
		})
	}
}

// TestHandleResult_TaskWithoutAgent: a task dispatched without an
// assignment has no agent to reset.
func TestHandleResult_TaskWithoutAgent(t *testing.T) {
	store := &mockStore{tasks: []task.Task{{ID: "t1", ProjectID: "p1", Status: task.StatusQueued}}}
	hub := &mockBroadcaster{}
	svc := NewAgentService(store, &mockQueue{}, hub)

	if err := svc.HandleResult(context.Background(), "completed", task.Result{Output: "done"}, "t1", "p1", "", 0); err != nil {
		t.Fatalf("HandleResult: %v", err)
	}
	for _, ev := range hub.events {
		if ev.eventType == event.EventAgentStatus {
			t.Errorf("agent status broadcast for a task without agent: %+v", ev.payload)
		}
	}
}

// TestDispatchThenResult_AgentIsIdleAgain: the dispatch records the agent on
// the task (the task row did not name it), so the result finds the agent.
func TestDispatchThenResult_AgentIsIdleAgain(t *testing.T) {
	registerExecutionProbe(t)
	store := dispatchStore("/data/workspaces/proj-1")
	svc := NewAgentService(queueTaskStore{store}, &mockQueue{}, &mockBroadcaster{})

	if err := svc.Dispatch(context.Background(), "agent-1", "task-1"); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if store.agents[0].Status != agent.StatusRunning || store.tasks[0].AgentID != "agent-1" {
		t.Fatalf("after dispatch: agent %q, task agent %q; want running and agent-1", store.agents[0].Status, store.tasks[0].AgentID)
	}
	if err := svc.HandleResult(context.Background(), "completed", task.Result{Output: "done"}, "task-1", "proj-1", "", 0); err != nil {
		t.Fatalf("HandleResult: %v", err)
	}
	if store.agents[0].Status != agent.StatusIdle {
		t.Errorf("agent status after the result = %q, want idle", store.agents[0].Status)
	}
}

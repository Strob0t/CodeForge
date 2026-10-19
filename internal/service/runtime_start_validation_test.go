package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// TestStartRun_RejectsInconsistentOrUnusableRequests: runs edit the project
// workspace, so a run whose task or agent belongs to another project, or whose
// project has no workspace, is rejected as a validation error (HTTP 400)
// before anything is created or published.
func TestStartRun_RejectsInconsistentOrUnusableRequests(t *testing.T) {
	tests := []struct {
		name      string
		workspace string
		agentProj string
		taskProj  string
	}{
		{name: "task of another project", workspace: "/ws", agentProj: "proj-1", taskProj: "proj-2"},
		{name: "agent of another project", workspace: "/ws", agentProj: "proj-2", taskProj: "proj-1"},
		{name: "project without workspace", workspace: "", agentProj: "proj-1", taskProj: "proj-1"},
		{name: "blank workspace", workspace: "  ", agentProj: "proj-1", taskProj: "proj-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store, queue, _ := newRuntimeTestEnv()
			store.projects = []project.Project{
				{ID: "proj-1", Name: "one", WorkspacePath: tt.workspace},
				{ID: "proj-2", Name: "two", WorkspacePath: "/other"},
			}
			store.agents = []agent.Agent{{ID: "agent-1", ProjectID: tt.agentProj, Backend: "aider", Status: agent.StatusIdle}}
			store.tasks = []task.Task{{ID: "task-1", ProjectID: tt.taskProj, Prompt: "p", Status: task.StatusPending}}

			_, err := svc.StartRun(context.Background(), &run.StartRequest{
				TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
			})

			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("StartRun error = %v, want ErrValidation", err)
			}
			if len(store.runs) != 0 {
				t.Errorf("runs created: %d", len(store.runs))
			}
			if _, ok := queue.lastMessage(messagequeue.SubjectRunStart); ok {
				t.Error("runs.start was published")
			}
			if store.agents[0].Status != agent.StatusIdle {
				t.Errorf("agent status = %q, want idle", store.agents[0].Status)
			}
		})
	}
}

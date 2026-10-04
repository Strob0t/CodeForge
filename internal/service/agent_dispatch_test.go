package service

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/agentbackend"
)

// executionProbeBackend records the executions it is asked to run and the
// tasks it is asked to stop.
type executionProbeBackend struct {
	mu         sync.Mutex
	executions []agentbackend.Execution
	stopped    []string
}

func (b *executionProbeBackend) Name() string { return "execution-probe" }
func (b *executionProbeBackend) Capabilities() agentbackend.Capabilities {
	return agentbackend.Capabilities{}
}

func (b *executionProbeBackend) Stop(_ context.Context, taskID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stopped = append(b.stopped, taskID)
	return nil
}

func (b *executionProbeBackend) stops() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stopped
}

func (b *executionProbeBackend) Execute(_ context.Context, e *agentbackend.Execution) (*task.Result, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.executions = append(b.executions, *e)
	return nil, nil
}

func (b *executionProbeBackend) reset() []agentbackend.Execution {
	b.mu.Lock()
	defer b.mu.Unlock()
	got := b.executions
	b.executions = nil
	b.stopped = nil
	return got
}

var (
	executionProbe     = &executionProbeBackend{}
	executionProbeOnce sync.Once
)

// registerExecutionProbe registers the probe once per test binary (the backend
// registry panics on duplicate names, e.g. with -count>1).
func registerExecutionProbe(t *testing.T) *executionProbeBackend {
	t.Helper()
	executionProbeOnce.Do(func() {
		agentbackend.Register("execution-probe", func(map[string]string) (agentbackend.Backend, error) {
			return executionProbe, nil
		})
	})
	executionProbe.reset()
	return executionProbe
}

func dispatchStore(workspace string) *mockStore {
	return &mockStore{
		projects: []project.Project{{ID: "proj-1", Name: "p", WorkspacePath: workspace}},
		agents:   []agent.Agent{{ID: "agent-1", ProjectID: "proj-1", Name: "a", Backend: "execution-probe", Status: agent.StatusIdle}},
		tasks:    []task.Task{{ID: "task-1", ProjectID: "proj-1", Title: "t", Prompt: "p", Status: task.StatusPending}},
	}
}

// TestAgentDispatch_SendsProjectWorkspace: the backend works in the project
// workspace (KI-23); before, it got no workspace and the worker ran the agent
// CLI in its own working directory.
func TestAgentDispatch_SendsProjectWorkspace(t *testing.T) {
	probe := registerExecutionProbe(t)
	svc := NewAgentService(dispatchStore("/data/workspaces/proj-1"), &mockQueue{}, &mockBroadcaster{})

	if err := svc.Dispatch(context.Background(), "agent-1", "task-1"); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	got := probe.reset()
	if len(got) != 1 {
		t.Fatalf("backend executions = %d, want 1", len(got))
	}
	if got[0].WorkspacePath != "/data/workspaces/proj-1" {
		t.Errorf("workspace = %q, want the project workspace", got[0].WorkspacePath)
	}
	if got[0].Task == nil || got[0].Task.ID != "task-1" {
		t.Errorf("task = %+v, want task-1", got[0].Task)
	}
}

// TestAgentDispatch_ProjectWithoutWorkspaceFails: a backend must not run
// without a workspace (it would edit the worker's own directory); the dispatch
// fails as a validation error and leaves agent and task untouched.
func TestAgentDispatch_ProjectWithoutWorkspaceFails(t *testing.T) {
	for _, workspace := range []string{"", "   "} {
		t.Run("workspace="+workspace, func(t *testing.T) {
			probe := registerExecutionProbe(t)
			store := dispatchStore(workspace)
			svc := NewAgentService(store, &mockQueue{}, &mockBroadcaster{})

			err := svc.Dispatch(context.Background(), "agent-1", "task-1")
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("Dispatch error = %v, want ErrValidation", err)
			}
			if got := probe.reset(); len(got) != 0 {
				t.Fatalf("backend was asked to execute %d task(s)", len(got))
			}
			if store.agents[0].Status != agent.StatusIdle {
				t.Errorf("agent status = %q, want idle", store.agents[0].Status)
			}
		})
	}
}

// TestAgentDispatch_TaskOfAnotherProjectFails: the backend works in the
// agent's project workspace, so a task of another project must not run there.
func TestAgentDispatch_TaskOfAnotherProjectFails(t *testing.T) {
	probe := registerExecutionProbe(t)
	store := dispatchStore("/data/workspaces/proj-1")
	store.projects = append(store.projects, project.Project{ID: "proj-2", Name: "q", WorkspacePath: "/data/workspaces/proj-2"})
	store.tasks[0].ProjectID = "proj-2"
	svc := NewAgentService(store, &mockQueue{}, &mockBroadcaster{})

	err := svc.Dispatch(context.Background(), "agent-1", "task-1")

	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("Dispatch error = %v, want ErrValidation", err)
	}
	if got := probe.reset(); len(got) != 0 {
		t.Fatalf("backend was asked to execute %d task(s)", len(got))
	}
	if store.agents[0].Status != agent.StatusIdle {
		t.Errorf("agent status = %q, want idle", store.agents[0].Status)
	}
}

// TestAgentStopTask_OnlyStopsTheCallersTask: tasks.cancel reaches every
// worker, so StopTask must not publish it for a task outside the caller's
// tenant (the tenant-scoped store does not find it) or the agent's project.
func TestAgentStopTask_OnlyStopsTheCallersTask(t *testing.T) {
	tests := []struct {
		name      string
		tasks     []task.Task
		wantErr   error
		wantStops []string
	}{
		{name: "own task", tasks: []task.Task{{ID: "task-1", ProjectID: "proj-1", Status: task.StatusRunning}}, wantStops: []string{"task-1"}},
		{name: "task of another tenant", tasks: nil, wantErr: domain.ErrNotFound},
		{name: "task of another project", tasks: []task.Task{{ID: "task-1", ProjectID: "proj-2", Status: task.StatusRunning}}, wantErr: domain.ErrValidation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probe := registerExecutionProbe(t)
			store := dispatchStore("/data/workspaces/proj-1")
			store.tasks = tt.tasks
			svc := NewAgentService(store, &mockQueue{}, &mockBroadcaster{})

			err := svc.StopTask(context.Background(), "agent-1", "task-1")

			if tt.wantErr == nil && err != nil {
				t.Fatalf("StopTask: %v", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("StopTask error = %v, want %v", err, tt.wantErr)
			}
			if got := probe.stops(); !slices.Equal(got, tt.wantStops) {
				t.Errorf("backend stopped %v, want %v", got, tt.wantStops)
			}
		})
	}
}

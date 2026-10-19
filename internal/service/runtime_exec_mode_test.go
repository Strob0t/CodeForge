package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/resource"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// recordingSandbox records every container operation so tests can assert
// that no container is created for rejected runs.
type recordingSandbox struct {
	mu    sync.Mutex
	calls []string
}

func (s *recordingSandbox) record(call string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, call)
}

func (s *recordingSandbox) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *recordingSandbox) Create(_ context.Context, runID, workDir string, _ ...resource.Limits) (*service.Sandbox, error) {
	s.record("create")
	return &service.Sandbox{ContainerID: "fake", RunID: runID, WorkDir: workDir}, nil
}

func (s *recordingSandbox) CreateHybrid(_ context.Context, runID, workDir string, _ ...resource.Limits) (*service.Sandbox, error) {
	s.record("create-hybrid")
	return &service.Sandbox{ContainerID: "fake", RunID: runID, WorkDir: workDir}, nil
}

func (s *recordingSandbox) Start(_ context.Context, _ string) error {
	s.record("start")
	return nil
}

func (s *recordingSandbox) Stop(_ context.Context, _ string) error {
	s.record("stop")
	return nil
}

func (s *recordingSandbox) Remove(_ context.Context, _ string) error {
	s.record("remove")
	return nil
}

func (s *recordingSandbox) Get(_ string) (*service.Sandbox, bool) { return nil, false }

func TestStartRun_RejectsExecModesWithoutIsolation(t *testing.T) {
	tests := []struct {
		name          string
		requested     run.ExecMode
		projectConfig map[string]string
		wantErr       error
	}{
		{name: "explicit sandbox", requested: run.ExecModeSandbox, wantErr: run.ErrExecModeUnavailable},
		{name: "explicit hybrid", requested: run.ExecModeHybrid, wantErr: run.ErrExecModeUnavailable},
		{
			name:          "explicit sandbox overrides project mount",
			requested:     run.ExecModeSandbox,
			projectConfig: map[string]string{"execution_mode": "mount"},
			wantErr:       run.ErrExecModeUnavailable,
		},
		{name: "explicit mixed case", requested: "Sandbox", wantErr: domain.ErrValidation},
		{name: "explicit with whitespace", requested: " sandbox", wantErr: domain.ErrValidation},
		{
			name:          "project default sandbox",
			projectConfig: map[string]string{"execution_mode": "sandbox"},
			wantErr:       run.ErrExecModeUnavailable,
		},
		{
			name:          "project default hybrid",
			projectConfig: map[string]string{"execution_mode": "hybrid"},
			wantErr:       run.ErrExecModeUnavailable,
		},
		{
			name:          "project default mixed case fails closed",
			projectConfig: map[string]string{"execution_mode": "Sandbox"},
			wantErr:       domain.ErrValidation,
		},
		{
			name:          "project default with whitespace fails closed",
			projectConfig: map[string]string{"execution_mode": " sandbox "},
			wantErr:       domain.ErrValidation,
		},
		{
			name:          "project default unknown value fails closed",
			projectConfig: map[string]string{"execution_mode": "docker"},
			wantErr:       domain.ErrValidation,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, store, queue, bc := newRuntimeTestEnv()
			store.projects[0].Config = tc.projectConfig
			sandbox := &recordingSandbox{}
			svc.SetSandboxService(sandbox)

			r, err := svc.StartRun(context.Background(), &run.StartRequest{
				TaskID:    "task-1",
				AgentID:   "agent-1",
				ProjectID: "proj-1",
				ExecMode:  tc.requested,
			})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("StartRun error = %v, want %v", err, tc.wantErr)
			}
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("StartRun error = %v, want it to wrap domain.ErrValidation (HTTP 400)", err)
			}
			if r != nil {
				t.Fatalf("StartRun returned run %+v, want nil", r)
			}
			if n := sandbox.callCount(); n != 0 {
				t.Fatalf("sandbox received %d calls (%v), want none", n, sandbox.calls)
			}
			if len(store.runs) != 0 {
				t.Fatalf("store has %d runs, want none", len(store.runs))
			}
			if len(queue.messages) != 0 {
				t.Fatalf("queue has %d messages, want none", len(queue.messages))
			}
			if len(bc.events) != 0 {
				t.Fatalf("broadcaster has %d events, want none", len(bc.events))
			}
		})
	}
}

func TestStartRun_ResolvesMountExecMode(t *testing.T) {
	tests := []struct {
		name          string
		requested     run.ExecMode
		projectConfig map[string]string
	}{
		{name: "no request and no project config"},
		{name: "empty project config value", projectConfig: map[string]string{"execution_mode": ""}},
		{name: "project default mount", projectConfig: map[string]string{"execution_mode": "mount"}},
		{name: "explicit mount", requested: run.ExecModeMount},
		{
			name:          "explicit mount overrides project sandbox",
			requested:     run.ExecModeMount,
			projectConfig: map[string]string{"execution_mode": "sandbox"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, store, queue, _ := newRuntimeTestEnv()
			store.projects[0].Config = tc.projectConfig
			sandbox := &recordingSandbox{}
			svc.SetSandboxService(sandbox)

			r, err := svc.StartRun(context.Background(), &run.StartRequest{
				TaskID:    "task-1",
				AgentID:   "agent-1",
				ProjectID: "proj-1",
				ExecMode:  tc.requested,
			})
			if err != nil {
				t.Fatalf("StartRun: %v", err)
			}
			if r.ExecMode != run.ExecModeMount {
				t.Fatalf("run exec_mode = %q, want mount", r.ExecMode)
			}
			msg, ok := queue.lastMessage(messagequeue.SubjectRunStart)
			if !ok {
				t.Fatal("expected a run start message")
			}
			var payload messagequeue.RunStartPayload
			if err := json.Unmarshal(msg.Data, &payload); err != nil {
				t.Fatalf("unmarshal run start payload: %v", err)
			}
			if payload.ExecMode != string(run.ExecModeMount) {
				t.Fatalf("payload exec_mode = %q, want mount", payload.ExecMode)
			}
			if n := sandbox.callCount(); n != 0 {
				t.Fatalf("sandbox received %d calls (%v), want none for mount", n, sandbox.calls)
			}
		})
	}
}

func TestStartRun_UnknownProject(t *testing.T) {
	svc, store, queue, _ := newRuntimeTestEnv()

	_, err := svc.StartRun(context.Background(), &run.StartRequest{
		TaskID:    "task-1",
		AgentID:   "agent-1",
		ProjectID: "missing-project",
	})
	if err == nil {
		t.Fatal("expected an error for an unknown project")
	}
	if len(store.runs) != 0 || len(queue.messages) != 0 {
		t.Fatalf("expected no run and no message, got %d runs and %d messages", len(store.runs), len(queue.messages))
	}
}

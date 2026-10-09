package agentbackend_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Strob0t/CodeForge/internal/adapter/aider"
	"github.com/Strob0t/CodeForge/internal/adapter/goose"
	"github.com/Strob0t/CodeForge/internal/adapter/opencode"
	"github.com/Strob0t/CodeForge/internal/adapter/openhands"
	"github.com/Strob0t/CodeForge/internal/adapter/plandex"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/agentbackend"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

type recordingQueue struct {
	subjects []string
	payloads [][]byte
}

func (q *recordingQueue) Publish(_ context.Context, subject string, data []byte) error {
	q.subjects = append(q.subjects, subject)
	q.payloads = append(q.payloads, data)
	return nil
}

func (q *recordingQueue) PublishWithDedup(ctx context.Context, subject string, data []byte, _ string) error {
	return q.Publish(ctx, subject, data)
}

func (q *recordingQueue) Subscribe(context.Context, string, messagequeue.Handler) (func(), error) {
	return func() {}, nil
}
func (q *recordingQueue) Drain() error      { return nil }
func (q *recordingQueue) Close() error      { return nil }
func (q *recordingQueue) IsConnected() bool { return true }

// natsBackends are the backends that run in the Python worker.
func natsBackends() map[string]func(messagequeue.Queue) agentbackend.Backend {
	return map[string]func(messagequeue.Queue) agentbackend.Backend{
		"aider":     func(q messagequeue.Queue) agentbackend.Backend { return aider.New(q) },
		"goose":     func(q messagequeue.Queue) agentbackend.Backend { return goose.New(q) },
		"opencode":  func(q messagequeue.Queue) agentbackend.Backend { return opencode.New(q) },
		"openhands": func(q messagequeue.Queue) agentbackend.Backend { return openhands.New(q) },
		"plandex":   func(q messagequeue.Queue) agentbackend.Backend { return plandex.New(q) },
	}
}

// TestNATSBackends_ExecutePublishesTaskAgentPayload: every worker backend
// receives the task with the backend name and the project workspace it works
// in (KI-23); before, the worker got the domain task and ran every backend
// with workspace_path "".
func TestNATSBackends_ExecutePublishesTaskAgentPayload(t *testing.T) {
	for name, newBackend := range natsBackends() {
		t.Run(name, func(t *testing.T) {
			q := &recordingQueue{}
			tsk := &task.Task{
				ID: "task-1", TenantID: "tenant-1", ProjectID: "proj-1", AgentID: "agent-1",
				Title: "Fix bug", Prompt: "fix the null pointer", Status: task.StatusQueued, DispatchID: "dispatch-1",
			}

			result, err := newBackend(q).Execute(context.Background(), &agentbackend.Execution{
				Task:             tsk,
				WorkspacePath:    "/data/workspaces/proj-1",
				HeartbeatSeconds: 10,
			})
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if result != nil {
				t.Fatal("expected nil result: execution is asynchronous")
			}
			if len(q.subjects) != 1 || q.subjects[0] != messagequeue.SubjectTaskAgent+"."+name {
				t.Fatalf("published to %v, want [%s.%s]", q.subjects, messagequeue.SubjectTaskAgent, name)
			}

			var got messagequeue.TaskAgentPayload
			if err := json.Unmarshal(q.payloads[0], &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			want := messagequeue.TaskAgentPayload{
				TaskID: "task-1", TenantID: "tenant-1", ProjectID: "proj-1", AgentID: "agent-1",
				Title: "Fix bug", Prompt: "fix the null pointer",
				Backend: name, WorkspacePath: "/data/workspaces/proj-1", HeartbeatSeconds: 10, DispatchID: "dispatch-1",
			}
			if got != want {
				t.Fatalf("payload = %+v\nwant      %+v", got, want)
			}
		})
	}
}

// TestNATSBackends_StopPublishesCancel: Stop asks the worker to cancel the task.
func TestNATSBackends_StopPublishesCancel(t *testing.T) {
	for name, newBackend := range natsBackends() {
		t.Run(name, func(t *testing.T) {
			q := &recordingQueue{}
			if err := newBackend(q).Stop(context.Background(), "task-1"); err != nil {
				t.Fatalf("Stop: %v", err)
			}
			if len(q.subjects) != 1 || q.subjects[0] != messagequeue.SubjectTaskCancel {
				t.Fatalf("published to %v, want [%s]", q.subjects, messagequeue.SubjectTaskCancel)
			}
			var got messagequeue.TaskCancelPayload
			if err := json.Unmarshal(q.payloads[0], &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got.TaskID != "task-1" {
				t.Fatalf("task_id = %q, want task-1", got.TaskID)
			}
		})
	}
}

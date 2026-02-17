package aider_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Strob0t/CodeForge/internal/adapter/aider"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

type mockQueue struct {
	published []publishedMsg
}

type publishedMsg struct {
	subject string
	data    []byte
}

func (m *mockQueue) Publish(_ context.Context, subject string, data []byte) error {
	m.published = append(m.published, publishedMsg{subject: subject, data: data})
	return nil
}

func (m *mockQueue) Subscribe(_ context.Context, _ string, _ messagequeue.Handler) (func(), error) {
	return func() {}, nil
}

func (m *mockQueue) Drain() error      { return nil }
func (m *mockQueue) Close() error      { return nil }
func (m *mockQueue) IsConnected() bool { return true }

func TestBackendName(t *testing.T) {
	q := &mockQueue{}
	b := aider.New(q)

	if b.Name() != "aider" {
		t.Fatalf("expected name 'aider', got %q", b.Name())
	}
}

func TestBackendCapabilities(t *testing.T) {
	q := &mockQueue{}
	b := aider.New(q)

	caps := b.Capabilities()
	if !caps.Edit {
		t.Fatal("expected Edit capability")
	}
	if !caps.Planner {
		t.Fatal("expected Planner capability")
	}
	if caps.Terminal {
		t.Fatal("unexpected Terminal capability")
	}
}

func TestExecutePublishesTask(t *testing.T) {
	q := &mockQueue{}
	b := aider.New(q)

	tsk := &task.Task{
		ID:        "task-1",
		ProjectID: "proj-1",
		Title:     "fix bug",
		Prompt:    "fix the null pointer",
		Status:    task.StatusPending,
	}

	result, err := b.Execute(context.Background(), tsk)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != nil {
		t.Fatal("expected nil result for async dispatch")
	}

	if len(q.published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(q.published))
	}

	msg := q.published[0]
	if msg.subject != "tasks.agent.aider" {
		t.Fatalf("expected subject 'tasks.agent.aider', got %q", msg.subject)
	}

	var published task.Task
	if err := json.Unmarshal(msg.data, &published); err != nil {
		t.Fatalf("unmarshal published data: %v", err)
	}
	if published.ID != "task-1" {
		t.Fatalf("expected task ID 'task-1', got %q", published.ID)
	}
}

func TestStopPublishesCancel(t *testing.T) {
	q := &mockQueue{}
	b := aider.New(q)

	if err := b.Stop(context.Background(), "task-1"); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if len(q.published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(q.published))
	}

	msg := q.published[0]
	if msg.subject != "tasks.cancel" {
		t.Fatalf("expected subject 'tasks.cancel', got %q", msg.subject)
	}
}

func TestRegister(t *testing.T) {
	// Register is called once; just verify it doesn't panic.
	// We can't test it properly here because the registry doesn't allow
	// deregistration, and it may already be registered from another test.
}

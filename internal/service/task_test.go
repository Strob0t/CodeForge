package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// mockQueue implements messagequeue.Queue for testing.
type mockQueue struct {
	published []struct {
		subject string
		data    []byte
	}
	publishErr error
}

func (q *mockQueue) Publish(_ context.Context, subject string, data []byte) error {
	if q.publishErr != nil {
		return q.publishErr
	}
	q.published = append(q.published, struct {
		subject string
		data    []byte
	}{subject, data})
	return nil
}

func (q *mockQueue) Subscribe(_ context.Context, _ string, _ messagequeue.Handler) (func(), error) {
	return func() {}, nil
}

func (q *mockQueue) Drain() error      { return nil }
func (q *mockQueue) Close() error      { return nil }
func (q *mockQueue) IsConnected() bool { return true }

// --- TaskService Tests ---

func TestTaskServiceList(t *testing.T) {
	store := &mockStore{
		tasks: []task.Task{
			{ID: "t1", ProjectID: "p1", Title: "Task 1"},
			{ID: "t2", ProjectID: "p1", Title: "Task 2"},
		},
	}
	svc := NewTaskService(store, &mockQueue{})

	got, err := svc.List(context.Background(), "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(got))
	}
}

func TestTaskServiceGet(t *testing.T) {
	store := &mockStore{
		tasks: []task.Task{{ID: "t1", Title: "Fix bug"}},
	}
	svc := NewTaskService(store, &mockQueue{})

	got, err := svc.Get(context.Background(), "t1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Title != "Fix bug" {
		t.Fatalf("expected 'Fix bug', got %q", got.Title)
	}
}

func TestTaskServiceGetNotFound(t *testing.T) {
	svc := NewTaskService(&mockStore{}, &mockQueue{})

	_, err := svc.Get(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestTaskServiceCreate(t *testing.T) {
	queue := &mockQueue{}
	svc := NewTaskService(&mockStore{}, queue)

	req := task.CreateRequest{ProjectID: "p1", Title: "New Task", Prompt: "Do something"}
	got, err := svc.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Title != "New Task" {
		t.Fatalf("expected 'New Task', got %q", got.Title)
	}
	if got.Status != task.StatusPending {
		t.Fatalf("expected status 'pending', got %q", got.Status)
	}

	// Verify NATS publish was called
	if len(queue.published) != 1 {
		t.Fatalf("expected 1 publish call, got %d", len(queue.published))
	}
	if queue.published[0].subject != messagequeue.SubjectTaskCreated {
		t.Fatalf("expected subject %q, got %q", messagequeue.SubjectTaskCreated, queue.published[0].subject)
	}
}

func TestTaskServiceCreatePublishFailure(t *testing.T) {
	// Even if queue publish fails, the task should still be returned (saved in DB).
	queue := &mockQueue{publishErr: errors.New("nats down")}
	svc := NewTaskService(&mockStore{}, queue)

	req := task.CreateRequest{ProjectID: "p1", Title: "Resilient Task"}
	got, err := svc.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Title != "Resilient Task" {
		t.Fatalf("expected 'Resilient Task', got %q", got.Title)
	}
}

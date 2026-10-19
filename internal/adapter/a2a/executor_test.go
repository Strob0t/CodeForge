package a2a

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	sdka2a "github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"

	"github.com/Strob0t/CodeForge/internal/domain"
	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const execTestTenant = "00000000-0000-0000-0000-00000000a2a0"

// tenantCtx is the context of an inbound request in execTestTenant (the HTTP
// tenant middleware sets it for every request).
func tenantCtx() context.Context {
	return tenantctx.WithTenant(context.Background(), execTestTenant)
}

// --- Minimal fakes for testing ---

// fakeStore implements only the A2A-related Store methods used by Executor.
type fakeStore struct {
	database.Store
	tasks     map[string]*a2adomain.A2ATask
	createErr error
	getErr    error
	updateErr error
}

func newFakeStore() *fakeStore {
	return &fakeStore{tasks: make(map[string]*a2adomain.A2ATask)}
}

func (s *fakeStore) CreateA2ATask(_ context.Context, t *a2adomain.A2ATask) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.tasks[t.ID] = t
	return nil
}

func (s *fakeStore) GetA2ATask(_ context.Context, id string) (*a2adomain.A2ATask, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	t, ok := s.tasks[id]
	if !ok {
		return nil, errors.New("task not found")
	}
	return t, nil
}

func (s *fakeStore) UpdateA2ATask(_ context.Context, t *a2adomain.A2ATask) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	s.tasks[t.ID] = t
	return nil
}

// fakeQueue implements messagequeue.Queue for testing (no-op).
type fakeQueue struct{}

func (fakeQueue) Publish(context.Context, string, []byte) error                  { return nil }
func (fakeQueue) PublishWithDedup(context.Context, string, []byte, string) error { return nil }
func (fakeQueue) Subscribe(context.Context, string, messagequeue.Handler) (func(), error) {
	return func() {}, nil
}
func (fakeQueue) Drain() error      { return nil }
func (fakeQueue) Close() error      { return nil }
func (fakeQueue) IsConnected() bool { return true }

// fakeBroadcaster implements broadcast.Broadcaster for testing (no-op).
type fakeBroadcaster struct{}

func (fakeBroadcaster) BroadcastEvent(context.Context, string, any) {}

// fakeEventQueue implements eventqueue.Queue for testing (no-op).
type fakeEventQueue struct{}

func (fakeEventQueue) Read(context.Context) (sdka2a.Event, sdka2a.TaskVersion, error) {
	return nil, 0, nil
}
func (fakeEventQueue) Write(context.Context, sdka2a.Event) error { return nil }
func (fakeEventQueue) WriteVersioned(context.Context, sdka2a.Event, sdka2a.TaskVersion) error {
	return nil
}
func (fakeEventQueue) Close() error { return nil }

// --- Tests ---

func TestExecutor_Execute_PromptTooLong(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	exec := NewExecutor(store, fakeQueue{}, fakeBroadcaster{}, nil)

	// Build a prompt that exceeds MaxPromptLength.
	longPrompt := strings.Repeat("x", MaxPromptLength+1)

	reqCtx := &a2asrv.RequestContext{
		TaskID: "test-task-1",
		Message: &sdka2a.Message{
			Role: sdka2a.MessageRoleUser,
			Parts: []sdka2a.Part{
				sdka2a.TextPart{Text: longPrompt},
			},
		},
	}

	err := exec.Execute(tenantCtx(), reqCtx, fakeEventQueue{})
	if err == nil {
		t.Fatal("Execute() should return error for prompt exceeding MaxPromptLength")
	}
	if !strings.Contains(err.Error(), "prompt exceeds maximum length") {
		t.Errorf("error = %q, want it to contain 'prompt exceeds maximum length'", err.Error())
	}

	// Verify no task was created in the store.
	if len(store.tasks) != 0 {
		t.Errorf("expected no tasks in store, got %d", len(store.tasks))
	}
}

func TestExecutor_Execute_PromptAtLimit(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	exec := NewExecutor(store, fakeQueue{}, fakeBroadcaster{}, nil)

	// Build a prompt exactly at MaxPromptLength (should succeed).
	exactPrompt := strings.Repeat("y", MaxPromptLength)

	reqCtx := &a2asrv.RequestContext{
		TaskID: "test-task-2",
		Message: &sdka2a.Message{
			Role: sdka2a.MessageRoleUser,
			Parts: []sdka2a.Part{
				sdka2a.TextPart{Text: exactPrompt},
			},
		},
	}

	err := exec.Execute(tenantCtx(), reqCtx, fakeEventQueue{})
	if err != nil {
		t.Fatalf("Execute() should succeed for prompt at MaxPromptLength, got: %v", err)
	}

	// Verify the task was created.
	if len(store.tasks) != 1 {
		t.Errorf("expected 1 task in store, got %d", len(store.tasks))
	}
}

func TestExecutor_Cancel_TaskNotFound(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	exec := NewExecutor(store, fakeQueue{}, fakeBroadcaster{}, nil)

	reqCtx := &a2asrv.RequestContext{
		TaskID: "nonexistent-task",
	}

	err := exec.Cancel(tenantCtx(), reqCtx, fakeEventQueue{})
	if err == nil {
		t.Fatal("Cancel() should return error for nonexistent task")
	}
	if !strings.Contains(err.Error(), "get a2a task for cancel") {
		t.Errorf("error = %q, want it to contain 'get a2a task for cancel'", err.Error())
	}
}

func TestExecutor_Cancel_StoreError(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.getErr = errors.New("database connection lost")
	exec := NewExecutor(store, fakeQueue{}, fakeBroadcaster{}, nil)

	reqCtx := &a2asrv.RequestContext{
		TaskID: "some-task",
	}

	err := exec.Cancel(tenantCtx(), reqCtx, fakeEventQueue{})
	if err == nil {
		t.Fatal("Cancel() should return error when store fails")
	}
	if !strings.Contains(err.Error(), "database connection lost") {
		t.Errorf("error = %q, want it to wrap the store error", err.Error())
	}
}

// Follow-up of KI-64: the executor used tenantctx.FromContext, which falls back
// to the default tenant. An inbound task now needs the request's tenant and
// carries it on a2a.task.created; without one nothing is created or published.
func TestExecutor_TenantIsRequired(t *testing.T) {
	t.Parallel()
	reqCtx := &a2asrv.RequestContext{
		TaskID:  "tenant-task",
		Message: &sdka2a.Message{Role: sdka2a.MessageRoleUser, Parts: []sdka2a.Part{sdka2a.TextPart{Text: "do it"}}},
	}

	t.Run("execute without a tenant", func(t *testing.T) {
		t.Parallel()
		store := newFakeStore()
		queue := &recordingQueue{}
		err := NewExecutor(store, queue, fakeBroadcaster{}, nil).Execute(context.Background(), reqCtx, fakeEventQueue{})
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("Execute error = %v, want ErrValidation", err)
		}
		if len(store.tasks) != 0 || len(queue.published) != 0 {
			t.Fatalf("created %d tasks and published %d messages without a tenant", len(store.tasks), len(queue.published))
		}
	})

	t.Run("execute carries the tenant", func(t *testing.T) {
		t.Parallel()
		queue := &recordingQueue{}
		if err := NewExecutor(newFakeStore(), queue, fakeBroadcaster{}, nil).Execute(tenantCtx(), reqCtx, fakeEventQueue{}); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		var payload messagequeue.A2ATaskCreatedPayload
		for _, p := range queue.published {
			if p.subject == messagequeue.SubjectA2ATaskCreated {
				if err := json.Unmarshal(p.data, &payload); err != nil {
					t.Fatalf("a2a.task.created payload: %v", err)
				}
			}
		}
		if payload.TenantID != execTestTenant {
			t.Fatalf("tenant_id = %q, want %q", payload.TenantID, execTestTenant)
		}
	})

	t.Run("cancel without a tenant", func(t *testing.T) {
		t.Parallel()
		store := newFakeStore()
		store.tasks["a2a-tenant-task"] = &a2adomain.A2ATask{ID: "a2a-tenant-task", State: a2adomain.TaskStateWorking}
		err := NewExecutor(store, fakeQueue{}, fakeBroadcaster{}, nil).Cancel(context.Background(), reqCtx, fakeEventQueue{})
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("Cancel error = %v, want ErrValidation", err)
		}
		if store.tasks["a2a-tenant-task"].State != a2adomain.TaskStateWorking {
			t.Fatal("task canceled without a tenant")
		}
	})
}

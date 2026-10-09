package a2a

import (
	"context"
	"errors"
	"testing"

	sdka2a "github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"

	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// Every A2A key of a tenant reached all of the tenant's A2A tasks through
// the protocol handler (S2-G fix, V1): it could list them in both
// directions with full history and artifacts, and cancel other callers'
// inbound tasks. The protocol handler now sees only the inbound tasks the
// calling key created.

// listingStore filters A2A tasks like the store does.
type listingStore struct {
	*fakeStore
	lastFilter *database.A2ATaskFilter
}

func (s *listingStore) ListA2ATasks(_ context.Context, filter *database.A2ATaskFilter) ([]a2adomain.A2ATask, int, error) {
	s.lastFilter = filter
	var out []a2adomain.A2ATask
	for _, t := range s.tasks {
		if filter.Direction != "" && string(t.Direction) != filter.Direction {
			continue
		}
		if filter.CallerKeyID != "" && t.CallerKeyID != filter.CallerKeyID {
			continue
		}
		out = append(out, *t)
	}
	return out, len(out), nil
}

// callerCtx is an inbound request of the A2A key keyID in the tenant the
// HTTP tenant middleware sets on every request.
func callerCtx(keyID string) context.Context {
	return middleware.ContextWithA2ACaller(tenantctx.WithTenant(context.Background(), screenTenant), keyID)
}

func newCallerStore() *listingStore {
	store := &listingStore{fakeStore: newFakeStore()}
	add := func(id string, direction a2adomain.Direction, caller string) {
		t := a2adomain.NewA2ATask(id)
		t.Direction = direction
		t.CallerKeyID = caller
		store.tasks[id] = t
	}
	add("task-of-a", a2adomain.DirectionInbound, "key-a")
	add("task-of-b", a2adomain.DirectionInbound, "key-b")
	add("outbound", a2adomain.DirectionOutbound, "")
	add("legacy", a2adomain.DirectionInbound, "") // created before tasks recorded their key
	return store
}

func TestTaskStoreAdapter_SeesOnlyTheCallersInboundTasks(t *testing.T) {
	store := newCallerStore()
	adapter := NewTaskStoreAdapter(store)

	if task, _, err := adapter.Get(callerCtx("key-a"), "task-of-a"); err != nil || task.ID != "task-of-a" {
		t.Fatalf("Get(own task) = %+v, %v", task, err)
	}
	for _, id := range []sdka2a.TaskID{"task-of-b", "outbound", "legacy"} {
		if _, _, err := adapter.Get(callerCtx("key-a"), id); !errors.Is(err, sdka2a.ErrTaskNotFound) {
			t.Errorf("Get(%s) as key-a = %v, want ErrTaskNotFound", id, err)
		}
	}

	resp, err := adapter.List(callerCtx("key-a"), &sdka2a.ListTasksRequest{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(resp.Tasks) != 1 || resp.Tasks[0].ID != "task-of-a" {
		t.Fatalf("List as key-a = %+v, want only task-of-a", resp.Tasks)
	}
	if store.lastFilter.Direction != string(a2adomain.DirectionInbound) || store.lastFilter.CallerKeyID != "key-a" {
		t.Errorf("list filter = %+v, want key-a's inbound tasks", store.lastFilter)
	}

	// Without a caller (no A2A key in the context) nothing is visible.
	if resp, err := adapter.List(context.Background(), &sdka2a.ListTasksRequest{}); err != nil || len(resp.Tasks) != 0 {
		t.Fatalf("List without caller = %+v, %v; want nothing", resp, err)
	}
	if _, _, err := adapter.Get(context.Background(), "legacy"); !errors.Is(err, sdka2a.ErrTaskNotFound) {
		t.Fatalf("Get without caller = %v, want ErrTaskNotFound", err)
	}
}

func TestTaskStoreAdapter_SaveRecordsTheCaller(t *testing.T) {
	store := newCallerStore()
	adapter := NewTaskStoreAdapter(store)
	task := &sdka2a.Task{ID: "new-task", Status: sdka2a.TaskStatus{State: sdka2a.TaskStateSubmitted}}
	if _, err := adapter.Save(callerCtx("key-b"), task, nil, nil, sdka2a.TaskVersionMissing); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := store.tasks["new-task"]; got == nil || got.CallerKeyID != "key-b" {
		t.Fatalf("saved task = %+v, want caller key-b", got)
	}
}

func TestExecutor_RecordsTheCallerAndCancelsOnlyItsTasks(t *testing.T) {
	store := newCallerStore()
	exec := NewExecutor(store, fakeQueue{}, fakeBroadcaster{}, nil)
	reqCtx := &a2asrv.RequestContext{
		TaskID:  "mine",
		Message: &sdka2a.Message{Role: sdka2a.MessageRoleUser, Parts: []sdka2a.Part{sdka2a.TextPart{Text: "hello"}}},
	}
	if err := exec.Execute(callerCtx("key-a"), reqCtx, fakeEventQueue{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := store.tasks["a2a-mine"]; got == nil || got.CallerKeyID != "key-a" {
		t.Fatalf("created task = %+v, want caller key-a", got)
	}

	// key-b cannot cancel key-a's task.
	if err := exec.Cancel(callerCtx("key-b"), &a2asrv.RequestContext{TaskID: "mine"}, fakeEventQueue{}); err == nil {
		t.Fatal("key-b cancelled key-a's task")
	}
	if got := store.tasks["a2a-mine"]; got.State == a2adomain.TaskStateCanceled {
		t.Fatal("key-a's task was cancelled by key-b")
	}
	if err := exec.Cancel(callerCtx("key-a"), &a2asrv.RequestContext{TaskID: "mine"}, fakeEventQueue{}); err != nil {
		t.Fatalf("Cancel(own task): %v", err)
	}
	if got := store.tasks["a2a-mine"]; got.State != a2adomain.TaskStateCanceled {
		t.Fatalf("own task = %s, want canceled", got.State)
	}
}

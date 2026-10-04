package a2a

import (
	"context"
	"errors"
	"fmt"
	"testing"

	sdka2a "github.com/a2aproject/a2a-go/a2a"

	"github.com/Strob0t/CodeForge/internal/domain"
	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
)

// versionedStore updates a task like the PostgreSQL store: only while the
// stored version equals the task's version, then increments it.
type versionedStore struct {
	*fakeStore
}

func (s *versionedStore) GetA2ATask(_ context.Context, id string) (*a2adomain.A2ATask, error) {
	t, ok := s.tasks[id]
	if !ok {
		return nil, fmt.Errorf("a2a task %s: %w", id, domain.ErrNotFound)
	}
	c := *t
	return &c, nil
}

func (s *versionedStore) UpdateA2ATask(_ context.Context, t *a2adomain.A2ATask) error {
	stored, ok := s.tasks[t.ID]
	if !ok || stored.Version != t.Version {
		return fmt.Errorf("update a2a task %s: %w", t.ID, domain.ErrConflict)
	}
	t.Version++
	c := *t
	c.CallerKeyID = stored.CallerKeyID
	s.tasks[t.ID] = &c
	return nil
}

// The SDK saves a task again with the version it read; the adapter passed
// that version plus one, which the store's optimistic lock never matched,
// so every update after the first save failed with a conflict.
func TestTaskStoreAdapter_UpdatesWithTheVersionTheSDKRead(t *testing.T) {
	store := &versionedStore{fakeStore: newFakeStore()}
	adapter := NewTaskStoreAdapter(store)
	ctx := callerCtx("key-a")
	task := &sdka2a.Task{ID: "t-1", ContextID: "c-1", Status: sdka2a.TaskStatus{State: sdka2a.TaskStateSubmitted}}

	v1, err := adapter.Save(ctx, task, nil, nil, sdka2a.TaskVersionMissing)
	if err != nil || v1 != 1 {
		t.Fatalf("create = %d, %v", v1, err)
	}
	task.Status.State = sdka2a.TaskStateWorking
	v2, err := adapter.Save(ctx, task, nil, nil, v1)
	if err != nil || v2 != 2 {
		t.Fatalf("first update = %d, %v, want version 2", v2, err)
	}
	task.Status.State = sdka2a.TaskStateCompleted
	if v3, err := adapter.Save(ctx, task, nil, nil, v2); err != nil || v3 != 3 {
		t.Fatalf("second update = %d, %v, want version 3", v3, err)
	}
	if _, err := adapter.Save(ctx, task, nil, nil, v1); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale update = %v, want ErrConflict", err)
	}
	got, version, err := adapter.Get(ctx, "t-1")
	if err != nil || version != 3 || got.Status.State != sdka2a.TaskStateCompleted {
		t.Fatalf("Get = %+v, %d, %v", got, version, err)
	}
}

func TestTaskStoreAdapter_MissingTaskIsTaskNotFound(t *testing.T) {
	adapter := NewTaskStoreAdapter(&versionedStore{fakeStore: newFakeStore()})
	if _, _, err := adapter.Get(callerCtx("key-a"), "missing"); !errors.Is(err, sdka2a.ErrTaskNotFound) {
		t.Fatalf("Get(missing) = %v, want ErrTaskNotFound", err)
	}
}

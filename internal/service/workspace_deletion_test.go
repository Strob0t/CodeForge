package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-96 D11: under default ACLs a tool can make entries only its tool UID can
// remove, so with workspace.tool_acls: required the Go Core no longer removes a
// deleted project's workspace itself: it records the deletion with the project
// row's removal and the worker removes the tree as the tenant's tool UID.

const delTenant = "11111111-1111-1111-1111-111111111111"

type fakeDeletionStore struct {
	mu        sync.Mutex
	busy      bool
	deleted   []string
	recorded  []project.WorkspaceDeletion
	done      []string
	failures  map[string]string
	pending   []project.WorkspaceDeletion
	tenantOf  map[string]string // deletion id -> tenant in ctx when marked
	pendingAt time.Duration
}

func (f *fakeDeletionStore) DeleteProjectForWorkspaceDeletion(ctx context.Context, projectID string, d *project.WorkspaceDeletion) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.busy {
		return project.ErrProjectBusy
	}
	if tenantctx.FromContext(ctx) != d.TenantID {
		return errors.New("tenant of the context differs")
	}
	f.deleted = append(f.deleted, projectID)
	f.recorded = append(f.recorded, *d)
	return nil
}

func (f *fakeDeletionStore) RecordWorkspaceDeletion(ctx context.Context, d *project.WorkspaceDeletion) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if tenantctx.FromContext(ctx) != d.TenantID {
		return errors.New("tenant of the context differs")
	}
	f.recorded = append(f.recorded, *d)
	return nil
}

func (f *fakeDeletionStore) MarkWorkspaceDeletionDone(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.done = append(f.done, id)
	if f.tenantOf == nil {
		f.tenantOf = map[string]string{}
	}
	f.tenantOf[id] = tenantctx.FromContext(ctx)
	return nil
}

func (f *fakeDeletionStore) RecordWorkspaceDeletionFailure(ctx context.Context, id, message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failures == nil {
		f.failures = map[string]string{}
	}
	f.failures[id] = message
	if f.tenantOf == nil {
		f.tenantOf = map[string]string{}
	}
	f.tenantOf[id] = tenantctx.FromContext(ctx)
	return nil
}

func (f *fakeDeletionStore) ListPendingWorkspaceDeletions(_ context.Context, olderThan time.Duration, _ int) ([]project.WorkspaceDeletion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pendingAt = olderThan
	return f.pending, nil
}

type unreachableQueue struct{ runtimeMockQueue }

func (q *unreachableQueue) Publish(context.Context, string, []byte) error {
	return errors.New("nats: connection closed")
}

func newDeletions(t *testing.T, store *fakeDeletionStore) (*service.WorkspaceDeletionService, *runtimeMockQueue) {
	t.Helper()
	queue := &runtimeMockQueue{}
	uids := service.NewToolUIDService(&toolUIDStoreFake{byTenant: map[string]int{delTenant: 20031}}, true)
	return service.NewWorkspaceDeletionService(store, queue, uids), queue
}

func TestWorkspaceDeletion_RecordsTheDeletionAndAsksTheWorker(t *testing.T) {
	store := &fakeDeletionStore{}
	svc, queue := newDeletions(t, store)
	ctx := tenantctx.WithTenant(context.Background(), delTenant)
	p := &project.Project{ID: "proj-1", WorkspacePath: "/data/workspaces/" + delTenant + "/proj-1"}

	if err := svc.Delete(ctx, p); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(store.recorded) != 1 || store.deleted[0] != "proj-1" {
		t.Fatalf("recorded %+v, deleted %v", store.recorded, store.deleted)
	}
	d := store.recorded[0]
	if d.ID == "" || d.TenantID != delTenant || d.ToolUID != 20031 || d.WorkspacePath != p.WorkspacePath {
		t.Fatalf("deletion = %+v", d)
	}
	msg, ok := queue.lastMessage(messagequeue.SubjectWorkspaceDeleteRequest)
	if !ok {
		t.Fatal("no workspace.delete.request published")
	}
	var payload messagequeue.WorkspaceDeleteRequestPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		t.Fatal(err)
	}
	want := messagequeue.WorkspaceDeleteRequestPayload{
		DeletionID: d.ID, TenantID: delTenant, ToolUID: 20031, ProjectID: "proj-1", WorkspacePath: p.WorkspacePath,
	}
	if payload != want {
		t.Fatalf("payload = %+v, want %+v", payload, want)
	}
}

func TestWorkspaceDeletion_AProjectWithActiveWorkIsNotDeleted(t *testing.T) {
	store := &fakeDeletionStore{busy: true}
	svc, queue := newDeletions(t, store)
	ctx := tenantctx.WithTenant(context.Background(), delTenant)
	err := svc.Delete(ctx, &project.Project{ID: "proj-1", WorkspacePath: "/data/workspaces/x/proj-1"})
	if !errors.Is(err, project.ErrProjectBusy) {
		t.Fatalf("err = %v, want ErrProjectBusy", err)
	}
	if _, ok := queue.lastMessage(messagequeue.SubjectWorkspaceDeleteRequest); ok {
		t.Fatal("published a deletion of a busy project")
	}
}

func TestWorkspaceDeletion_APublishFailureLeavesTheDeletionPending(t *testing.T) {
	store := &fakeDeletionStore{}
	uids := service.NewToolUIDService(&toolUIDStoreFake{byTenant: map[string]int{delTenant: 20031}}, true)
	svc := service.NewWorkspaceDeletionService(store, &unreachableQueue{}, uids)
	ctx := tenantctx.WithTenant(context.Background(), delTenant)
	if err := svc.Delete(ctx, &project.Project{ID: "proj-1", WorkspacePath: "/data/workspaces/x/proj-1"}); err != nil {
		t.Fatalf("the project is deleted; the retry job publishes again: %v", err)
	}
	if len(store.recorded) != 1 {
		t.Fatal("the deletion was not recorded")
	}
}

func TestWorkspaceDeletion_Results(t *testing.T) {
	store := &fakeDeletionStore{}
	svc, _ := newDeletions(t, store)
	ok, _ := json.Marshal(messagequeue.WorkspaceDeleteResultPayload{DeletionID: "d1", TenantID: delTenant, OK: true})
	failed, _ := json.Marshal(messagequeue.WorkspaceDeleteResultPayload{DeletionID: "d2", TenantID: delTenant, Error: "rm: Permission denied"})
	for _, data := range [][]byte{ok, failed} {
		if err := svc.HandleResult(context.Background(), data); err != nil {
			t.Fatalf("HandleResult: %v", err)
		}
	}
	if len(store.done) != 1 || store.done[0] != "d1" {
		t.Fatalf("done = %v", store.done)
	}
	if store.failures["d2"] != "rm: Permission denied" {
		t.Fatalf("failures = %v", store.failures)
	}
	if store.tenantOf["d1"] != delTenant || store.tenantOf["d2"] != delTenant {
		t.Fatalf("results handled outside their tenant: %v", store.tenantOf)
	}
	if err := svc.HandleResult(context.Background(), []byte("{")); err == nil {
		t.Fatal("an unreadable result must be an error (dead-lettered)")
	}
}

func TestWorkspaceDeletion_ADeadLetteredRequestIsAFailedAttempt(t *testing.T) {
	store := &fakeDeletionStore{}
	svc, _ := newDeletions(t, store)
	data, _ := json.Marshal(messagequeue.WorkspaceDeleteRequestPayload{DeletionID: "d3", TenantID: delTenant, ToolUID: 20031})
	if err := svc.HandleDeadLetter(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if store.failures["d3"] == "" || store.tenantOf["d3"] != delTenant {
		t.Fatalf("failures = %v, tenants = %v", store.failures, store.tenantOf)
	}
	// Unreadable copies are ignored (a forged dead-letter cannot fail anything).
	if err := svc.HandleDeadLetter(context.Background(), []byte("nope")); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceDeletion_PendingDeletionsArePublishedAgain(t *testing.T) {
	store := &fakeDeletionStore{pending: []project.WorkspaceDeletion{
		{ID: "d4", TenantID: delTenant, ProjectID: "p4", WorkspacePath: "/data/workspaces/x/p4", ToolUID: 20031},
		{ID: "d5", TenantID: "22222222-2222-2222-2222-222222222222", ProjectID: "p5", WorkspacePath: "/data/workspaces/y/p5", ToolUID: 20032},
	}}
	svc, queue := newDeletions(t, store)
	if n := svc.RetryPending(context.Background()); n != 2 {
		t.Fatalf("RetryPending = %d, want 2", n)
	}
	if store.pendingAt != service.WorkspaceDeletionRetryInterval {
		t.Fatalf("pending older than %v", store.pendingAt)
	}
	var tenants []string
	for _, m := range queue.messages {
		var p messagequeue.WorkspaceDeleteRequestPayload
		if err := json.Unmarshal(m.Data, &p); err != nil {
			t.Fatal(err)
		}
		tenants = append(tenants, p.TenantID)
	}
	if len(tenants) != 2 || tenants[0] != delTenant || tenants[1] != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("republished %v", tenants)
	}
}

// A Go Core that was down republishes pending deletions at start; stop ends the job.
func TestWorkspaceDeletion_TheRetryJobRunsAtStartAndStops(t *testing.T) {
	store := &fakeDeletionStore{pending: []project.WorkspaceDeletion{
		{ID: "d6", TenantID: delTenant, ProjectID: "p6", WorkspacePath: "/data/workspaces/x/p6", ToolUID: 20031},
	}}
	svc, queue := newDeletions(t, store)
	stop := svc.StartRetryJob(context.Background())
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, ok := queue.lastMessage(messagequeue.SubjectWorkspaceDeleteRequest); ok {
			break
		}
		if time.Now().After(deadline) {
			stop()
			t.Fatal("the retry job did not publish the pending deletion at start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	stop()
}

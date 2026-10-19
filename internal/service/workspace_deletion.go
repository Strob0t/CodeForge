package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// WorkspaceDeletionRetryInterval is how often pending workspace deletions
// are published again (and how old one must be to be published again).
const WorkspaceDeletionRetryInterval = 10 * time.Minute

const workspaceDeletionRetryBatch = 100

// WorkspaceDeletionService deletes projects whose workspaces the worker
// removes as the tenant's tool UID (KI-96 D11). Under the tenant
// directories' default ACLs a tool can create entries (mkdtemp, mkdir -m
// 0700) that neither the Go Core nor the worker can remove, only the tool
// UID; a project deletion that left them behind would make GDPR erasure
// silently incomplete. The project row's removal and the deletion record
// are one transaction; workspace.delete.request is published at once and
// again for pending deletions until the worker reports one done.
type WorkspaceDeletionService struct {
	store    database.WorkspaceDeletionStore
	queue    messagequeue.Queue
	toolUIDs *ToolUIDService
}

// NewWorkspaceDeletionService creates the service.
func NewWorkspaceDeletionService(store database.WorkspaceDeletionStore, queue messagequeue.Queue, toolUIDs *ToolUIDService) *WorkspaceDeletionService {
	return &WorkspaceDeletionService{store: store, queue: queue, toolUIDs: toolUIDs}
}

// Delete deletes p (project.ErrProjectBusy while it has active work) and
// asks the worker to remove its workspace. A publish that fails leaves the
// deletion pending for the retry job.
func (s *WorkspaceDeletionService) Delete(ctx context.Context, p *project.Project) error {
	tenantID := tenantctx.FromContext(ctx)
	uid, err := s.toolUIDs.ToolUIDFor(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("delete project %s: %w", p.ID, err)
	}
	d := &project.WorkspaceDeletion{
		ID:            uuid.NewString(),
		TenantID:      tenantID,
		ProjectID:     p.ID,
		WorkspacePath: p.WorkspacePath,
		ToolUID:       uid,
	}
	if err := s.store.DeleteProjectForWorkspaceDeletion(ctx, p.ID, d); err != nil {
		return err
	}
	s.publish(ctx, d)
	return nil
}

// Discard hands the workspace dir of p, which a re-clone replaces, to the
// worker (KI-189): the Go Core only renames the entry in the tenant
// directory it owns to <project>.discarded-<id>, never walks the tree, and
// the worker removes it as the tenant's tool UID like a deleted project's.
// The deletion is recorded before the rename, so a failed rename leaves a
// record of a path that is gone, which the worker reports done.
func (s *WorkspaceDeletionService) Discard(ctx context.Context, p *project.Project, dir string) error {
	tenantID := tenantctx.FromContext(ctx)
	uid, err := s.toolUIDs.ToolUIDFor(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("discard workspace of project %s: %w", p.ID, err)
	}
	id := uuid.NewString()
	d := &project.WorkspaceDeletion{
		ID:            id,
		TenantID:      tenantID,
		ProjectID:     p.ID,
		WorkspacePath: filepath.Join(filepath.Dir(dir), p.ID+".discarded-"+id),
		ToolUID:       uid,
	}
	if err := s.store.RecordWorkspaceDeletion(ctx, d); err != nil {
		return fmt.Errorf("discard workspace of project %s: %w", p.ID, err)
	}
	if err := os.Rename(dir, d.WorkspacePath); err != nil {
		return fmt.Errorf("discard workspace of project %s: move it aside: %w", p.ID, err)
	}
	s.publish(ctx, d)
	return nil
}

func (s *WorkspaceDeletionService) publish(ctx context.Context, d *project.WorkspaceDeletion) bool {
	data, err := json.Marshal(messagequeue.WorkspaceDeleteRequestPayload{
		DeletionID:    d.ID,
		TenantID:      d.TenantID,
		ToolUID:       d.ToolUID,
		ProjectID:     d.ProjectID,
		WorkspacePath: d.WorkspacePath,
	})
	if err != nil {
		slog.Error("marshal workspace deletion", "deletion_id", d.ID, "error", err.Error())
		return false
	}
	if err := s.queue.Publish(tenantctx.WithTenant(ctx, d.TenantID), messagequeue.SubjectWorkspaceDeleteRequest, data); err != nil {
		slog.Error("workspace deletion not published, the retry job publishes it again",
			"deletion_id", d.ID, "tenant_id", d.TenantID, "error", err.Error())
		return false
	}
	return true
}

// HandleResult records the worker's outcome of a deletion in its tenant. An
// unreadable result is an error (dead-lettered); one of an unknown deletion
// is ignored.
func (s *WorkspaceDeletionService) HandleResult(ctx context.Context, data []byte) error {
	var res messagequeue.WorkspaceDeleteResultPayload
	if err := json.Unmarshal(data, &res); err != nil {
		return fmt.Errorf("unmarshal workspace deletion result: %w", err)
	}
	if res.DeletionID == "" || res.TenantID == "" {
		slog.Warn("workspace deletion result without deletion or tenant, ignored")
		return nil
	}
	ctx = tenantctx.WithTenant(ctx, res.TenantID)
	var err error
	if res.OK {
		err = s.store.MarkWorkspaceDeletionDone(ctx, res.DeletionID)
	} else {
		message := res.Error
		if message == "" {
			message = "the worker could not remove the workspace"
		}
		slog.Error("workspace deletion failed, it is retried", "deletion_id", res.DeletionID, "tenant_id", res.TenantID, "error", message)
		err = s.store.RecordWorkspaceDeletionFailure(ctx, res.DeletionID, message)
	}
	if errors.Is(err, domain.ErrNotFound) {
		slog.Warn("result of an unknown workspace deletion, ignored", "deletion_id", res.DeletionID)
		return nil
	}
	return err
}

// HandleDeadLetter counts a request the worker could not take as a failed
// attempt; the retry job publishes it again. A copy that cannot be read or
// names an unknown deletion is ignored.
func (s *WorkspaceDeletionService) HandleDeadLetter(ctx context.Context, data []byte) error {
	var req messagequeue.WorkspaceDeleteRequestPayload
	if err := json.Unmarshal(data, &req); err != nil || req.DeletionID == "" || req.TenantID == "" {
		slog.Warn("unreadable dead-lettered workspace deletion, ignored")
		return nil
	}
	err := s.store.RecordWorkspaceDeletionFailure(tenantctx.WithTenant(ctx, req.TenantID), req.DeletionID,
		"the worker could not take the request (dead-lettered)")
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	return err
}

// RetryPending publishes every deletion pending for longer than the retry
// interval again, each in its own tenant; it returns how many it published.
func (s *WorkspaceDeletionService) RetryPending(ctx context.Context) int {
	pending, err := s.store.ListPendingWorkspaceDeletions(ctx, WorkspaceDeletionRetryInterval, workspaceDeletionRetryBatch)
	if err != nil {
		slog.Error("list pending workspace deletions", "error", err.Error())
		return 0
	}
	published := 0
	for i := range pending {
		if s.publish(ctx, &pending[i]) {
			published++
		}
	}
	if published > 0 {
		slog.Info("pending workspace deletions published again", "count", published)
	}
	return published
}

// StartSubscribers subscribes to the worker's results and to dead-lettered requests.
func (s *WorkspaceDeletionService) StartSubscribers(ctx context.Context) (func(), error) {
	cancelResults, err := s.queue.Subscribe(ctx, messagequeue.SubjectWorkspaceDeleteResult,
		func(ctx context.Context, _ string, data []byte) error { return s.HandleResult(ctx, data) })
	if err != nil {
		return nil, fmt.Errorf("subscribe workspace deletion results: %w", err)
	}
	cancelDLQ, err := s.queue.Subscribe(ctx, messagequeue.SubjectWorkspaceDeleteRequest+deadLetterSuffix,
		func(ctx context.Context, _ string, data []byte) error { return s.HandleDeadLetter(ctx, data) })
	if err != nil {
		cancelResults()
		return nil, fmt.Errorf("subscribe dead-lettered workspace deletions: %w", err)
	}
	return func() { cancelResults(); cancelDLQ() }, nil
}

// StartRetryJob publishes pending deletions again at start (a Go Core that
// was down) and every retry interval; stop ends it and waits for a running pass.
func (s *WorkspaceDeletionService) StartRetryJob(ctx context.Context) (stop func()) {
	return startPeriodic(ctx, WorkspaceDeletionRetryInterval, true, func(ctx context.Context) { s.RetryPending(ctx) })
}

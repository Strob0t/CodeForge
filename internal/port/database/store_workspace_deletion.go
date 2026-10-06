package database

import (
	"context"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// WorkspaceDeletionStore records project workspaces the worker removes as
// the tenant's tool UID (KI-96 D11).
type WorkspaceDeletionStore interface {
	// DeleteProjectForWorkspaceDeletion deletes the project and records d in
	// one transaction; project.ErrProjectBusy while the project has an active
	// run, conversation turn or backend task, domain.ErrNotFound for an
	// unknown project or one of another tenant.
	DeleteProjectForWorkspaceDeletion(ctx context.Context, projectID string, d *project.WorkspaceDeletion) error
	// RecordWorkspaceDeletion records d for the tenant in ctx, the project
	// stays (a workspace a re-clone replaces, KI-189).
	RecordWorkspaceDeletion(ctx context.Context, d *project.WorkspaceDeletion) error
	// MarkWorkspaceDeletionDone marks the deletion of the tenant in ctx done.
	MarkWorkspaceDeletionDone(ctx context.Context, id string) error
	// RecordWorkspaceDeletionFailure counts a failed attempt and keeps its error.
	RecordWorkspaceDeletionFailure(ctx context.Context, id, message string) error
	// ListPendingWorkspaceDeletions returns up to limit deletions of every
	// tenant not done and requested more than olderThan ago (the retry job).
	ListPendingWorkspaceDeletions(ctx context.Context, olderThan time.Duration, limit int) ([]project.WorkspaceDeletion, error)
}

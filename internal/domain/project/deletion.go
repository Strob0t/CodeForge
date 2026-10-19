package project

import (
	"errors"
	"time"
)

// ErrProjectBusy is returned when a project cannot be deleted while it has
// active work (a run, a conversation turn or a backend task).
var ErrProjectBusy = errors.New("project has active work: stop its runs, conversation turns and tasks first")

// WorkspaceDeletion is a deleted project's workspace the worker removes as
// the tenant's tool UID (KI-96 D11): under default ACLs a tool can create
// entries only its UID can remove, so the Go Core does not remove them
// itself. Pending deletions are published again until the worker reports
// one done.
type WorkspaceDeletion struct {
	ID            string     `json:"id"`
	TenantID      string     `json:"tenant_id"`
	ProjectID     string     `json:"project_id"`
	WorkspacePath string     `json:"workspace_path"`
	ToolUID       int        `json:"tool_uid"`
	RequestedAt   time.Time  `json:"requested_at"`
	DoneAt        *time.Time `json:"done_at,omitempty"`
	Attempts      int        `json:"attempts"`
	LastError     string     `json:"last_error,omitempty"`
}

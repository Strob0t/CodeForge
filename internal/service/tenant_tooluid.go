package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/tenant"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
	"github.com/Strob0t/CodeForge/internal/workspaceacl"
)

// toolUIDStore is what ToolUIDService needs of the store.
type toolUIDStore interface {
	AllocateToolUID(ctx context.Context, tenantID string) (int, error)
	AdvanceToolUIDSequence(ctx context.Context, atLeast int) (bool, error)
}

// ToolUIDService hands out the tenants' tool UIDs (KI-96, ADR-018): the
// worker runs every tool process of a tenant as its UID. A UID is allocated
// the first time the Go Core needs one for the tenant (its first workspace
// directory, its first tool work), and only with workspace.tool_acls:
// required; development databases allocate nothing. Set UIDs never change,
// so they are cached for the process lifetime.
type ToolUIDService struct {
	store    toolUIDStore
	required bool

	mu    sync.Mutex
	cache map[string]int
}

// NewToolUIDService creates the service; required is workspace.tool_acls == required.
func NewToolUIDService(store toolUIDStore, required bool) *ToolUIDService {
	return &ToolUIDService{store: store, required: required, cache: make(map[string]int)}
}

// Required reports whether tool UIDs are in use (workspace.tool_acls: required).
// A nil service is off.
func (s *ToolUIDService) Required() bool {
	return s != nil && s.required
}

// ToolUIDFor returns the tool UID of tenantID, allocating it the first time.
// tenant.ErrToolUIDRangeExhausted when every UID of the range is taken.
func (s *ToolUIDService) ToolUIDFor(ctx context.Context, tenantID string) (int, error) {
	if tenantID == "" {
		return 0, fmt.Errorf("tool uid: no tenant: %w", domain.ErrValidation)
	}
	s.mu.Lock()
	uid, ok := s.cache[tenantID]
	s.mu.Unlock()
	if ok {
		return uid, nil
	}
	uid, err := s.store.AllocateToolUID(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	if !tenant.IsToolUID(uid) {
		return 0, fmt.Errorf("tool uid %d of tenant %s is outside %d-%d", uid, tenantID, tenant.ToolUIDMin, tenant.ToolUIDMax)
	}
	s.mu.Lock()
	s.cache[tenantID] = uid
	s.mu.Unlock()
	return uid, nil
}

// adoptedWorkspaceLister lists the adopted workspaces of every tenant (the
// startup check of PrepareAtStartup).
type adoptedWorkspaceLister interface {
	ListAdoptedWorkspaces(ctx context.Context, root string) ([]project.Project, error)
}

// toolUIDBindingsDir holds the worker's record of which tenant each tool UID
// belongs to (<root>/.codeforge/uids/<uid>, KI-96 D2/D3): it survives a
// database restore.
const toolUIDBindingsDir = ".codeforge/uids"

// PrepareAtStartup runs the Go Core's startup steps for per-tenant tool
// identities (KI-96) when they are required: it fails on a platform without
// POSIX ACLs, moves the UID sequence past every UID the workspaces volume
// binds (a database restored to an older point would otherwise hand a bound
// UID to a new tenant), and prepares or reports every adopted workspace.
func (s *ToolUIDService) PrepareAtStartup(ctx context.Context, root string, adopted adoptedWorkspaceLister) error {
	if !s.Required() {
		return nil
	}
	if !workspaceacl.Supported {
		return errors.New("workspace.tool_acls: required needs POSIX ACLs (Linux)")
	}
	highest, err := highestBoundToolUID(root)
	if err != nil {
		return err
	}
	moved, err := s.store.AdvanceToolUIDSequence(ctx, highest)
	if err != nil {
		return err
	}
	if moved {
		slog.Warn("the tool UID sequence was behind the UIDs the workspaces volume binds (a database restore?): advanced",
			"highest_bound_uid", highest)
	}
	if adopted != nil {
		s.checkAdoptedWorkspaces(ctx, root, adopted)
	}
	return nil
}

// highestBoundToolUID is the highest tool UID the workspaces volume binds (0: none).
func highestBoundToolUID(root string) (int, error) {
	entries, err := os.ReadDir(filepath.Join(root, toolUIDBindingsDir))
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read the tool uid bindings: %w", err)
	}
	highest := 0
	for _, e := range entries {
		uid, err := strconv.Atoi(e.Name())
		if err != nil || !tenant.IsToolUID(uid) {
			continue
		}
		highest = max(highest, uid)
	}
	return highest, nil
}

// checkAdoptedWorkspaces gives every adopted workspace the Core owns the ACLs
// of a project for its tenant's tool UID, and logs the operator's command
// for the others (the worker refuses their tool work until then).
func (s *ToolUIDService) checkAdoptedWorkspaces(ctx context.Context, root string, adopted adoptedWorkspaceLister) {
	projects, err := adopted.ListAdoptedWorkspaces(ctx, root)
	if err != nil {
		slog.Error("cannot list the adopted workspaces for their tool UIDs", "error", err)
		return
	}
	for i := range projects {
		p := &projects[i]
		tctx := tenantctx.WithTenant(ctx, p.TenantID)
		uid, err := s.ToolUIDFor(tctx, p.TenantID)
		if err != nil {
			slog.Error("adopted workspace without a tool UID", "project_id", p.ID, "tenant_id", p.TenantID, "error", err)
			continue
		}
		if err := workspaceacl.SetProjectACLs(p.WorkspacePath, uid); err != nil {
			slog.Warn("adopted workspace needs ACLs for its tenant's tool UID: run the command as an operator",
				"project_id", p.ID, "tenant_id", p.TenantID, "tool_uid", uid, "path", p.WorkspacePath,
				"reason", err.Error(), "command", AdoptedWorkspaceCommand(p.WorkspacePath, uid))
			continue
		}
		slog.Info("adopted workspace opened to its tenant's tool UID", "project_id", p.ID, "tenant_id", p.TenantID, "tool_uid", uid)
	}
}

// toolUIDSource gives a service that publishes payloads starting tool
// processes the tenants' tool UIDs (embedded; nil: tool ACLs off).
type toolUIDSource struct {
	toolUIDs *ToolUIDService
}

// SetToolUIDs sets the tool UID service (KI-96).
func (s *toolUIDSource) SetToolUIDs(svc *ToolUIDService) {
	s.toolUIDs = svc
}

// PayloadToolUID is the tool_uid of a payload that starts tool processes for
// tenantID (the payload's own tenant_id, so the two always match): 0 (the
// field is omitted) while tool ACLs are off.
func (s *ToolUIDService) PayloadToolUID(ctx context.Context, tenantID string) (int, error) {
	if !s.Required() {
		return 0, nil
	}
	return s.ToolUIDFor(ctx, tenantID)
}

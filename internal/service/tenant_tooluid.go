package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/tenant"
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

package postgres_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// --------------------------------------------------------------------------
// TestStore_Conversation_TenantIsolation
// --------------------------------------------------------------------------

func TestStore_Conversation_TenantIsolation(t *testing.T) {
	store := setupStore(t)
	tenantA := createTestTenant(t, store)
	tenantB := createTestTenant(t, store)
	ctxA := ctxWithTenant(t, tenantA)
	ctxB := ctxWithTenant(t, tenantB)

	// Create a real project under tenant A (project_id is UUID FK).
	proj, err := store.CreateProject(ctxA, &project.CreateRequest{
		Name:     "conv-test-project",
		Provider: "local",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Create a conversation under tenant A.
	conv, err := store.CreateConversation(ctxA, &conversation.Conversation{
		ProjectID: proj.ID,
		Title:     "Test Conversation",
	})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	t.Run("Get_SameTenant", func(t *testing.T) {
		got, err := store.GetConversation(ctxA, conv.ID)
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if got.ID != conv.ID {
			t.Fatalf("expected %s, got %s", conv.ID, got.ID)
		}
	})

	t.Run("Get_WrongTenant", func(t *testing.T) {
		_, err := store.GetConversation(ctxB, conv.ID)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Delete_WrongTenant", func(t *testing.T) {
		err := store.DeleteConversation(ctxB, conv.ID)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Delete_SameTenant", func(t *testing.T) {
		if err := store.DeleteConversation(ctxA, conv.ID); err != nil {
			t.Fatalf("expected success, got %v", err)
		}
	})
}

// --------------------------------------------------------------------------
// TestConversationStore_SourceScanTenantIsolation (FIX-010, FIX-011, FIX-031)
// --------------------------------------------------------------------------

func TestConversationStore_SourceScanTenantIsolation(t *testing.T) {
	const filename = "store_conversation.go"
	content := readStoreSource(t, filename)

	t.Run("ContainsTenantID", func(t *testing.T) {
		assertFileContainsTenantID(t, content, filename)
	})

	t.Run("UsesTenantFromCtx", func(t *testing.T) {
		assertFileUsesTenantFromCtx(t, content, filename)
	})

	t.Run("AllQueriesHaveTenantID", func(t *testing.T) {
		// CreateToolMessages does not pass tenantFromCtx to the INSERT,
		// but the conversation_id FK ensures tenant scoping. The
		// subsequent UPDATE explicitly uses tenant_id.
		assertSQLQueriesHaveTenantID(t, content, filename, nil)
	})

	t.Run("ListMessages_JoinIncludesTenantID", func(t *testing.T) {
		// FIX-016 regression guard: ListMessages uses a JOIN to
		// conversations which must include c.tenant_id.
		if !strings.Contains(content, "c.tenant_id") {
			t.Error("ListMessages JOIN must include c.tenant_id for tenant isolation")
		}
	})

	t.Run("SearchConversationMessages_UsesTenant", func(t *testing.T) {
		if !strings.Contains(content, "c.tenant_id = $1") {
			t.Error("SearchConversationMessages must filter by c.tenant_id")
		}
	})
}

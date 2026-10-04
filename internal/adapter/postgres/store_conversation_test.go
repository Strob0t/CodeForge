package postgres_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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

// --------------------------------------------------------------------------
// TestStore_ListMessages_KeepsInsertOrder (KI-147)
// --------------------------------------------------------------------------

// CreateToolMessages inserts a whole turn in one batch, so every message of
// the turn has the same created_at; ListMessages must still return them in
// the order they were written, because the next turn's LLM history is built
// from this list and a tool result must follow the call it answers.
func TestStore_ListMessages_KeepsInsertOrder(t *testing.T) {
	store := setupStore(t)
	ctx := ctxWithTenant(t, createTestTenant(t, store))

	proj, err := store.CreateProject(ctx, &project.CreateRequest{Name: "ki147-order", Provider: "local"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	conv, err := store.CreateConversation(ctx, &conversation.Conversation{ProjectID: proj.ID, Title: "order"})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if _, err := store.CreateMessage(ctx, &conversation.Message{ConversationID: conv.ID, Role: "user", Content: "draft"}); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	var batch []conversation.Message
	for i := range 10 {
		callID := fmt.Sprintf("call_%02d", i)
		batch = append(batch,
			conversation.Message{
				Role:      "assistant",
				Content:   "call " + callID,
				ToolCalls: json.RawMessage(fmt.Sprintf(`[{"id":%q,"type":"function","function":{"name":"read_file","arguments":"{}"}}]`, callID)),
			},
			conversation.Message{Role: "tool", Content: "result " + callID, ToolCallID: callID, ToolName: "read_file"},
		)
	}
	batch = append(batch, conversation.Message{Role: "assistant", Content: "done"})
	if err := store.CreateToolMessages(ctx, conv.ID, batch); err != nil {
		t.Fatalf("CreateToolMessages: %v", err)
	}
	// Rows are scanned in heap order, which stops matching the insert order
	// once storage is reused (vacuumed deletes, compaction, retention) or
	// several connections write. Changing an indexed column moves the first
	// message behind the turn in the heap, so the sort gets unsorted input,
	// as in a long-lived database, and ties are no longer kept by chance.
	if _, err := retentionPool(t).Exec(ctx,
		`UPDATE conversation_messages SET content = 'start' WHERE conversation_id = $1 AND content = 'draft'`,
		conv.ID); err != nil {
		t.Fatalf("move first message: %v", err)
	}
	if _, err := store.CreateMessage(ctx, &conversation.Message{ConversationID: conv.ID, Role: "user", Content: "next"}); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	got, err := store.ListMessages(ctx, conv.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	want := []string{"start"}
	for i := range batch {
		want = append(want, batch[i].Content)
	}
	want = append(want, "next")
	gotContent := make([]string, len(got))
	for i := range got {
		gotContent[i] = got[i].Content
	}
	if !slices.Equal(gotContent, want) {
		t.Fatalf("messages out of insert order:\n got  %q\n want %q", gotContent, want)
	}
}

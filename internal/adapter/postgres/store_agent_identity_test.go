package postgres_test

import (
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// --------------------------------------------------------------------------
// TestStore_AgentIdentity_TenantIsolation
// --------------------------------------------------------------------------

func TestStore_AgentIdentity_TenantIsolation(t *testing.T) {
	store := setupStore(t)
	tenantA := createTestTenant(t, store)
	tenantB := createTestTenant(t, store)
	ctxA := ctxWithTenant(t, tenantA)
	ctxB := ctxWithTenant(t, tenantB)

	// Create a project under tenant A (agent needs project_id FK).
	proj, err := store.CreateProject(ctxA, &project.CreateRequest{
		Name:     "agent-identity-test",
		Provider: "local",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Create an agent under tenant A.
	ag, err := store.CreateAgent(ctxA, proj.ID, "test-agent", "mock", nil, nil)
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	t.Run("IncrementAgentStats_WrongTenant", func(t *testing.T) {
		err := store.IncrementAgentStats(ctxB, ag.ID, 0.01, true)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound for wrong tenant, got %v", err)
		}
	})

	t.Run("IncrementAgentStats_SameTenant", func(t *testing.T) {
		err := store.IncrementAgentStats(ctxA, ag.ID, 0.01, true)
		if err != nil {
			t.Fatalf("expected success for same tenant, got %v", err)
		}
	})

	t.Run("UpdateAgentState_WrongTenant", func(t *testing.T) {
		err := store.UpdateAgentState(ctxB, ag.ID, map[string]string{"key": "val"})
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound for wrong tenant, got %v", err)
		}
	})

	t.Run("UpdateAgentState_SameTenant", func(t *testing.T) {
		err := store.UpdateAgentState(ctxA, ag.ID, map[string]string{"key": "val"})
		if err != nil {
			t.Fatalf("expected success for same tenant, got %v", err)
		}
	})

	t.Run("SendAgentMessage_ThenListFromWrongTenant", func(t *testing.T) {
		msg := &agent.InboxMessage{
			AgentID:   ag.ID,
			FromAgent: "other-agent",
			Content:   "hello from tenant A",
			Priority:  1,
		}
		if err := store.SendAgentMessage(ctxA, msg); err != nil {
			t.Fatalf("SendAgentMessage: %v", err)
		}

		// Listing from tenant B should return empty.
		msgs, err := store.ListAgentInbox(ctxB, ag.ID, false)
		if err != nil {
			t.Fatalf("ListAgentInbox: %v", err)
		}
		if len(msgs) != 0 {
			t.Fatalf("expected 0 messages from wrong tenant, got %d", len(msgs))
		}

		// Listing from tenant A should return the message.
		msgs, err = store.ListAgentInbox(ctxA, ag.ID, false)
		if err != nil {
			t.Fatalf("ListAgentInbox: %v", err)
		}
		if len(msgs) == 0 {
			t.Fatal("expected at least 1 message from correct tenant")
		}

		// MarkInboxRead from tenant B should fail.
		err = store.MarkInboxRead(ctxB, msgs[0].ID)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound for MarkInboxRead wrong tenant, got %v", err)
		}

		// MarkInboxRead from tenant A should succeed.
		err = store.MarkInboxRead(ctxA, msgs[0].ID)
		if err != nil {
			t.Fatalf("expected success for MarkInboxRead same tenant, got %v", err)
		}
	})
}

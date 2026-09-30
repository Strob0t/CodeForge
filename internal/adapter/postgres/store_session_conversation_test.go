package postgres_test

import (
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/service"
)

// Conversation sessions: a session without metadata can be created (it was
// stored as '' in a jsonb column, so every conversation session failed), and
// deleting a conversation removes its conversation-only sessions while a
// session that also belongs to a task stays, detached (the foreign key's SET
// NULL alone violated sessions_task_or_conversation).

func TestStore_CreateSessionWithoutMetadata(t *testing.T) {
	f := newStatusFixture(t)
	conv := f.conversation(t)

	sess := &run.Session{ProjectID: f.project.ID, ConversationID: conv.ID, Status: run.SessionStatusActive}
	if err := f.store.CreateSession(f.ctx, sess); err != nil {
		t.Fatalf("CreateSession without metadata: %v", err)
	}
	got, err := f.store.GetSession(f.ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Metadata != "{}" || sess.Metadata != "{}" {
		t.Fatalf("metadata stored %q, returned %q, want the column default {}", got.Metadata, sess.Metadata)
	}
}

func TestSessionService_EnsureConversationSession(t *testing.T) {
	f := newStatusFixture(t)
	conv := f.conversation(t)
	sessions := service.NewSessionService(f.store, nil)

	first, err := sessions.EnsureConversationSession(f.ctx, f.project.ID, conv.ID)
	if err != nil {
		t.Fatalf("EnsureConversationSession: %v", err)
	}
	if first.ConversationID != conv.ID || first.Status != run.SessionStatusActive {
		t.Fatalf("session = %+v, want an active session of conversation %s", first, conv.ID)
	}

	again, err := sessions.EnsureConversationSession(f.ctx, f.project.ID, conv.ID)
	if err != nil {
		t.Fatalf("EnsureConversationSession again: %v", err)
	}
	if again.ID != first.ID {
		t.Fatalf("second call created session %s, want the active session %s", again.ID, first.ID)
	}

	if err := sessions.CompleteSession(f.ctx, first.ID); err != nil {
		t.Fatalf("CompleteSession: %v", err)
	}
	next, err := sessions.EnsureConversationSession(f.ctx, f.project.ID, conv.ID)
	if err != nil {
		t.Fatalf("EnsureConversationSession after completion: %v", err)
	}
	if next.ID == first.ID || next.ParentSessionID != first.ID {
		t.Fatalf("session after completion = %+v, want a new one with parent %s", next, first.ID)
	}
}

func TestStore_DeleteConversationWithSessions(t *testing.T) {
	f := newStatusFixture(t)
	pool := retentionPool(t)
	conv := f.conversation(t)

	convSession := &run.Session{ProjectID: f.project.ID, ConversationID: conv.ID, Status: run.SessionStatusActive}
	if err := f.store.CreateSession(f.ctx, convSession); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	taskSession := &run.Session{ProjectID: f.project.ID, TaskID: f.task.ID, ConversationID: conv.ID, Status: run.SessionStatusActive}
	if err := f.store.CreateSession(f.ctx, taskSession); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if err := f.store.DeleteConversation(f.ctx, conv.ID); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}

	if _, err := f.store.GetConversation(f.ctx, conv.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetConversation after delete: %v, want not found", err)
	}
	assertRows(t, pool, "sessions",
		map[string]string{"task session": taskSession.ID},
		map[string]string{"conversation session": convSession.ID})
	kept, err := f.store.GetSession(f.ctx, taskSession.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if kept.ConversationID != "" || kept.TaskID != f.task.ID {
		t.Fatalf("task session = task %q conversation %q, want task %q and no conversation", kept.TaskID, kept.ConversationID, f.task.ID)
	}
}

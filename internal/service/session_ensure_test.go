package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// purgedSessionStore finds a conversation's active session, which the
// retention job deletes before it is touched.
type purgedSessionStore struct {
	mockStore
	existing *run.Session
	touchErr error
	created  []*run.Session
}

func (s *purgedSessionStore) GetSessionByConversation(context.Context, string) (*run.Session, error) {
	return s.existing, nil
}

func (s *purgedSessionStore) TouchSession(context.Context, string) error { return s.touchErr }

func (s *purgedSessionStore) CreateSession(_ context.Context, sess *run.Session) error {
	sess.ID = fmt.Sprintf("new-%d", len(s.created)+1)
	s.created = append(s.created, sess)
	return nil
}

// A session purged between lookup and reuse is not handed out: the
// conversation gets a new session (without the purged one as parent).
func TestEnsureConversationSession_SessionPurgedMeanwhile(t *testing.T) {
	store := &purgedSessionStore{
		existing: &run.Session{ID: "old", ConversationID: "c1", Status: run.SessionStatusActive},
		touchErr: fmt.Errorf("touch session old: %w", domain.ErrNotFound),
	}
	sess, err := NewSessionService(store, nil).EnsureConversationSession(context.Background(), "p1", "c1")
	if err != nil {
		t.Fatalf("EnsureConversationSession: %v", err)
	}
	if len(store.created) != 1 || sess != store.created[0] {
		t.Fatalf("session = %+v, want a new one (created %d)", sess, len(store.created))
	}
	if sess.ParentSessionID != "" || sess.ConversationID != "c1" || sess.ProjectID != "p1" {
		t.Fatalf("new session = %+v, want conversation c1 of project p1 without a parent", sess)
	}
}

// A touch that fails for another reason keeps the session in use.
func TestEnsureConversationSession_TouchFailureKeepsSession(t *testing.T) {
	store := &purgedSessionStore{
		existing: &run.Session{ID: "old", ConversationID: "c1", Status: run.SessionStatusActive},
		touchErr: errors.New("db down"),
	}
	sess, err := NewSessionService(store, nil).EnsureConversationSession(context.Background(), "p1", "c1")
	if err != nil || sess.ID != "old" || len(store.created) != 0 {
		t.Fatalf("EnsureConversationSession = %+v, %v (created %d), want the existing session", sess, err, len(store.created))
	}
}

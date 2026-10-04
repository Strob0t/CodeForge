package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/llmkey"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// gdprMockStore implements the subset of database.Store used by GDPRService.
type gdprMockStore struct {
	mockStore // embed the existing mock for unused methods

	user            *user.User
	getUserErr      error
	apiKeys         []user.APIKey
	llmKeys         []llmkey.LLMKey
	projects        []project.Project
	sessions        []run.Session
	conversations   []conversation.Conversation
	messages        []conversation.Message
	tasks           []task.Task
	runs            []run.Run
	auditEntries    []database.AuditEntry
	deleteUserErr   error
	anonymizedRows  int64
	anonymizeErr    error
	deleteCalled    bool
	anonymizeCalled bool
	steps           []string         // erasure calls in order
	stepErrs        map[string]error // failing erasure step
}

func (m *gdprMockStore) GetUser(_ context.Context, _ string) (*user.User, error) {
	return m.user, m.getUserErr
}

func (m *gdprMockStore) ListAPIKeysByUser(_ context.Context, _ string) ([]user.APIKey, error) {
	return m.apiKeys, nil
}

func (m *gdprMockStore) ListLLMKeysByUser(_ context.Context, _ string) ([]llmkey.LLMKey, error) {
	return m.llmKeys, nil
}

func (m *gdprMockStore) ListProjects(_ context.Context) ([]project.Project, error) {
	return m.projects, nil
}

func (m *gdprMockStore) ListSessions(_ context.Context, _ string) ([]run.Session, error) {
	return m.sessions, nil
}

func (m *gdprMockStore) ListConversationsByProject(_ context.Context, _ string) ([]conversation.Conversation, error) {
	return m.conversations, nil
}

func (m *gdprMockStore) ListMessages(_ context.Context, _ string) ([]conversation.Message, error) {
	return m.messages, nil
}

func (m *gdprMockStore) ListTasks(_ context.Context, _ string) ([]task.Task, error) {
	return m.tasks, nil
}

func (m *gdprMockStore) ListStaleRuns(_ context.Context, _ run.Status, _ time.Duration, _ int) ([]run.Run, error) {
	return nil, nil
}
func (m *gdprMockStore) TouchRun(_ context.Context, _ string, _ run.Status) error { return nil }

func (m *gdprMockStore) ListRunsByTask(_ context.Context, _ string) ([]run.Run, error) {
	return m.runs, nil
}

func (m *gdprMockStore) ListAuditEntriesByAdmin(_ context.Context, _ string, _ int) ([]database.AuditEntry, error) {
	return m.auditEntries, nil
}

func (m *gdprMockStore) DeleteUser(_ context.Context, _ string) error {
	m.deleteCalled = true
	m.steps = append(m.steps, "delete_user")
	return m.deleteUserErr
}

func (m *gdprMockStore) AnonymizeAuditLogForUser(_ context.Context, _ string) (int64, error) {
	m.anonymizeCalled = true
	m.steps = append(m.steps, "audit_log")
	return m.anonymizedRows, m.anonymizeErr
}

func (m *gdprMockStore) AnonymizeConsentsForUser(_ context.Context, _ string) (int64, error) {
	m.steps = append(m.steps, "user_consents")
	return 1, m.stepErrs["user_consents"]
}

func (m *gdprMockStore) AnonymizeChannelMessagesForUser(_ context.Context, _ string) (int64, error) {
	m.steps = append(m.steps, "channel_messages")
	return 1, m.stepErrs["channel_messages"]
}

func (m *gdprMockStore) AnonymizeQuarantineReviewsForUser(_ context.Context, _ string) (int64, error) {
	m.steps = append(m.steps, "quarantine_reviews")
	return 1, m.stepErrs["quarantine_reviews"]
}

// Erasure removes the user's personal data from every row that outlives the
// user (audit entries, consent records, channel messages, quarantine reviews)
// before it deletes
// the user row; if one of these steps fails, the user is not deleted.
func TestDeleteUserData_AnonymizesRowsThatOutliveTheUser(t *testing.T) {
	store := &gdprMockStore{}
	if err := NewGDPRService(store).DeleteUserData(context.Background(), "u1"); err != nil {
		t.Fatalf("DeleteUserData: %v", err)
	}
	want := []string{"audit_log", "user_consents", "channel_messages", "quarantine_reviews", "delete_user"}
	if strings.Join(store.steps, ",") != strings.Join(want, ",") {
		t.Fatalf("steps = %v, want %v", store.steps, want)
	}
}

func TestDeleteUserData_StepFailureKeepsUser(t *testing.T) {
	for _, step := range []string{"user_consents", "channel_messages", "quarantine_reviews"} {
		t.Run(step, func(t *testing.T) {
			store := &gdprMockStore{stepErrs: map[string]error{step: errors.New("db down")}}
			err := NewGDPRService(store).DeleteUserData(context.Background(), "u1")
			if err == nil || !strings.Contains(err.Error(), step) {
				t.Fatalf("DeleteUserData error = %v, want one naming %s", err, step)
			}
			if store.deleteCalled {
				t.Fatal("DeleteUser must not run after a failed anonymization")
			}
		})
	}
}

// Deleting an account (DELETE /users/{id}) is an erasure too: it takes the
// same anonymization steps before it deletes the user row.
func TestAuthDeleteUser_ErasesLikeGDPR(t *testing.T) {
	store := &gdprMockStore{}
	svc := NewAuthService(store, &config.Auth{JWTSecret: "test-secret-key-must-be-long-enough"})
	if err := svc.DeleteUser(context.Background(), "u1"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	want := []string{"audit_log", "user_consents", "channel_messages", "quarantine_reviews", "delete_user"}
	if strings.Join(store.steps, ",") != strings.Join(want, ",") {
		t.Fatalf("steps = %v, want %v", store.steps, want)
	}

	failing := &gdprMockStore{stepErrs: map[string]error{"user_consents": errors.New("db down")}}
	svc = NewAuthService(failing, &config.Auth{JWTSecret: "test-secret-key-must-be-long-enough"})
	if err := svc.DeleteUser(context.Background(), "u1"); err == nil || failing.deleteCalled {
		t.Fatalf("DeleteUser = %v, deleted %v: want an error and the user kept", err, failing.deleteCalled)
	}
}

func TestExportUserData_Complete(t *testing.T) {
	store := &gdprMockStore{
		user:          &user.User{ID: "u1", Email: "test@example.com"},
		apiKeys:       []user.APIKey{{ID: "k1"}},
		llmKeys:       []llmkey.LLMKey{{ID: "l1"}},
		projects:      []project.Project{{ID: "p1", Name: "TestProject"}},
		sessions:      []run.Session{{ID: "s1"}},
		conversations: []conversation.Conversation{{ID: "c1"}},
		messages:      []conversation.Message{{ID: "m1", Content: "hello"}},
		tasks:         []task.Task{{ID: "t1"}},
		runs:          []run.Run{{ID: "r1"}},
		auditEntries:  []database.AuditEntry{{ID: "a1", Action: "login"}},
	}

	svc := NewGDPRService(store)
	export, err := svc.ExportUserData(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if export.User.Email != "test@example.com" {
		t.Errorf("expected email test@example.com, got %s", export.User.Email)
	}
	if len(export.APIKeys) != 1 {
		t.Errorf("expected 1 api key, got %d", len(export.APIKeys))
	}
	if len(export.LLMKeys) != 1 {
		t.Errorf("expected 1 llm key, got %d", len(export.LLMKeys))
	}
	if len(export.Sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(export.Sessions))
	}
	if len(export.Conversations) != 1 {
		t.Errorf("expected 1 conversation, got %d", len(export.Conversations))
	}
	if len(export.Conversations) > 0 && len(export.Conversations[0].Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(export.Conversations[0].Messages))
	}
	if len(export.CostRecords) != 1 {
		t.Errorf("expected 1 cost record, got %d", len(export.CostRecords))
	}
	if len(export.AuditTrail) != 1 {
		t.Errorf("expected 1 audit entry, got %d", len(export.AuditTrail))
	}
	if export.FormatVersion != "1.0" {
		t.Errorf("expected format version 1.0, got %s", export.FormatVersion)
	}
	if export.ExportedAt.IsZero() {
		t.Error("expected non-zero exported_at timestamp")
	}
}

func TestExportUserData_EmptyUser(t *testing.T) {
	store := &gdprMockStore{
		user: &user.User{ID: "u1", Email: "empty@example.com"},
	}

	svc := NewGDPRService(store)
	export, err := svc.ExportUserData(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(export.APIKeys) != 0 {
		t.Errorf("expected 0 api keys, got %d", len(export.APIKeys))
	}
	if len(export.Conversations) != 0 {
		t.Errorf("expected 0 conversations, got %d", len(export.Conversations))
	}
}

func TestExportUserData_UserNotFound(t *testing.T) {
	store := &gdprMockStore{
		getUserErr: errors.New("user not found"),
	}

	svc := NewGDPRService(store)
	_, err := svc.ExportUserData(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
}

func TestDeleteUserData_AnonymizesAuditLog(t *testing.T) {
	store := &gdprMockStore{
		user:           &user.User{ID: "u1"},
		anonymizedRows: 5,
	}

	svc := NewGDPRService(store)
	err := svc.DeleteUserData(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !store.anonymizeCalled {
		t.Error("expected AnonymizeAuditLogForUser to be called before deletion")
	}
	if !store.deleteCalled {
		t.Error("expected DeleteUser to be called")
	}
}

func TestDeleteUserData_AnonymizeFailure(t *testing.T) {
	store := &gdprMockStore{
		anonymizeErr: errors.New("db connection lost"),
	}

	svc := NewGDPRService(store)
	err := svc.DeleteUserData(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected error when anonymization fails")
	}
	if store.deleteCalled {
		t.Error("DeleteUser should NOT be called when anonymization fails")
	}
}

func TestDeleteUserData_DeleteFailure(t *testing.T) {
	store := &gdprMockStore{
		anonymizedRows: 3,
		deleteUserErr:  errors.New("fk constraint violation"),
	}

	svc := NewGDPRService(store)
	err := svc.DeleteUserData(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected error when delete fails")
	}
	if !store.anonymizeCalled {
		t.Error("anonymization should still have been called")
	}
}

func TestDeleteUserData_NonExistentUser(t *testing.T) {
	store := &gdprMockStore{
		anonymizedRows: 0, // no entries to anonymize
		deleteUserErr:  errors.New("user not found"),
	}

	svc := NewGDPRService(store)
	err := svc.DeleteUserData(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
}

// Suppress unused import warnings for time.
var _ = time.Now

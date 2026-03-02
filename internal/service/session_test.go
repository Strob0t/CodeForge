package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
	"github.com/Strob0t/CodeForge/internal/service"
)

// sessionMockStore provides in-memory run and session storage.
type sessionMockStore struct {
	runtimeMockStore
	sessRuns     []run.Run
	sessSessions []run.Session
}

func (m *sessionMockStore) GetRun(_ context.Context, id string) (*run.Run, error) {
	for i := range m.sessRuns {
		if m.sessRuns[i].ID == id {
			return &m.sessRuns[i], nil
		}
	}
	return nil, errMockNotFound
}

func (m *sessionMockStore) CreateSession(_ context.Context, s *run.Session) error {
	s.ID = fmt.Sprintf("sess-%d", len(m.sessSessions)+1)
	m.sessSessions = append(m.sessSessions, *s)
	return nil
}

func (m *sessionMockStore) GetSession(_ context.Context, id string) (*run.Session, error) {
	for i := range m.sessSessions {
		if m.sessSessions[i].ID == id {
			return &m.sessSessions[i], nil
		}
	}
	return nil, errMockNotFound
}

// sessEventStore is a minimal event store mock for session tests.
type sessEventStore struct{}

var _ eventstore.Store = (*sessEventStore)(nil)

func (m *sessEventStore) Append(_ context.Context, _ *event.AgentEvent) error { return nil }
func (m *sessEventStore) LoadByTask(_ context.Context, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}
func (m *sessEventStore) LoadByAgent(_ context.Context, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}
func (m *sessEventStore) LoadByRun(_ context.Context, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}
func (m *sessEventStore) LoadTrajectory(_ context.Context, _ string, _ eventstore.TrajectoryFilter, _ string, _ int) (*eventstore.TrajectoryPage, error) {
	return nil, nil
}
func (m *sessEventStore) TrajectoryStats(_ context.Context, _ string) (*eventstore.TrajectorySummary, error) {
	return nil, nil
}
func (m *sessEventStore) LoadEventsRange(_ context.Context, _, _, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}
func (m *sessEventStore) ListCheckpoints(_ context.Context, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}
func (m *sessEventStore) AppendAudit(_ context.Context, _ *event.AuditEntry) error { return nil }
func (m *sessEventStore) LoadAudit(_ context.Context, _ *event.AuditFilter, _ string, _ int) (*event.AuditPage, error) {
	return nil, nil
}

func TestSession_Resume(t *testing.T) {
	store := &sessionMockStore{
		sessRuns: []run.Run{
			{ID: "run-1", ProjectID: "proj-1", TaskID: "task-1", AgentID: "agent-1", Status: run.StatusCompleted},
		},
	}
	es := &sessEventStore{}
	svc := service.NewSessionService(store, es)
	ctx := context.Background()

	sess, err := svc.Resume(ctx, run.ResumeRequest{
		RunID:  "run-1",
		Prompt: "continue from here",
	})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if sess.ParentRunID != "run-1" {
		t.Errorf("expected ParentRunID run-1, got %s", sess.ParentRunID)
	}
	if sess.Status != run.SessionStatusActive {
		t.Errorf("expected status active, got %s", sess.Status)
	}
	if sess.ProjectID != "proj-1" {
		t.Errorf("expected ProjectID proj-1, got %s", sess.ProjectID)
	}

	// Verify metadata JSON
	var meta map[string]string
	if err := json.Unmarshal([]byte(sess.Metadata), &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if meta["resumed_from"] != "run-1" {
		t.Errorf("metadata resumed_from: expected run-1, got %s", meta["resumed_from"])
	}
	if meta["prompt"] != "continue from here" {
		t.Errorf("metadata prompt: expected 'continue from here', got %s", meta["prompt"])
	}
}

func TestSession_Fork(t *testing.T) {
	store := &sessionMockStore{
		sessRuns: []run.Run{
			{ID: "run-2", ProjectID: "proj-2", TaskID: "task-2", AgentID: "agent-2", Status: run.StatusCompleted},
		},
	}
	es := &sessEventStore{}
	svc := service.NewSessionService(store, es)
	ctx := context.Background()

	sess, err := svc.Fork(ctx, run.ForkRequest{
		RunID:       "run-2",
		FromEventID: "evt-42",
		Prompt:      "try different approach",
	})
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if sess.ParentRunID != "run-2" {
		t.Errorf("expected ParentRunID run-2, got %s", sess.ParentRunID)
	}
	if sess.Status != run.SessionStatusActive {
		t.Errorf("expected status active, got %s", sess.Status)
	}

	// Verify metadata JSON
	var meta map[string]string
	if err := json.Unmarshal([]byte(sess.Metadata), &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if meta["forked_from"] != "run-2" {
		t.Errorf("metadata forked_from: expected run-2, got %s", meta["forked_from"])
	}
	if meta["from_event"] != "evt-42" {
		t.Errorf("metadata from_event: expected evt-42, got %s", meta["from_event"])
	}
}

func TestSession_Resume_NotFound(t *testing.T) {
	store := &sessionMockStore{} // empty — no runs
	es := &sessEventStore{}
	svc := service.NewSessionService(store, es)
	ctx := context.Background()

	_, err := svc.Resume(ctx, run.ResumeRequest{RunID: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent run")
	}
	if !strings.Contains(err.Error(), "source run") {
		t.Errorf("expected error to contain 'source run', got: %s", err.Error())
	}
}

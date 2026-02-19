package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
)

// SessionService manages resumable execution sessions.
type SessionService struct {
	store  database.Store
	events eventstore.Store
}

// NewSessionService creates a new SessionService.
func NewSessionService(store database.Store, events eventstore.Store) *SessionService {
	return &SessionService{store: store, events: events}
}

// GetSession returns a session by ID.
func (s *SessionService) GetSession(ctx context.Context, id string) (*run.Session, error) {
	return s.store.GetSession(ctx, id)
}

// ListSessions returns all sessions for a project.
func (s *SessionService) ListSessions(ctx context.Context, projectID string) ([]run.Session, error) {
	return s.store.ListSessions(ctx, projectID)
}

// Resume creates a new session that continues from a previous run.
func (s *SessionService) Resume(ctx context.Context, req run.ResumeRequest) (*run.Session, error) {
	r, err := s.store.GetRun(ctx, req.RunID)
	if err != nil {
		return nil, fmt.Errorf("resume: source run: %w", err)
	}

	meta, _ := json.Marshal(map[string]string{
		"resumed_from": r.ID,
		"prompt":       req.Prompt,
	})

	sess := &run.Session{
		ProjectID:   r.ProjectID,
		TaskID:      r.TaskID,
		ParentRunID: r.ID,
		Status:      run.SessionStatusActive,
		Metadata:    string(meta),
	}

	if err := s.store.CreateSession(ctx, sess); err != nil {
		return nil, fmt.Errorf("resume: create session: %w", err)
	}

	slog.Debug("session resumed", "session_id", sess.ID, "from_run", r.ID)
	return sess, nil
}

// Fork creates a new session that branches from a specific point in a run.
func (s *SessionService) Fork(ctx context.Context, req run.ForkRequest) (*run.Session, error) {
	r, err := s.store.GetRun(ctx, req.RunID)
	if err != nil {
		return nil, fmt.Errorf("fork: source run: %w", err)
	}

	meta, _ := json.Marshal(map[string]string{
		"forked_from": r.ID,
		"from_event":  req.FromEventID,
		"prompt":      req.Prompt,
	})

	sess := &run.Session{
		ProjectID:   r.ProjectID,
		TaskID:      r.TaskID,
		ParentRunID: r.ID,
		Status:      run.SessionStatusActive,
		Metadata:    string(meta),
	}

	if err := s.store.CreateSession(ctx, sess); err != nil {
		return nil, fmt.Errorf("fork: create session: %w", err)
	}

	_ = s.events.AppendAudit(ctx, &event.AuditEntry{
		ProjectID: r.ProjectID,
		RunID:     r.ID,
		AgentID:   r.AgentID,
		Action:    "session.forked",
		Details:   fmt.Sprintf("Forked to session %s from event %s", sess.ID, req.FromEventID),
	})

	slog.Debug("session forked", "session_id", sess.ID, "from_run", r.ID, "from_event", req.FromEventID)
	return sess, nil
}

// Rewind marks a session for rewinding to a specific event point.
func (s *SessionService) Rewind(ctx context.Context, req run.RewindRequest) (*run.Session, error) {
	r, err := s.store.GetRun(ctx, req.RunID)
	if err != nil {
		return nil, fmt.Errorf("rewind: source run: %w", err)
	}

	meta, _ := json.Marshal(map[string]string{
		"rewound_from": r.ID,
		"to_event":     req.ToEventID,
	})

	sess := &run.Session{
		ProjectID:   r.ProjectID,
		TaskID:      r.TaskID,
		ParentRunID: r.ID,
		Status:      run.SessionStatusActive,
		Metadata:    string(meta),
	}

	if err := s.store.CreateSession(ctx, sess); err != nil {
		return nil, fmt.Errorf("rewind: create session: %w", err)
	}

	_ = s.events.AppendAudit(ctx, &event.AuditEntry{
		ProjectID: r.ProjectID,
		RunID:     r.ID,
		AgentID:   r.AgentID,
		Action:    "session.rewound",
		Details:   fmt.Sprintf("Rewound to event %s, new session %s", req.ToEventID, sess.ID),
	})

	slog.Debug("session rewound", "session_id", sess.ID, "from_run", r.ID, "to_event", req.ToEventID)
	return sess, nil
}

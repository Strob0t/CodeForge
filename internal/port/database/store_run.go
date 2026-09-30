package database

import (
	"context"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// RunStore defines database operations for runs and sessions.
type RunStore interface {
	// Runs
	CreateRun(ctx context.Context, r *run.Run) error
	GetRun(ctx context.Context, id string) (*run.Run, error)
	// Status writes follow run.SourceStatuses and return domain.ErrConflict
	// for any other transition.
	UpdateRunStatus(ctx context.Context, id string, status run.Status, stepCount int, costUSD float64, tokensIn, tokensOut int64) error
	EnterQualityGate(ctx context.Context, req *run.CompletionRequest) error
	CompleteRun(ctx context.Context, req *run.CompletionRequest) error
	// CountRunStep counts a tool call of a running run (domain.ErrConflict otherwise).
	CountRunStep(ctx context.Context, id string) error
	// AddRunUsage adds usage to a run's counters whatever its status and
	// returns the run as stored afterwards.
	AddRunUsage(ctx context.Context, id string, usage *run.Usage) (*run.Run, error)
	// RaiseRunUsage raises a run's counters to reported totals whatever its
	// status; counters never go down.
	RaiseRunUsage(ctx context.Context, id string, totals *run.Usage) error
	UpdateRunArtifact(ctx context.Context, id, artifactType string, valid *bool, errors []string) error
	ListRunsByTask(ctx context.Context, taskID string) ([]run.Run, error)

	// Sessions
	CreateSession(ctx context.Context, s *run.Session) error
	GetSession(ctx context.Context, id string) (*run.Session, error)
	GetSessionByConversation(ctx context.Context, conversationID string) (*run.Session, error)
	ListSessions(ctx context.Context, projectID string) ([]run.Session, error)
	UpdateSessionStatus(ctx context.Context, id string, status run.SessionStatus, currentRunID string) error

	// Retention
	DeleteExpiredSessions(ctx context.Context, before time.Time, batchSize int) (int64, error)
	DeleteExpiredRuns(ctx context.Context, before time.Time, batchSize int) (int64, error)
}

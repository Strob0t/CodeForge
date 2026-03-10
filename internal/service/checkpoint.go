package service

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/Strob0t/CodeForge/internal/git"
)

// Checkpoint records a single git shadow commit for rollback.
type Checkpoint struct {
	RunID      string    `json:"run_id"`
	CommitHash string    `json:"commit_hash"`
	Tool       string    `json:"tool"`
	CallID     string    `json:"call_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// fileSnapshot holds a snapshot of a file's content before modification.
type fileSnapshot struct {
	Path    string
	Content []byte
}

// CheckpointService manages git-based shadow checkpoints for agent runs
// and file-content snapshots for per-tool-call revert.
type CheckpointService struct {
	mu          sync.Mutex
	checkpoints map[string][]Checkpoint // runID -> ordered list
	pool        *git.Pool

	snapshotMu sync.RWMutex
	snapshots  map[string]map[string]fileSnapshot // runID -> callID -> snapshot
}

// NewCheckpointService creates a new CheckpointService with a shared git pool.
func NewCheckpointService(pool *git.Pool) *CheckpointService {
	return &CheckpointService{
		checkpoints: make(map[string][]Checkpoint),
		pool:        pool,
		snapshots:   make(map[string]map[string]fileSnapshot),
	}
}

// CreateCheckpoint stages all changes and creates a shadow git commit.
func (s *CheckpointService) CreateCheckpoint(ctx context.Context, runID, workspacePath, tool, callID string) error {
	var hash string
	err := s.pool.Run(ctx, func() error {
		// Stage all changes
		if _, err := runCheckpointGit(ctx, workspacePath, "add", "-A"); err != nil {
			return fmt.Errorf("checkpoint git add: %w", err)
		}

		// Create shadow commit
		msg := fmt.Sprintf("codeforge-checkpoint: %s", callID)
		if _, err := runCheckpointGit(ctx, workspacePath, "commit", "--allow-empty", "-m", msg); err != nil {
			return fmt.Errorf("checkpoint git commit: %w", err)
		}

		// Get commit hash
		h, err := runCheckpointGit(ctx, workspacePath, "rev-parse", "HEAD")
		if err != nil {
			return fmt.Errorf("checkpoint get hash: %w", err)
		}
		hash = h
		return nil
	})
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.checkpoints[runID] = append(s.checkpoints[runID], Checkpoint{
		RunID:      runID,
		CommitHash: strings.TrimSpace(hash),
		Tool:       tool,
		CallID:     callID,
		CreatedAt:  time.Now(),
	})
	s.mu.Unlock()

	return nil
}

// GetCheckpoints returns the ordered list of checkpoints for a run.
func (s *CheckpointService) GetCheckpoints(runID string) []Checkpoint {
	s.mu.Lock()
	defer s.mu.Unlock()
	cps := s.checkpoints[runID]
	out := make([]Checkpoint, len(cps))
	copy(out, cps)
	return out
}

// RewindToFirst resets the workspace to the state before the first checkpoint.
func (s *CheckpointService) RewindToFirst(ctx context.Context, runID, workspacePath string) error {
	s.mu.Lock()
	cps := s.checkpoints[runID]
	s.mu.Unlock()

	if len(cps) == 0 {
		return fmt.Errorf("no checkpoints for run %s", runID)
	}

	target := cps[0].CommitHash + "^"
	return s.pool.Run(ctx, func() error {
		if _, err := runCheckpointGit(ctx, workspacePath, "reset", "--hard", target); err != nil {
			return fmt.Errorf("rewind to first: %w", err)
		}
		return nil
	})
}

// RewindToLast resets the workspace to the state before the last checkpoint.
func (s *CheckpointService) RewindToLast(ctx context.Context, runID, workspacePath string) error {
	s.mu.Lock()
	cps := s.checkpoints[runID]
	s.mu.Unlock()

	if len(cps) == 0 {
		return fmt.Errorf("no checkpoints for run %s", runID)
	}

	target := cps[len(cps)-1].CommitHash + "^"
	return s.pool.Run(ctx, func() error {
		if _, err := runCheckpointGit(ctx, workspacePath, "reset", "--hard", target); err != nil {
			return fmt.Errorf("rewind to last: %w", err)
		}
		return nil
	})
}

// CleanupCheckpoints removes shadow commits but keeps the current working state.
func (s *CheckpointService) CleanupCheckpoints(ctx context.Context, runID, workspacePath string) error {
	s.mu.Lock()
	cps := s.checkpoints[runID]
	delete(s.checkpoints, runID)
	s.mu.Unlock()

	if len(cps) == 0 {
		return nil
	}

	target := cps[0].CommitHash + "^"
	return s.pool.Run(ctx, func() error {
		if _, err := runCheckpointGit(ctx, workspacePath, "reset", "--soft", target); err != nil {
			return fmt.Errorf("cleanup checkpoints: %w", err)
		}
		return nil
	})
}

// Store reads the current content of path and saves it under runID/callID.
// This captures a pre-edit snapshot for per-tool-call revert.
func (s *CheckpointService) Store(runID, callID, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("checkpoint read: %w", err)
	}

	s.snapshotMu.Lock()
	defer s.snapshotMu.Unlock()

	if s.snapshots[runID] == nil {
		s.snapshots[runID] = make(map[string]fileSnapshot)
	}
	s.snapshots[runID][callID] = fileSnapshot{Path: path, Content: data}
	return nil
}

// Revert restores the file to its checkpointed content and removes the snapshot.
func (s *CheckpointService) Revert(runID, callID string) error {
	s.snapshotMu.RLock()
	calls, ok := s.snapshots[runID]
	if !ok {
		s.snapshotMu.RUnlock()
		return fmt.Errorf("no checkpoints for run %s", runID)
	}
	snap, ok := calls[callID]
	if !ok {
		s.snapshotMu.RUnlock()
		return fmt.Errorf("no checkpoint for call %s in run %s", callID, runID)
	}
	s.snapshotMu.RUnlock()

	if err := os.WriteFile(snap.Path, snap.Content, 0o644); err != nil {
		return fmt.Errorf("checkpoint revert: %w", err)
	}

	// Remove the used snapshot
	s.snapshotMu.Lock()
	delete(s.snapshots[runID], callID)
	s.snapshotMu.Unlock()

	return nil
}

// ClearRun removes all file snapshots for a given run.
func (s *CheckpointService) ClearRun(runID string) {
	s.snapshotMu.Lock()
	delete(s.snapshots, runID)
	s.snapshotMu.Unlock()
}

// runCheckpointGit executes a git command in the given directory.
func runCheckpointGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return stdout.String(), nil
}

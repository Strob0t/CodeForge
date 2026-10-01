package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/proctemp"
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
//
// A checkpoint is a commit of the whole working tree (tracked changes and
// untracked files that are not ignored), built from a private index with
// commit-tree and kept under checkpointRef, never on a branch. HEAD, the
// user's index and every branch stay as they are, so delivery sees exactly
// the run's change (KI-27). The chain in the repository, not this process,
// is the record rollback, patch delivery and cleanup work from (see
// checkpoint_chain.go), so they also work after a restart.
type CheckpointService struct {
	pool *git.Pool

	mu          sync.Mutex
	checkpoints map[string][]Checkpoint // runID -> checkpoints this process created (informational)
	runLocks    map[string]*sync.Mutex  // runID -> serializes the run's checkpoint operations
	indexDir    string                  // holds the per-run private indexes; created on first use

	snapshotMu sync.RWMutex
	snapshots  map[string]map[string]fileSnapshot // runID -> callID -> snapshot
}

// NewCheckpointService creates a new CheckpointService with a shared git pool.
func NewCheckpointService(pool *git.Pool) *CheckpointService {
	return &CheckpointService{
		pool:        pool,
		checkpoints: make(map[string][]Checkpoint),
		runLocks:    make(map[string]*sync.Mutex),
		snapshots:   make(map[string]map[string]fileSnapshot),
	}
}

// lockRun serializes the checkpoint operations of one run; the run's private
// index is not shared between concurrent git processes.
func (s *CheckpointService) lockRun(runID string) func() {
	s.mu.Lock()
	l, ok := s.runLocks[runID]
	if !ok {
		l = &sync.Mutex{}
		s.runLocks[runID] = l
	}
	s.mu.Unlock()
	l.Lock()
	return l.Unlock
}

// runIndex returns the run's private index, kept across its checkpoints so
// that git's stat cache spares re-hashing unchanged files. A missing one (the
// first checkpoint, or after a restart) is seeded from the user's index.
func (s *CheckpointService) runIndex(ctx context.Context, repo *git.Repo, runID string) (*privateIndex, error) {
	s.mu.Lock()
	if s.indexDir == "" {
		dir, err := proctemp.MkdirTemp("checkpoints-*")
		if err != nil {
			s.mu.Unlock()
			return nil, fmt.Errorf("create checkpoint index directory: %w", err)
		}
		s.indexDir = dir
	}
	idx := indexAt(filepath.Join(s.indexDir, runID+".index"))
	s.mu.Unlock()

	if _, err := os.Lstat(idx.path); errors.Is(err, os.ErrNotExist) {
		if err := seedIndex(repo, idx.path); err != nil {
			return nil, err
		}
		idx.renormalizeIfFiltered(ctx, repo)
	} else if err != nil {
		return nil, fmt.Errorf("checkpoint index: %w", err)
	}
	return idx, nil
}

// CreateCheckpoint records the workspace's working tree as the run's next
// checkpoint without touching HEAD, branches or the user's index. The run's
// first checkpoint also records where HEAD pointed and what the user's index
// held. Git runs hardened (git.OpenRepo, KI-77): the workspace's hooks,
// fsmonitor and filter drivers do not run.
func (s *CheckpointService) CreateCheckpoint(ctx context.Context, runID, workspacePath, tool, callID string) error {
	if err := checkRunID(runID); err != nil {
		return err
	}
	unlock := s.lockRun(runID)
	defer unlock()

	var hash string
	err := s.pool.Run(ctx, func() error {
		repo, err := git.OpenRepo(ctx, workspacePath)
		if err != nil {
			return fmt.Errorf("checkpoint: %w", err)
		}
		idx, err := s.runIndex(ctx, repo, runID)
		if err != nil {
			return err
		}
		if err := idx.addWorktree(ctx, repo); err != nil {
			return fmt.Errorf("checkpoint tree: %w", err)
		}
		tree, err := idx.writeTree(ctx, repo)
		if err != nil {
			return fmt.Errorf("checkpoint tree: %w", err)
		}
		tip, tipTree, err := readCheckpointRef(ctx, repo, runID)
		if err != nil {
			return err
		}
		if tip == "" {
			hash, err = createBaseCheckpoint(ctx, repo, runID, tree, callID)
		} else {
			hash, err = appendCheckpoint(ctx, repo, runID, tip, tipTree, tree, callID)
		}
		return err
	})
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.checkpoints[runID] = append(s.checkpoints[runID], Checkpoint{
		RunID:      runID,
		CommitHash: hash,
		Tool:       tool,
		CallID:     callID,
		CreatedAt:  time.Now(),
	})
	s.mu.Unlock()

	return nil
}

// GetCheckpoints returns the checkpoints this process created for a run, in
// order. It is informational; the repository holds the record.
func (s *CheckpointService) GetCheckpoints(runID string) []Checkpoint {
	s.mu.Lock()
	defer s.mu.Unlock()
	cps := s.checkpoints[runID]
	out := make([]Checkpoint, len(cps))
	copy(out, cps)
	return out
}

// withChain runs fn on the workspace repository and the tip of the run's
// checkpoint chain; ErrNoCheckpoints when the workspace holds none.
func (s *CheckpointService) withChain(ctx context.Context, runID, workspacePath, op string, fn func(repo *git.Repo, idx *privateIndex, tip string) error) error {
	if err := checkRunID(runID); err != nil {
		return err
	}
	unlock := s.lockRun(runID)
	defer unlock()
	return s.pool.Run(ctx, func() error {
		repo, err := git.OpenRepo(ctx, workspacePath)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
		tip, _, err := readCheckpointRef(ctx, repo, runID)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
		if tip == "" {
			return fmt.Errorf("run %s: %w", runID, ErrNoCheckpoints)
		}
		idx, err := s.runIndex(ctx, repo, runID)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
		if err := fn(repo, idx, tip); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
		return nil
	})
}

// RewindToFirst restores the workspace to its state before the run's first
// change: the working tree as it was, including the user's uncommitted and
// untracked files; HEAD on the branch (or detached commit, or unborn branch)
// it was on; and the user's index. Files the run added are removed; ignored
// files are left alone. Branches the run created are kept. It works from the
// checkpoint chain in the repository, also after a restart.
func (s *CheckpointService) RewindToFirst(ctx context.Context, runID, workspacePath string) error {
	return s.withChain(ctx, runID, workspacePath, "rewind to first", func(repo *git.Repo, idx *privateIndex, tip string) error {
		base, err := resolveBase(ctx, repo, tip)
		if err != nil {
			return err
		}
		if err := restoreHead(ctx, repo, base); err != nil {
			return fmt.Errorf("restore HEAD: %w", err)
		}
		if err := restoreWorktree(ctx, repo, idx, base.commit); err != nil {
			return fmt.Errorf("restore working tree: %w", err)
		}
		if err := restoreUserIndex(ctx, repo, base); err != nil {
			return fmt.Errorf("restore index: %w", err)
		}
		return nil
	})
}

// RewindToLast restores the working tree to its state at the run's last
// checkpoint, undoing only the change made after it. HEAD and the user's
// index are not moved.
func (s *CheckpointService) RewindToLast(ctx context.Context, runID, workspacePath string) error {
	return s.withChain(ctx, runID, workspacePath, "rewind to last", func(repo *git.Repo, idx *privateIndex, tip string) error {
		return restoreWorktree(ctx, repo, idx, tip)
	})
}

// CleanupCheckpoints deletes the run's checkpoint ref, whether or not this
// process created the checkpoints, and its private index; the working tree,
// the index and HEAD are not touched. A workspace without a repository has
// nothing to clean up.
func (s *CheckpointService) CleanupCheckpoints(ctx context.Context, runID, workspacePath string) error {
	s.mu.Lock()
	delete(s.checkpoints, runID)
	s.mu.Unlock()
	if checkRunID(runID) != nil {
		return nil // no checkpoint was created under an invalid run ID
	}

	unlock := s.lockRun(runID)
	defer func() {
		s.mu.Lock()
		delete(s.runLocks, runID)
		if s.indexDir != "" {
			_ = os.Remove(filepath.Join(s.indexDir, runID+".index"))
		}
		s.mu.Unlock()
		unlock()
	}()

	return s.pool.Run(ctx, func() error {
		repo, err := git.OpenRepo(ctx, workspacePath)
		if errors.Is(err, git.ErrNotRepository) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("cleanup checkpoints: %w", err)
		}
		if _, err := repo.Run(ctx, nil, "update-ref", "-d", checkpointRef(runID)); err != nil {
			return fmt.Errorf("cleanup checkpoints: %w", err)
		}
		return nil
	})
}

// Store reads the current content of path and saves it under runID/callID.
// This captures a pre-edit snapshot for per-tool-call revert.
func (s *CheckpointService) Store(runID, callID, path string) error {
	data, err := os.ReadFile(path) //nolint:gosec // path is validated by caller (workspace-scoped)
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

	// A file the run deleted is recreated readable and writable for the
	// workspace group, like the rest of the workspace (KI-71).
	if err := os.WriteFile(snap.Path, snap.Content, project.WorkspaceFilePerm); err != nil { //nolint:gosec // G306: shared with the worker's tool user

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

package service

import (
	"context"
	"fmt"
	"os"
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
//
// A checkpoint is a commit of the whole working tree (tracked changes and
// untracked files that are not ignored), built from a private index with
// commit-tree and kept under checkpointRef, never on a branch. HEAD, the
// user's index and every branch stay as they are, so delivery (commit,
// branch, PR, patch) sees exactly the run's change, before or after the
// checkpoints are cleaned up (KI-27). The run's first checkpoint has the
// commit checked out when the run began as its parent, each later one the
// previous checkpoint.
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

// checkpointRef is the ref that keeps a run's checkpoints reachable. It is
// outside refs/heads and refs/tags, so no branch, push or clone carries it.
func checkpointRef(runID string) string {
	return "refs/codeforge/checkpoints/" + runID
}

// checkpointIdentity is the author of checkpoint commits: the workspace may
// have no git identity configured, and checkpoints are not the user's commits.
var checkpointIdentity = []string{
	"GIT_AUTHOR_NAME=CodeForge Checkpoint",
	"GIT_AUTHOR_EMAIL=checkpoint@codeforge.invalid",
	"GIT_COMMITTER_NAME=CodeForge Checkpoint",
	"GIT_COMMITTER_EMAIL=checkpoint@codeforge.invalid",
}

// CreateCheckpoint records the workspace's working tree as the run's next
// checkpoint without touching HEAD, branches or the user's index; the
// workspace's hooks do not run.
func (s *CheckpointService) CreateCheckpoint(ctx context.Context, runID, workspacePath, tool, callID string) error {
	var hash string
	err := s.pool.Run(ctx, func() error {
		tree, err := worktreeTree(ctx, workspacePath)
		if err != nil {
			return fmt.Errorf("checkpoint tree: %w", err)
		}
		args := []string{"commit-tree", "--no-gpg-sign", "-m", "codeforge-checkpoint: " + callID}
		parent, hasParent, err := s.checkpointParent(ctx, runID, workspacePath)
		if err != nil {
			return fmt.Errorf("checkpoint parent: %w", err)
		}
		if hasParent {
			args = append(args, "-p", parent)
		}
		out, err := runGit(ctx, workspacePath, checkpointIdentity, append(args, tree)...)
		if err != nil {
			return fmt.Errorf("checkpoint commit: %w", err)
		}
		hash = strings.TrimSpace(out)
		if _, err := runGit(ctx, workspacePath, nil, "update-ref", checkpointRef(runID), hash); err != nil {
			return fmt.Errorf("checkpoint ref: %w", err)
		}
		return nil
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

// checkpointParent returns the parent of the run's next checkpoint: its last
// checkpoint, else the commit checked out now (none on an unborn branch).
func (s *CheckpointService) checkpointParent(ctx context.Context, runID, workspacePath string) (parent string, ok bool, err error) {
	if last, ok := s.checkpointAt(runID, -1); ok {
		return last.CommitHash, true, nil
	}
	return headCommit(ctx, workspacePath)
}

// headCommit returns the commit HEAD points to; ok is false on an unborn
// branch.
func headCommit(ctx context.Context, dir string) (commit string, ok bool, err error) {
	if _, err := runGit(ctx, dir, nil, "rev-parse", "--git-dir"); err != nil {
		return "", false, err
	}
	out, err := runGit(ctx, dir, nil, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		return "", false, nil // a repository whose branch has no commit yet
	}
	return strings.TrimSpace(out), true, nil
}

// worktreeTree writes the tree of the workspace's working tree as it is.
func worktreeTree(ctx context.Context, dir string) (string, error) {
	idx, err := newWorktreeIndex(ctx, dir)
	if err != nil {
		return "", err
	}
	defer idx.remove()
	out, err := runGit(ctx, dir, idx.env, "write-tree")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
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

// checkpointAt returns the run's checkpoint at index i; -1 is the last one.
func (s *CheckpointService) checkpointAt(runID string, i int) (Checkpoint, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cps := s.checkpoints[runID]
	if len(cps) == 0 {
		return Checkpoint{}, false
	}
	if i < 0 {
		i = len(cps) - 1
	}
	return cps[i], true
}

// RewindToFirst restores the workspace to its state at the run's first
// checkpoint, before the run changed anything: the working tree as it was,
// including the user's uncommitted and untracked files, and, if the run
// moved HEAD (an agent commit), the commit checked out when the run began
// with the index reset to it. Files the run added are removed; ignored files
// are left alone.
func (s *CheckpointService) RewindToFirst(ctx context.Context, runID, workspacePath string) error {
	first, ok := s.checkpointAt(runID, 0)
	if !ok {
		return fmt.Errorf("no checkpoints for run %s", runID)
	}
	return s.pool.Run(ctx, func() error {
		if err := restoreHead(ctx, workspacePath, first.CommitHash); err != nil {
			return fmt.Errorf("rewind to first: %w", err)
		}
		if err := restoreWorktree(ctx, workspacePath, first.CommitHash); err != nil {
			return fmt.Errorf("rewind to first: %w", err)
		}
		return nil
	})
}

// RewindToLast restores the working tree to its state at the run's last
// checkpoint, undoing only the change made after it. HEAD is not moved.
func (s *CheckpointService) RewindToLast(ctx context.Context, runID, workspacePath string) error {
	last, ok := s.checkpointAt(runID, -1)
	if !ok {
		return fmt.Errorf("no checkpoints for run %s", runID)
	}
	return s.pool.Run(ctx, func() error {
		if err := restoreWorktree(ctx, workspacePath, last.CommitHash); err != nil {
			return fmt.Errorf("rewind to last: %w", err)
		}
		return nil
	})
}

// restoreHead moves the checked-out branch and the index back to the first
// checkpoint's parent, the commit checked out when the run began, if the run
// moved HEAD. The working tree is left to restoreWorktree.
func restoreHead(ctx context.Context, dir, firstCheckpoint string) error {
	out, err := runGit(ctx, dir, nil, "rev-list", "--parents", "--max-count=1", firstCheckpoint)
	if err != nil {
		return err
	}
	fields := strings.Fields(out)
	if len(fields) < 2 {
		return nil // the run began on an unborn branch: no commit to go back to
	}
	preRunHead := fields[1]
	head, ok, err := headCommit(ctx, dir)
	if err != nil {
		return err
	}
	if ok && head == preRunHead {
		return nil
	}
	_, err = runGit(ctx, dir, nil, "reset", "--quiet", "--mixed", preRunHead)
	return err
}

// restoreWorktree makes the working tree match the checkpoint's tree: a
// private index holding the current working tree is moved to the checkpoint
// with read-tree -u, which rewrites changed files and deletes the files the
// checkpoint does not have.
func restoreWorktree(ctx context.Context, dir, checkpoint string) error {
	idx, err := newWorktreeIndex(ctx, dir)
	if err != nil {
		return err
	}
	defer idx.remove()
	_, err = runGit(ctx, dir, idx.env, "read-tree", "-u", "--reset", checkpoint)
	return err
}

// CleanupCheckpoints forgets the run's checkpoints and deletes their ref;
// the working tree, the index and HEAD are not touched.
func (s *CheckpointService) CleanupCheckpoints(ctx context.Context, runID, workspacePath string) error {
	s.mu.Lock()
	cps := s.checkpoints[runID]
	delete(s.checkpoints, runID)
	s.mu.Unlock()

	if len(cps) == 0 {
		return nil
	}

	return s.pool.Run(ctx, func() error {
		if _, err := runGit(ctx, workspacePath, nil, "update-ref", "-d", checkpointRef(runID)); err != nil {
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

	if err := os.WriteFile(snap.Path, snap.Content, 0o600); err != nil {
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

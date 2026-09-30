package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// DeliveryResult holds the outcome of a delivery operation.
type DeliveryResult struct {
	Mode       run.DeliverMode `json:"mode"`
	PatchPath  string          `json:"patch_path,omitempty"`
	CommitHash string          `json:"commit_hash,omitempty"`
	BranchName string          `json:"branch_name,omitempty"`
	PRURL      string          `json:"pr_url,omitempty"`
	PushError  string          `json:"push_error,omitempty"` // P2-5: propagate push failure
}

// DeliverService executes delivery strategies after a successful run. All
// git (and gh) runs through git.OpenRepo's hardened repository (KI-77):
// the workspace is agent-writable, so its hooks, filters, drivers and
// transport settings must not run in the Go Core.
type DeliverService struct {
	store database.Store
	cfg   *config.Runtime
	pool  *git.Pool
}

// NewDeliverService creates a new DeliverService with a shared git pool.
func NewDeliverService(store database.Store, cfg *config.Runtime, pool *git.Pool) *DeliverService {
	return &DeliverService{store: store, cfg: cfg, pool: pool}
}

// deliveryIdentity is the author of delivery commits in a repository that
// configures none (the Go Core ignores global git config).
var deliveryIdentity = []string{
	"GIT_AUTHOR_NAME=CodeForge",
	"GIT_AUTHOR_EMAIL=codeforge@codeforge.invalid",
	"GIT_COMMITTER_NAME=CodeForge",
	"GIT_COMMITTER_EMAIL=codeforge@codeforge.invalid",
}

// Deliver executes the delivery strategy for the given run.
func (s *DeliverService) Deliver(ctx context.Context, r *run.Run, taskTitle string) (*DeliveryResult, error) {
	if r.DeliverMode == "" || r.DeliverMode == run.DeliverModeNone {
		return &DeliveryResult{Mode: run.DeliverModeNone}, nil
	}

	// Look up workspace path from project
	proj, err := s.store.GetProject(ctx, r.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("get project for delivery: %w", err)
	}
	dir := proj.WorkspacePath
	if dir == "" {
		return nil, fmt.Errorf("project %s has no workspace_path", r.ProjectID)
	}

	shortID := r.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}

	switch r.DeliverMode {
	case run.DeliverModePatch:
		return s.deliverPatch(ctx, dir, r)
	case run.DeliverModeCommitLocal:
		return s.deliverCommitLocal(ctx, dir, r, shortID, taskTitle)
	case run.DeliverModeBranch:
		return s.deliverBranch(ctx, dir, r, shortID, taskTitle)
	case run.DeliverModePR:
		return s.deliverPR(ctx, dir, r, shortID, taskTitle)
	default:
		return nil, fmt.Errorf("unsupported deliver mode %q", r.DeliverMode)
	}
}

// deliverPatch writes the run's change as a patch: the working tree
// (including new files) against the run's base checkpoint, the working tree
// before its first change, so the user's earlier uncommitted work and files
// of earlier deliveries are not part of it. The diff is taken from a private
// index (the user's index is not touched) before the checkpoints are cleaned
// up, and the patch is written inside the repository's .git directory, where
// git does not see it as a change.
func (s *DeliverService) deliverPatch(ctx context.Context, dir string, r *run.Run) (*DeliveryResult, error) {
	var result *DeliveryResult
	err := s.pool.Run(ctx, func() error {
		repo, err := git.OpenRepo(ctx, dir)
		if err != nil {
			return fmt.Errorf("patch delivery: %w", err)
		}
		diff, err := runChange(ctx, repo, r.ID)
		if err != nil {
			return fmt.Errorf("patch delivery: %w", err)
		}
		patchFile, err := writePatch(repo, r.ID, diff)
		if err != nil {
			return fmt.Errorf("write patch: %w", err)
		}

		slog.Info("patch delivered", "run_id", r.ID, "path", patchFile)
		result = &DeliveryResult{
			Mode:      run.DeliverModePatch,
			PatchPath: patchFile,
		}
		return nil
	})
	return result, err
}

// runChange returns the run's change as a binary diff: its base checkpoint
// against the working tree now.
func runChange(ctx context.Context, repo *git.Repo, runID string) (string, error) {
	if err := checkRunID(runID); err != nil {
		return "", err
	}
	tip, _, err := readCheckpointRef(ctx, repo, runID)
	if err != nil {
		return "", err
	}
	if tip == "" {
		return "", fmt.Errorf("run %s: %w: its change is unknown", runID, ErrNoCheckpoints)
	}
	base, err := resolveBase(ctx, repo, tip)
	if err != nil {
		return "", err
	}
	idx, err := newWorktreeIndex(ctx, repo)
	if err != nil {
		return "", fmt.Errorf("patch index: %w", err)
	}
	defer idx.remove()
	diff, err := repo.Run(ctx, idx.env, "diff", "--cached", "--binary", "--no-ext-diff", "--no-textconv", base.commit, "--")
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}
	return diff, nil
}

// patchDir is where patches are written, relative to the .git directory.
const patchDir = "codeforge/patches"

// writePatch writes the patch to .git/codeforge/patches/<run>.patch and
// returns its path. The directory is agent-writable: all access goes through
// an os.Root on .git, which refuses symlinks leading out of it; the
// directories must not be symlinks at all, and an existing file (or symlink)
// of that name is replaced, never written through.
func writePatch(repo *git.Repo, runID, diff string) (string, error) {
	root, err := os.OpenRoot(repo.GitDir)
	if err != nil {
		return "", err
	}
	defer func() { _ = root.Close() }()
	for _, d := range []string{"codeforge", patchDir} {
		if err := root.Mkdir(d, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			return "", err
		}
		info, err := root.Lstat(d)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", fmt.Errorf(".git/%s is not a directory", d)
		}
	}
	name := patchDir + "/" + runID + ".patch"
	if err := root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	_, writeErr := f.WriteString(diff)
	if closeErr := f.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		return "", writeErr
	}
	return filepath.Join(repo.GitDir, filepath.FromSlash(name)), nil
}

func (s *DeliverService) deliverCommitLocal(ctx context.Context, dir string, r *run.Run, shortID, taskTitle string) (*DeliveryResult, error) {
	var result *DeliveryResult
	err := s.pool.Run(ctx, func() error {
		repo, err := git.OpenRepo(ctx, dir)
		if err != nil {
			return fmt.Errorf("commit-local delivery: %w", err)
		}
		hash, err := s.commitWorkspace(ctx, repo, shortID, taskTitle)
		if err != nil {
			return err
		}
		slog.Info("commit-local delivered", "run_id", r.ID, "hash", hash)
		result = &DeliveryResult{
			Mode:       run.DeliverModeCommitLocal,
			CommitHash: hash,
		}
		return nil
	})
	return result, err
}

// commitWorkspace commits the whole working tree on the checked-out branch
// and returns the commit.
func (s *DeliverService) commitWorkspace(ctx context.Context, repo *git.Repo, shortID, taskTitle string) (string, error) {
	if _, err := repo.Run(ctx, nil, "add", "-A"); err != nil {
		return "", fmt.Errorf("git add: %w", err)
	}
	var identity []string
	if !repo.HasConfig("user.name") || !repo.HasConfig("user.email") {
		identity = deliveryIdentity
	}
	msg := fmt.Sprintf("%s %s [run %s]", s.cfg.DeliveryCommitPrefix, taskTitle, shortID)
	if _, err := repo.Run(ctx, identity, "commit", "--no-verify", "--no-gpg-sign", "-m", msg); err != nil {
		return "", fmt.Errorf("git commit: %w", err)
	}
	hash, err := repo.Run(ctx, nil, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	return strings.TrimSpace(hash), nil
}

func (s *DeliverService) deliverBranch(ctx context.Context, dir string, r *run.Run, shortID, taskTitle string) (*DeliveryResult, error) {
	var result *DeliveryResult
	err := s.pool.Run(ctx, func() error {
		repo, err := git.OpenRepo(ctx, dir)
		if err != nil {
			return fmt.Errorf("branch delivery: %w", err)
		}
		branchName := fmt.Sprintf("codeforge/%s", shortID)

		if _, err := repo.Run(ctx, nil, "checkout", "-b", branchName); err != nil {
			return fmt.Errorf("git checkout -b: %w", err)
		}
		commitHash, err := s.commitWorkspace(ctx, repo, shortID, taskTitle)
		if err != nil {
			return err
		}

		// The remote and its transport come from agent-writable config: a
		// repository that configures transports is not pushed from.
		pushErr := repo.RequireNetworkSafe()
		if pushErr == nil {
			_, pushErr = repo.Run(ctx, nil, "push", "--no-verify", "-u", "origin", branchName)
		}
		var pushError string
		if pushErr != nil {
			pushError = pushErr.Error()
			slog.Warn("git push failed (branch delivery)", "run_id", r.ID, "error", pushErr)
		}

		slog.Info("branch delivered", "run_id", r.ID, "branch", branchName)
		result = &DeliveryResult{
			Mode:       run.DeliverModeBranch,
			BranchName: branchName,
			CommitHash: commitHash,
			PushError:  pushError,
		}
		return nil
	})
	return result, err
}

func (s *DeliverService) deliverPR(ctx context.Context, dir string, r *run.Run, shortID, taskTitle string) (*DeliveryResult, error) {
	// First create branch (already uses pool internally)
	branchResult, err := s.deliverBranch(ctx, dir, r, shortID, taskTitle)
	if err != nil {
		return nil, fmt.Errorf("branch for PR: %w", err)
	}

	// P2-5: If push failed, skip PR creation — can't create PR without remote branch.
	if branchResult.PushError != "" {
		slog.Warn("skipping PR creation due to push failure", "run_id", r.ID, "push_error", branchResult.PushError)
		return branchResult, nil
	}

	// gh reads the remote from the workspace repository and runs git: it
	// gets the repository's hardened environment.
	repo, err := git.OpenRepo(ctx, dir)
	if err == nil {
		err = repo.RequireNetworkSafe()
	}
	if err != nil {
		slog.Warn("gh pr create skipped, falling back to branch-only", "run_id", r.ID, "error", err)
		return branchResult, nil
	}
	prTitle := fmt.Sprintf("%s %s", s.cfg.DeliveryCommitPrefix, taskTitle)
	prBody := fmt.Sprintf("Automated delivery from CodeForge run %s", r.ID)
	cmd := repo.Command(ctx, "gh", "pr", "create",
		"--title", prTitle,
		"--body", prBody,
		"--head", branchResult.BranchName,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if prErr := cmd.Run(); prErr != nil {
		slog.Warn("gh pr create failed, falling back to branch-only", "run_id", r.ID, "error", prErr, "stderr", strings.TrimSpace(stderr.String()))
		return branchResult, nil
	}
	prURL := strings.TrimSpace(stdout.String())

	slog.Info("PR delivered", "run_id", r.ID, "url", prURL)
	return &DeliveryResult{
		Mode:       run.DeliverModePR,
		BranchName: branchResult.BranchName,
		CommitHash: branchResult.CommitHash,
		PRURL:      prURL,
	}, nil
}

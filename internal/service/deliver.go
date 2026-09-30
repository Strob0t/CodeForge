package service

import (
	"bytes"
	"context"
	"fmt"
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
		return s.deliverPatch(ctx, dir, r, shortID)
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

// deliverPatch writes the run's whole change as a patch: the working tree
// (including new files) against HEAD, diffed from a private index so that the
// user's index is not touched.
func (s *DeliverService) deliverPatch(ctx context.Context, dir string, r *run.Run, shortID string) (*DeliveryResult, error) {
	var result *DeliveryResult
	err := s.pool.Run(ctx, func() error {
		repo, err := git.OpenRepo(ctx, dir)
		if err != nil {
			return fmt.Errorf("patch delivery: %w", err)
		}
		idx, err := newWorktreeIndex(ctx, repo)
		if err != nil {
			return fmt.Errorf("patch index: %w", err)
		}
		defer idx.remove()
		diff, err := repo.Run(ctx, idx.env, "diff", "--cached", "--binary", "--no-ext-diff", "--no-textconv")
		if err != nil {
			return fmt.Errorf("git diff: %w", err)
		}

		patchFile := filepath.Join(dir, fmt.Sprintf("%s.patch", shortID))
		if err := os.WriteFile(patchFile, []byte(diff), 0o600); err != nil {
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

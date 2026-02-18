package service

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
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
}

// DeliverService executes delivery strategies after a successful run.
type DeliverService struct {
	store database.Store
	cfg   *config.Runtime
	pool  *git.Pool
}

// NewDeliverService creates a new DeliverService with a shared git pool.
func NewDeliverService(store database.Store, cfg *config.Runtime, pool *git.Pool) *DeliverService {
	return &DeliverService{store: store, cfg: cfg, pool: pool}
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

func (s *DeliverService) deliverPatch(ctx context.Context, dir string, r *run.Run, shortID string) (*DeliveryResult, error) {
	var result *DeliveryResult
	err := s.pool.Run(ctx, func() error {
		diff, err := runDeliverGit(ctx, dir, "diff", "HEAD")
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
		if _, err := runDeliverGit(ctx, dir, "add", "-A"); err != nil {
			return fmt.Errorf("git add: %w", err)
		}

		msg := fmt.Sprintf("%s %s [run %s]", s.cfg.DeliveryCommitPrefix, taskTitle, shortID)
		if _, err := runDeliverGit(ctx, dir, "commit", "-m", msg); err != nil {
			return fmt.Errorf("git commit: %w", err)
		}

		hash, err := runDeliverGit(ctx, dir, "rev-parse", "HEAD")
		if err != nil {
			return fmt.Errorf("git rev-parse: %w", err)
		}

		slog.Info("commit-local delivered", "run_id", r.ID, "hash", strings.TrimSpace(hash))
		result = &DeliveryResult{
			Mode:       run.DeliverModeCommitLocal,
			CommitHash: strings.TrimSpace(hash),
		}
		return nil
	})
	return result, err
}

func (s *DeliverService) deliverBranch(ctx context.Context, dir string, r *run.Run, shortID, taskTitle string) (*DeliveryResult, error) {
	var result *DeliveryResult
	err := s.pool.Run(ctx, func() error {
		branchName := fmt.Sprintf("codeforge/%s", shortID)

		if _, err := runDeliverGit(ctx, dir, "checkout", "-b", branchName); err != nil {
			return fmt.Errorf("git checkout -b: %w", err)
		}

		// Commit on the new branch (add, commit, rev-parse)
		if _, err := runDeliverGit(ctx, dir, "add", "-A"); err != nil {
			return fmt.Errorf("git add: %w", err)
		}

		msg := fmt.Sprintf("%s %s [run %s]", s.cfg.DeliveryCommitPrefix, taskTitle, shortID)
		if _, err := runDeliverGit(ctx, dir, "commit", "-m", msg); err != nil {
			return fmt.Errorf("git commit: %w", err)
		}

		hash, err := runDeliverGit(ctx, dir, "rev-parse", "HEAD")
		if err != nil {
			return fmt.Errorf("git rev-parse: %w", err)
		}
		commitHash := strings.TrimSpace(hash)

		if _, pushErr := runDeliverGit(ctx, dir, "push", "-u", "origin", branchName); pushErr != nil {
			slog.Warn("git push failed (branch delivery)", "run_id", r.ID, "error", pushErr)
		}

		slog.Info("branch delivered", "run_id", r.ID, "branch", branchName)
		result = &DeliveryResult{
			Mode:       run.DeliverModeBranch,
			BranchName: branchName,
			CommitHash: commitHash,
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

	// Try to create PR using gh CLI (not a git operation, no pool needed)
	prTitle := fmt.Sprintf("%s %s", s.cfg.DeliveryCommitPrefix, taskTitle)
	prBody := fmt.Sprintf("Automated delivery from CodeForge run %s", r.ID)
	prURL, prErr := runDeliverCmd(ctx, dir, "gh", "pr", "create",
		"--title", prTitle,
		"--body", prBody,
		"--head", branchResult.BranchName,
	)
	if prErr != nil {
		slog.Warn("gh pr create failed, falling back to branch-only", "run_id", r.ID, "error", prErr)
		return branchResult, nil
	}

	slog.Info("PR delivered", "run_id", r.ID, "url", strings.TrimSpace(prURL))
	return &DeliveryResult{
		Mode:       run.DeliverModePR,
		BranchName: branchResult.BranchName,
		CommitHash: branchResult.CommitHash,
		PRURL:      strings.TrimSpace(prURL),
	}, nil
}

// runDeliverGit runs a git command in the given directory.
func runDeliverGit(ctx context.Context, dir string, args ...string) (string, error) {
	return runDeliverCmd(ctx, dir, "git", args...)
}

// runDeliverCmd runs an arbitrary command in the given directory.
func runDeliverCmd(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return stdout.String(), nil
}

// Package gitlocal implements the gitprovider.Provider interface using local git CLI commands.
package gitlocal

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

const providerName = "local"

// Provider interacts with local git repositories via the git CLI.
type Provider struct {
	pool *git.Pool
}

// NewProvider creates a Provider that limits concurrent git operations via pool.
func NewProvider(pool *git.Pool) *Provider {
	return &Provider{pool: pool}
}

// Name returns "local".
func (p *Provider) Name() string { return providerName }

// Capabilities returns what the local git provider supports.
func (p *Provider) Capabilities() gitprovider.Capabilities {
	return gitprovider.Capabilities{
		Clone: true,
	}
}

// CloneURL returns the URL as-is for local git operations.
func (p *Provider) CloneURL(_ context.Context, repo string) (string, error) {
	return repo, nil
}

// ListRepos is not supported for the local provider.
func (p *Provider) ListRepos(_ context.Context) ([]string, error) {
	return nil, fmt.Errorf("gitlocal: ListRepos not supported")
}

// Clone clones a repository to the given local path.
// If the destination already exists and is a git repository with a matching remote,
// it fetches and resets to the latest state instead of failing.
func (p *Provider) Clone(ctx context.Context, url, destPath string, opts ...gitprovider.CloneOption) error {
	absPath, err := filepath.Abs(destPath)
	if err != nil {
		return fmt.Errorf("gitlocal: resolve path: %w", err)
	}

	o := gitprovider.ApplyCloneOptions(opts)

	return p.pool.Run(ctx, func() error {
		// Check if destination already exists.
		if info, statErr := os.Stat(absPath); statErr == nil && info.IsDir() {
			return p.reclone(ctx, url, absPath, o)
		}

		args := []string{"clone"}
		if o.Branch != "" {
			args = append(args, "--branch", o.Branch, "--single-branch")
		}
		args = append(args, url, absPath)
		if _, execErr := runGit(ctx, "", args...); execErr != nil {
			return fmt.Errorf("gitlocal: clone: %w", execErr)
		}
		return nil
	})
}

// reclone handles re-cloning when the destination directory already exists.
// If it contains a git repo with a matching remote, it fetches + resets.
// Otherwise it removes the directory and does a fresh clone.
func (p *Provider) reclone(ctx context.Context, url, absPath string, o gitprovider.CloneOptions) error {
	// Check if it's a git repo by running git rev-parse.
	if _, err := runGit(ctx, absPath, "rev-parse", "--git-dir"); err == nil {
		// It's a git repo — check if the remote matches.
		remote, _ := runGit(ctx, absPath, "remote", "get-url", "origin")
		if strings.TrimSpace(remote) == url {
			// Same remote: fetch + reset to latest.
			if _, err := runGit(ctx, absPath, "fetch", "origin"); err != nil {
				return fmt.Errorf("gitlocal: fetch: %w", err)
			}

			branch := o.Branch
			if branch == "" {
				// Determine the branch to reset to:
				// 1. Try symbolic-ref for remote HEAD
				// 2. Fall back to current local branch
				ref, refErr := runGit(ctx, absPath, "symbolic-ref", "refs/remotes/origin/HEAD")
				if refErr == nil {
					branch = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ref), "refs/remotes/origin/"))
				}
				if branch == "" {
					// Use the current checked-out branch.
					cur, curErr := runGit(ctx, absPath, "rev-parse", "--abbrev-ref", "HEAD")
					if curErr == nil && strings.TrimSpace(cur) != "" {
						branch = strings.TrimSpace(cur)
					}
				}
				if branch == "" {
					branch = "main"
				}
			}

			if _, err := runGit(ctx, absPath, "checkout", branch); err != nil {
				return fmt.Errorf("gitlocal: checkout %s: %w", branch, err)
			}
			if _, err := runGit(ctx, absPath, "reset", "--hard", "origin/"+branch); err != nil {
				return fmt.Errorf("gitlocal: reset: %w", err)
			}
			return nil
		}
	}

	// Not a git repo or different remote — remove and re-clone.
	if err := os.RemoveAll(absPath); err != nil {
		return fmt.Errorf("gitlocal: remove existing directory: %w", err)
	}

	args := []string{"clone"}
	if o.Branch != "" {
		args = append(args, "--branch", o.Branch, "--single-branch")
	}
	args = append(args, url, absPath)
	if _, err := runGit(ctx, "", args...); err != nil {
		return fmt.Errorf("gitlocal: clone: %w", err)
	}
	return nil
}

// Status returns the git status of a local repository.
func (p *Provider) Status(ctx context.Context, repoPath string) (*project.GitStatus, error) {
	var status *project.GitStatus
	err := p.pool.Run(ctx, func() error {
		status = &project.GitStatus{}

		// Current branch
		branch, err := runGit(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			return fmt.Errorf("gitlocal: get branch: %w", err)
		}
		status.Branch = strings.TrimSpace(branch)

		// Latest commit hash and message
		logOut, err := runGit(ctx, repoPath, "log", "-1", "--format=%H%n%s")
		if err != nil {
			return fmt.Errorf("gitlocal: get log: %w", err)
		}
		logLines := strings.SplitN(strings.TrimSpace(logOut), "\n", 2)
		if len(logLines) >= 1 {
			status.CommitHash = logLines[0]
		}
		if len(logLines) >= 2 {
			status.CommitMessage = logLines[1]
		}

		// Porcelain status for modified/untracked
		porcelain, err := runGit(ctx, repoPath, "status", "--porcelain")
		if err != nil {
			return fmt.Errorf("gitlocal: porcelain status: %w", err)
		}
		for _, line := range strings.Split(porcelain, "\n") {
			if len(line) < 3 {
				continue
			}
			indicator := line[:2]
			file := strings.TrimSpace(line[3:])
			if indicator == "??" {
				status.Untracked = append(status.Untracked, file)
			} else {
				status.Modified = append(status.Modified, file)
			}
		}
		status.Dirty = len(status.Modified) > 0 || len(status.Untracked) > 0

		// Ahead/behind tracking branch
		revList, _ := runGit(ctx, repoPath, "rev-list", "--left-right", "--count", "@{upstream}...HEAD")
		revList = strings.TrimSpace(revList)
		if revList != "" {
			parts := strings.Fields(revList)
			if len(parts) == 2 {
				_, _ = fmt.Sscanf(parts[0], "%d", &status.Behind)
				_, _ = fmt.Sscanf(parts[1], "%d", &status.Ahead)
			}
		}

		return nil
	})
	return status, err
}

// Pull fetches and merges updates for the given repository.
func (p *Provider) Pull(ctx context.Context, repoPath string) error {
	return p.pool.Run(ctx, func() error {
		if _, err := runGit(ctx, repoPath, "pull"); err != nil {
			return fmt.Errorf("gitlocal: pull: %w", err)
		}
		return nil
	})
}

// ListBranches returns all branches of a local repository.
func (p *Provider) ListBranches(ctx context.Context, repoPath string) ([]project.Branch, error) {
	var branches []project.Branch
	err := p.pool.Run(ctx, func() error {
		out, err := runGit(ctx, repoPath, "branch", "--list")
		if err != nil {
			return fmt.Errorf("gitlocal: list branches: %w", err)
		}

		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			current := false
			if strings.HasPrefix(line, "* ") {
				current = true
				line = strings.TrimPrefix(line, "* ")
			}
			branches = append(branches, project.Branch{
				Name:    strings.TrimSpace(line),
				Current: current,
			})
		}
		return nil
	})
	return branches, err
}

// Checkout switches to the specified branch.
func (p *Provider) Checkout(ctx context.Context, repoPath, branch string) error {
	return p.pool.Run(ctx, func() error {
		if _, err := runGit(ctx, repoPath, "checkout", branch); err != nil {
			return fmt.Errorf("gitlocal: checkout %s: %w", branch, err)
		}
		return nil
	})
}

// runGit executes a git command and returns its combined stdout.
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
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

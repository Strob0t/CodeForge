// Package gitlocal implements the gitprovider.Provider interface using local git CLI commands.
package gitlocal

import (
	"bytes"
	"context"
	"fmt"
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
func (p *Provider) Clone(ctx context.Context, url, destPath string) error {
	absPath, err := filepath.Abs(destPath)
	if err != nil {
		return fmt.Errorf("gitlocal: resolve path: %w", err)
	}

	return p.pool.Run(ctx, func() error {
		if _, execErr := runGit(ctx, "", "clone", url, absPath); execErr != nil {
			return fmt.Errorf("gitlocal: clone: %w", execErr)
		}
		return nil
	})
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

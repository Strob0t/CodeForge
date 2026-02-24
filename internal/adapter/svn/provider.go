// Package svn implements the gitprovider.Provider interface for SVN repositories using the svn CLI.
package svn

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

const providerName = "svn"

// Provider interacts with SVN repositories via the svn CLI.
type Provider struct {
	pool        *git.Pool
	execCommand func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// NewProvider creates an SVN provider that limits concurrent operations via pool.
func NewProvider(pool *git.Pool) *Provider {
	return &Provider{pool: pool, execCommand: exec.CommandContext}
}

// Name returns "svn".
func (p *Provider) Name() string { return providerName }

// Capabilities returns what the SVN provider supports.
func (p *Provider) Capabilities() gitprovider.Capabilities {
	return gitprovider.Capabilities{
		Clone:       true,
		Push:        false,
		PullRequest: false,
		Webhook:     false,
		Issues:      false,
	}
}

// CloneURL returns the URL as-is for SVN operations.
func (p *Provider) CloneURL(_ context.Context, repo string) (string, error) {
	return repo, nil
}

// ListRepos is not supported for SVN.
func (p *Provider) ListRepos(_ context.Context) ([]string, error) {
	return nil, fmt.Errorf("svn: ListRepos not supported")
}

// Clone checks out an SVN repository to the given local path.
// CloneOption is accepted for interface compatibility but ignored (SVN has no branch concept in clone).
func (p *Provider) Clone(ctx context.Context, url, destPath string, _ ...gitprovider.CloneOption) error {
	absPath, err := filepath.Abs(destPath)
	if err != nil {
		return fmt.Errorf("svn: resolve path: %w", err)
	}

	return p.pool.Run(ctx, func() error {
		if _, execErr := p.runSVN(ctx, "", "checkout", url, absPath); execErr != nil {
			return fmt.Errorf("svn: checkout: %w", execErr)
		}
		return nil
	})
}

// Status returns the status of an SVN working copy.
func (p *Provider) Status(ctx context.Context, repoPath string) (*project.GitStatus, error) {
	var status *project.GitStatus
	err := p.pool.Run(ctx, func() error {
		status = &project.GitStatus{}

		// Get SVN info for current revision
		info, err := p.runSVN(ctx, repoPath, "info", "--show-item", "revision")
		if err != nil {
			return fmt.Errorf("svn: info: %w", err)
		}
		status.CommitHash = strings.TrimSpace(info)

		// Get last log entry
		logOut, err := p.runSVN(ctx, repoPath, "log", "-l", "1", "--non-interactive")
		if err == nil {
			lines := strings.Split(strings.TrimSpace(logOut), "\n")
			// SVN log format: separator, metadata, blank, message, separator
			if len(lines) >= 4 {
				status.CommitMessage = strings.TrimSpace(lines[3])
			}
		}

		// Get URL as branch name
		urlOut, err := p.runSVN(ctx, repoPath, "info", "--show-item", "relative-url")
		if err == nil {
			relURL := strings.TrimSpace(urlOut)
			status.Branch = relURL
		}

		// Check for modified/untracked files
		st, err := p.runSVN(ctx, repoPath, "status")
		if err != nil {
			return fmt.Errorf("svn: status: %w", err)
		}
		for _, line := range strings.Split(st, "\n") {
			if len(line) < 2 {
				continue
			}
			indicator := line[0]
			file := strings.TrimSpace(line[1:])
			if file == "" {
				continue
			}
			switch indicator {
			case '?':
				status.Untracked = append(status.Untracked, file)
			case 'M', 'A', 'D', 'C', 'R':
				status.Modified = append(status.Modified, file)
			}
		}
		status.Dirty = len(status.Modified) > 0 || len(status.Untracked) > 0

		return nil
	})
	return status, err
}

// Pull updates an SVN working copy (svn update).
func (p *Provider) Pull(ctx context.Context, repoPath string) error {
	return p.pool.Run(ctx, func() error {
		if _, err := p.runSVN(ctx, repoPath, "update", "--non-interactive"); err != nil {
			return fmt.Errorf("svn: update: %w", err)
		}
		return nil
	})
}

// ListBranches lists SVN branches by listing the branches/ directory.
func (p *Provider) ListBranches(ctx context.Context, repoPath string) ([]project.Branch, error) {
	var branches []project.Branch
	err := p.pool.Run(ctx, func() error {
		// Get repo root URL
		rootURL, err := p.runSVN(ctx, repoPath, "info", "--show-item", "repos-root-url")
		if err != nil {
			return fmt.Errorf("svn: get repo root: %w", err)
		}
		rootURL = strings.TrimSpace(rootURL)

		// List branches
		branchesURL := rootURL + "/branches"
		out, err := p.runSVN(ctx, "", "ls", branchesURL, "--non-interactive")
		if err != nil {
			// No branches directory -- return trunk only
			branches = append(branches, project.Branch{Name: "trunk", Current: true})
			return nil
		}

		// Get current relative URL
		curURL, _ := p.runSVN(ctx, repoPath, "info", "--show-item", "relative-url")
		curURL = strings.TrimSpace(curURL)

		branches = append(branches, project.Branch{
			Name:    "trunk",
			Current: strings.Contains(curURL, "trunk"),
		})

		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSuffix(strings.TrimSpace(line), "/")
			if line == "" {
				continue
			}
			branches = append(branches, project.Branch{
				Name:    line,
				Current: strings.Contains(curURL, "branches/"+line),
			})
		}

		return nil
	})
	return branches, err
}

// Checkout switches to a different SVN branch by doing svn switch.
func (p *Provider) Checkout(ctx context.Context, repoPath, branch string) error {
	return p.pool.Run(ctx, func() error {
		rootURL, err := p.runSVN(ctx, repoPath, "info", "--show-item", "repos-root-url")
		if err != nil {
			return fmt.Errorf("svn: get repo root: %w", err)
		}
		rootURL = strings.TrimSpace(rootURL)

		var targetURL string
		if branch == "trunk" {
			targetURL = rootURL + "/trunk"
		} else {
			targetURL = rootURL + "/branches/" + branch
		}

		if _, err := p.runSVN(ctx, repoPath, "switch", targetURL, "--non-interactive"); err != nil {
			return fmt.Errorf("svn: switch to %s: %w", branch, err)
		}
		return nil
	})
}

// runSVN executes an svn command and returns stdout.
func (p *Provider) runSVN(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := p.execCommand(ctx, "svn", args...)
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

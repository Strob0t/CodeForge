// Package github implements a gitprovider.Provider that uses the GitHub REST API
// for repository listing and token-authenticated clone URLs, while delegating
// local git operations (status, pull, branches, checkout) to the git CLI.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

const providerName = "github-api"

// Provider implements gitprovider.Provider for GitHub using the REST API
// for listing repos and token-based clone URLs.
type Provider struct {
	token      string
	baseURL    string // GitHub API base URL (default: https://api.github.com)
	httpClient *http.Client
}

// NewProvider creates a GitHub API provider with the given token and base URL.
func NewProvider(token, baseURL string) *Provider {
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}
	return &Provider{
		token:   token,
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *Provider) Name() string { return providerName }

func (p *Provider) Capabilities() gitprovider.Capabilities {
	return gitprovider.Capabilities{
		Clone:       true,
		Push:        true,
		PullRequest: true,
		Webhook:     true,
		Issues:      true,
	}
}

// CloneURL returns a token-authenticated HTTPS clone URL for the given repo.
// Security: Token is embedded in URL (standard git HTTPS auth pattern, used by
// GitHub Actions and GitLab CI). This URL must never be logged or displayed to
// users. For interactive use, prefer SSH keys or credential helpers.
func (p *Provider) CloneURL(_ context.Context, repo string) (string, error) {
	if repo == "" {
		return "", fmt.Errorf("github: empty repository identifier")
	}
	host := "github.com"
	if p.baseURL != "" && p.baseURL != "https://api.github.com" {
		// GitHub Enterprise: extract host from baseURL
		host = strings.TrimPrefix(p.baseURL, "https://")
		host = strings.TrimPrefix(host, "http://")
		host = strings.SplitN(host, "/", 2)[0]
	}
	return fmt.Sprintf("https://x-access-token:%s@%s/%s.git", p.token, host, repo), nil
}

// ghRepo is the minimal subset of the GitHub API repo response we need.
type ghRepo struct {
	FullName string `json:"full_name"`
}

// ListRepos lists all repositories accessible to the authenticated user,
// handling pagination via the Link header.
func (p *Provider) ListRepos(ctx context.Context) ([]string, error) {
	var repos []string
	url := fmt.Sprintf("%s/user/repos?per_page=100&sort=updated", p.baseURL)

	for url != "" {
		body, nextURL, err := p.doGetPaginated(ctx, url)
		if err != nil {
			return nil, fmt.Errorf("github: list repos: %w", err)
		}

		var page []ghRepo
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("github: parse repos response: %w", err)
		}

		for i := range page {
			repos = append(repos, page[i].FullName)
		}
		url = nextURL
	}

	return repos, nil
}

// Clone clones a repository to the given local path using git CLI.
func (p *Provider) Clone(ctx context.Context, url, destPath string, opts ...gitprovider.CloneOption) error {
	absPath, err := filepath.Abs(destPath)
	if err != nil {
		return fmt.Errorf("github: resolve path: %w", err)
	}

	o := gitprovider.ApplyCloneOptions(opts)
	args := []string{"clone"}
	if o.Branch != "" {
		args = append(args, "--branch", o.Branch, "--single-branch")
	}
	args = append(args, url, absPath)

	if _, execErr := runGit(ctx, "", args...); execErr != nil {
		return fmt.Errorf("github: clone: %w", execErr)
	}
	return nil
}

// Status returns the git status of a local repository (delegates to git CLI).
func (p *Provider) Status(ctx context.Context, repoPath string) (*project.GitStatus, error) {
	status := &project.GitStatus{}

	branch, err := runGit(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("github: get branch: %w", err)
	}
	status.Branch = strings.TrimSpace(branch)

	logOut, err := runGit(ctx, repoPath, "log", "-1", "--format=%H%n%s")
	if err == nil {
		lines := strings.SplitN(strings.TrimSpace(logOut), "\n", 2)
		if len(lines) >= 1 {
			status.CommitHash = lines[0]
		}
		if len(lines) >= 2 {
			status.CommitMessage = lines[1]
		}
	}

	porcelain, err := runGit(ctx, repoPath, "status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("github: porcelain status: %w", err)
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

	revList, _ := runGit(ctx, repoPath, "rev-list", "--left-right", "--count", "@{upstream}...HEAD")
	if parts := strings.Fields(strings.TrimSpace(revList)); len(parts) == 2 {
		_, _ = fmt.Sscanf(parts[0], "%d", &status.Behind)
		_, _ = fmt.Sscanf(parts[1], "%d", &status.Ahead)
	}

	return status, nil
}

// Pull fetches and merges updates for the given repository.
func (p *Provider) Pull(ctx context.Context, repoPath string) error {
	if _, err := runGit(ctx, repoPath, "pull"); err != nil {
		return fmt.Errorf("github: pull: %w", err)
	}
	return nil
}

// ListBranches returns all branches of a local repository.
func (p *Provider) ListBranches(ctx context.Context, repoPath string) ([]project.Branch, error) {
	out, err := runGit(ctx, repoPath, "branch", "--list")
	if err != nil {
		return nil, fmt.Errorf("github: list branches: %w", err)
	}

	var branches []project.Branch
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
	return branches, nil
}

// Checkout switches to the specified branch.
func (p *Provider) Checkout(ctx context.Context, repoPath, branch string) error {
	if _, err := runGit(ctx, repoPath, "checkout", branch); err != nil {
		return fmt.Errorf("github: checkout %s: %w", branch, err)
	}
	return nil
}

// doGetPaginated performs a GET request and returns the body + the "next" URL from the Link header.
func (p *Provider) doGetPaginated(ctx context.Context, url string) (body []byte, nextURL string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}

	resp, err := p.httpClient.Do(req) //nolint:gosec // G704: url is constructed internally from GitHub API base URL
	if err != nil {
		return nil, "", fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("github API %d", resp.StatusCode)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, "", fmt.Errorf("read response: %w", err)
	}

	nextURL = parseLinkNext(resp.Header.Get("Link"))
	return buf.Bytes(), nextURL, nil
}

// parseLinkNext extracts the "next" URL from a GitHub Link header.
func parseLinkNext(header string) string {
	if header == "" {
		return ""
	}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="next"`) {
			continue
		}
		start := strings.Index(part, "<")
		end := strings.Index(part, ">")
		if start >= 0 && end > start {
			return part[start+1 : end]
		}
	}
	return ""
}

// runGit executes a git command and returns its combined stdout.
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // G204: args are controlled by caller (internal git operations)
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

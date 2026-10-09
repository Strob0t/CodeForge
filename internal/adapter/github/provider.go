// Package github implements a gitprovider.Provider that uses the GitHub REST API
// for repository listing, pull requests (PR delivery, KI-117) and
// token-authenticated clone URLs, while delegating local git operations
// (status, pull, branches, checkout) to the git CLI.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/Strob0t/CodeForge/internal/adapter/githubapi"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/netutil"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

const providerName = "github-api"

// maxRepoPages bounds ListRepos: up to 5,000 repositories.
const maxRepoPages = 50

// Provider implements gitprovider.Provider for GitHub using the REST API
// for listing repos, opening pull requests and token-based clone URLs.
type Provider struct {
	token      string
	baseURL    string // GitHub API base URL (default: https://api.github.com)
	httpClient *http.Client
}

var _ gitprovider.PullRequestCreator = (*Provider)(nil)

// apiClient is the HTTP client of every provider NewProvider creates: the
// base URL is project configuration, which tenants write, so it connects
// only to the addresses its outbound policy allows (public ones, plus the
// private hosts of pm.allowed_private_hosts, KI-166) and follows redirects
// only within the API's origin.
var apiClient atomic.Pointer[http.Client]

func init() {
	apiClient.Store(githubapi.PublicHTTPClient())
}

// SetOutboundPolicy makes the providers created from now on connect through
// policy; the Go Core builds it from pm.allowed_private_hosts at startup.
func SetOutboundPolicy(policy *netutil.OutboundPolicy) {
	apiClient.Store(githubapi.NewHTTPClient(policy))
}

// NewProvider creates a GitHub API provider with the given token and base
// URL.
func NewProvider(token, baseURL string) *Provider {
	return newProvider(token, baseURL, apiClient.Load())
}

func newProvider(token, baseURL string, httpClient *http.Client) *Provider {
	if baseURL == "" {
		baseURL = githubapi.DefaultBaseURL
	}
	return &Provider{
		token:      token,
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		httpClient: httpClient,
	}
}

func (p *Provider) api() (*githubapi.Client, error) {
	return githubapi.NewClient(p.baseURL, p.token, p.httpClient)
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

// ListRepos lists the repositories accessible to the authenticated user,
// following the pagination within the API's origin.
func (p *Provider) ListRepos(ctx context.Context) ([]string, error) {
	api, err := p.api()
	if err != nil {
		return nil, fmt.Errorf("github: list repos: %w", err)
	}
	list, more, err := githubapi.List[ghRepo](ctx, api, "/user/repos?per_page=100&sort=updated", maxRepoPages)
	if err != nil {
		return nil, fmt.Errorf("github: list repos: %w", err)
	}
	if more {
		slog.WarnContext(ctx, "github repository listing truncated", "pages", maxRepoPages)
	}
	repos := make([]string, 0, len(list))
	for i := range list {
		repos = append(repos, list[i].FullName)
	}
	return repos, nil
}

// CreatePullRequest opens pr through the REST API and returns its web URL.
// Without a base it targets the repository's default branch, as gh did.
func (p *Provider) CreatePullRequest(ctx context.Context, pr *gitprovider.PullRequest) (string, error) {
	if pr.Head == "" || pr.Title == "" {
		return "", fmt.Errorf("%w: a pull request needs a head branch and a title", domain.ErrValidation)
	}
	repoPath, err := githubapi.RepoPath(pr.Repo)
	if err != nil {
		return "", err
	}
	api, err := p.api()
	if err != nil {
		return "", err
	}
	base := pr.Base
	if base == "" {
		page, err := api.Do(ctx, http.MethodGet, repoPath, nil)
		if err != nil {
			return "", fmt.Errorf("github: repository %s: %w", pr.Repo, err)
		}
		repo, err := githubapi.Decode[struct {
			DefaultBranch string `json:"default_branch"`
		}](page.Body)
		if err != nil {
			return "", err
		}
		if repo.DefaultBranch == "" {
			return "", fmt.Errorf("github: repository %s has no default branch", pr.Repo)
		}
		base = repo.DefaultBranch
	}
	body, err := json.Marshal(struct {
		Title string `json:"title"`
		Head  string `json:"head"`
		Base  string `json:"base"`
		Body  string `json:"body,omitempty"`
	}{pr.Title, pr.Head, base, pr.Body})
	if err != nil {
		return "", fmt.Errorf("github: marshal pull request: %w", err)
	}
	page, err := api.Do(ctx, http.MethodPost, repoPath+"/pulls", body)
	if err != nil {
		return "", fmt.Errorf("github: create pull request: %w", err)
	}
	created, err := githubapi.Decode[struct {
		HTMLURL string `json:"html_url"`
	}](page.Body)
	if err != nil {
		return "", err
	}
	if created.HTMLURL == "" {
		return "", fmt.Errorf("github: create pull request: the answer carries no pull request URL")
	}
	return created.HTMLURL, nil
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

	porcelain, err := runGit(ctx, repoPath, "status", "--porcelain", "--ignore-submodules=all")
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
	if _, err := runGit(ctx, repoPath, "pull", "--no-recurse-submodules"); err != nil {
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

// Checkout switches to the specified branch. Only a valid branch name
// reaches git, and `git switch --end-of-options` reads it as nothing but a
// branch: with `git checkout`, "-f", "." or a file name would discard
// uncommitted changes (KI-189).
func (p *Provider) Checkout(ctx context.Context, repoPath, branch string) error {
	if err := git.CheckBranchName(ctx, branch); err != nil {
		return fmt.Errorf("github: checkout: %w: %w", domain.ErrValidation, err)
	}
	if _, err := runGit(ctx, repoPath, "switch", "--end-of-options", branch); err != nil {
		return fmt.Errorf("github: checkout %s: %w", branch, err)
	}
	return nil
}

// runGit runs git hardened in the workspace repository at dir, or outside any
// repository when dir is "" (clone).
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	return git.RunIn(ctx, dir, args...) // hardened: workspaces are agent-writable (KI-77)
}

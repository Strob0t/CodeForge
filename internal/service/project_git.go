package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/netutil"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

// resolveGitProvider creates a git provider for the given project.
// For local projects with an empty provider field, it defaults to "local".
func resolveGitProvider(p *project.Project) (gitprovider.Provider, error) {
	name := p.Provider
	if name == "" {
		name = "local"
	}
	return gitprovider.New(name, p.Config)
}

// repoInfoClient is the shared HTTP client for repo info API calls.
var repoInfoClient = &http.Client{Timeout: 10 * time.Second, Transport: netutil.SafeTransport()}

// Status returns the git status of a project's workspace.
func (s *ProjectService) Status(ctx context.Context, id string) (*project.GitStatus, error) {
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if p.WorkspacePath == "" {
		return nil, fmt.Errorf("%w: project %s has no workspace (not cloned)", domain.ErrValidation, id)
	}

	gp, err := resolveGitProvider(p)
	if err != nil {
		return nil, fmt.Errorf("create git provider: %w", err)
	}

	return gp.Status(ctx, p.WorkspacePath)
}

// Pull fetches and merges updates for a project's workspace.
func (s *ProjectService) Pull(ctx context.Context, id string) error {
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}
	if p.WorkspacePath == "" {
		return fmt.Errorf("%w: project %s has no workspace (not cloned)", domain.ErrValidation, id)
	}

	gp, err := resolveGitProvider(p)
	if err != nil {
		return fmt.Errorf("create git provider: %w", err)
	}

	if err := gp.Pull(ctx, p.WorkspacePath); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "no remote") ||
			strings.Contains(errMsg, "does not have a default remote") ||
			strings.Contains(errMsg, "No remote repository specified") ||
			strings.Contains(errMsg, "no such remote") ||
			strings.Contains(errMsg, "no tracking information") {
			return fmt.Errorf("%w: no remote configured for this project — add a remote with 'git remote add origin <url>'", domain.ErrValidation)
		}
		return fmt.Errorf("git pull: %w", err)
	}
	return nil
}

// ListBranches returns all branches of a project's workspace.
func (s *ProjectService) ListBranches(ctx context.Context, id string) ([]project.Branch, error) {
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if p.WorkspacePath == "" {
		return nil, fmt.Errorf("%w: project %s has no workspace (not cloned)", domain.ErrValidation, id)
	}

	gp, err := resolveGitProvider(p)
	if err != nil {
		return nil, fmt.Errorf("create git provider: %w", err)
	}

	return gp.ListBranches(ctx, p.WorkspacePath)
}

// Checkout switches a project's workspace to the specified branch.
func (s *ProjectService) Checkout(ctx context.Context, id, branch string) error {
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}
	if p.WorkspacePath == "" {
		return fmt.Errorf("%w: project %s has no workspace (not cloned)", domain.ErrValidation, id)
	}

	gp, err := resolveGitProvider(p)
	if err != nil {
		return fmt.Errorf("create git provider: %w", err)
	}

	return gp.Checkout(ctx, p.WorkspacePath, branch)
}

// FetchRepoInfo queries the hosting platform's public API to retrieve
// repository metadata (name, description, default branch, language, etc.).
// It parses the URL to determine the provider, then makes the appropriate API call.
func (s *ProjectService) FetchRepoInfo(ctx context.Context, repoURL string) (*project.RepoInfo, error) {
	parsed, err := project.ParseRepoURL(repoURL)
	if err != nil {
		return nil, fmt.Errorf("parse repo URL: %w", err)
	}

	switch parsed.Provider {
	case "github":
		return s.fetchGitHubRepoInfo(ctx, parsed)
	case "gitlab":
		return s.fetchGitLabRepoInfo(ctx, parsed)
	case "gitea":
		return s.fetchGiteaRepoInfo(ctx, parsed)
	default:
		return nil, fmt.Errorf("unsupported provider for repo info: %q (host: %s)", parsed.Provider, parsed.Host)
	}
}

// fetchGitHubRepoInfo fetches repo info from the GitHub API.
func (s *ProjectService) fetchGitHubRepoInfo(ctx context.Context, parsed *project.ParsedRepoURL) (*project.RepoInfo, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", parsed.Owner, parsed.Repo)

	var resp struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		DefaultBranch string `json:"default_branch"`
		Language      string `json:"language"`
		Stars         int    `json:"stargazers_count"`
		Private       bool   `json:"private"`
	}

	if err := fetchJSON(ctx, apiURL, &resp); err != nil {
		return nil, fmt.Errorf("github API: %w", err)
	}

	return &project.RepoInfo{
		Name:          resp.Name,
		Description:   resp.Description,
		DefaultBranch: resp.DefaultBranch,
		Language:      resp.Language,
		Stars:         resp.Stars,
		Private:       resp.Private,
	}, nil
}

// fetchGitLabRepoInfo fetches repo info from the GitLab API.
func (s *ProjectService) fetchGitLabRepoInfo(ctx context.Context, parsed *project.ParsedRepoURL) (*project.RepoInfo, error) {
	// GitLab uses URL-encoded project path as ID.
	projectPath := parsed.Owner + "%2F" + parsed.Repo
	host := parsed.Host
	if host == "" {
		host = "gitlab.com"
	}
	apiURL := fmt.Sprintf("https://%s/api/v4/projects/%s", host, projectPath)

	var resp struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		DefaultBranch string `json:"default_branch"`
		Stars         int    `json:"star_count"`
		Visibility    string `json:"visibility"`
	}

	if err := fetchJSON(ctx, apiURL, &resp); err != nil {
		return nil, fmt.Errorf("gitlab API: %w", err)
	}

	return &project.RepoInfo{
		Name:          resp.Name,
		Description:   resp.Description,
		DefaultBranch: resp.DefaultBranch,
		Stars:         resp.Stars,
		Private:       resp.Visibility != "public",
	}, nil
}

// isAllowedGiteaHost checks whether the given host resolves to a public IP.
// It blocks requests to loopback, private, and link-local addresses to prevent SSRF.
func isAllowedGiteaHost(ctx context.Context, host string) bool {
	h := host
	// Strip port if present.
	if i := strings.LastIndex(h, ":"); i != -1 {
		h = h[:i]
	}

	// Block known cloud metadata endpoints.
	if h == "169.254.169.254" || h == "metadata.google.internal" {
		return false
	}

	// Resolve hostname and check all IPs.
	addrs, lookupErr := net.DefaultResolver.LookupIPAddr(ctx, h)
	if lookupErr != nil {
		// If lookup fails, try parsing as literal IP.
		ip := net.ParseIP(h)
		if ip == nil {
			return false
		}
		return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast()
	}

	for _, addr := range addrs {
		if addr.IP.IsLoopback() || addr.IP.IsPrivate() || addr.IP.IsLinkLocalUnicast() || addr.IP.IsLinkLocalMulticast() {
			return false
		}
	}
	return true
}

// fetchGiteaRepoInfo fetches repo info from the Gitea/Forgejo API (GitHub-compatible).
func (s *ProjectService) fetchGiteaRepoInfo(ctx context.Context, parsed *project.ParsedRepoURL) (*project.RepoInfo, error) {
	host := parsed.Host
	if host == "" {
		return nil, fmt.Errorf("gitea: host is required")
	}
	if !isAllowedGiteaHost(ctx, host) {
		return nil, fmt.Errorf("gitea: host %q is not allowed (private/loopback address)", host)
	}
	apiURL := fmt.Sprintf("https://%s/api/v1/repos/%s/%s", host, parsed.Owner, parsed.Repo)

	var resp struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		DefaultBranch string `json:"default_branch"`
		Language      string `json:"language"`
		Stars         int    `json:"stars_count"`
		Private       bool   `json:"private"`
	}

	if err := fetchJSON(ctx, apiURL, &resp); err != nil {
		return nil, fmt.Errorf("gitea API: %w", err)
	}

	return &project.RepoInfo{
		Name:          resp.Name,
		Description:   resp.Description,
		DefaultBranch: resp.DefaultBranch,
		Language:      resp.Language,
		Stars:         resp.Stars,
		Private:       resp.Private,
	}, nil
}

// fetchJSON performs a GET request and decodes the JSON response body.
func fetchJSON(ctx context.Context, rawURL string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "CodeForge/1.0")

	resp, err := repoInfoClient.Do(req) //nolint:gosec // URL from validated project repo config
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if readErr != nil {
			return fmt.Errorf("HTTP %d (reading body: %w)", resp.StatusCode, readErr)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// ListRemoteBranches queries a remote repository for its branch names.
// It validates the URL (scheme allowlist, host present) and runs git ls-remote.
func (s *ProjectService) ListRemoteBranches(ctx context.Context, repoURL string) ([]string, error) {
	if repoURL == "" {
		return nil, fmt.Errorf("list remote branches: url is required")
	}

	parsed, err := url.Parse(repoURL)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("list remote branches: invalid repository URL")
	}
	switch parsed.Scheme {
	case "https", "http", "git", "ssh":
		// allowed
	default:
		return nil, fmt.Errorf("list remote branches: unsupported URL scheme: only https, http, git, ssh are allowed")
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "git", "ls-remote", "--heads", repoURL) //nolint:gosec // repoURL validated: parsed URL with scheme allowlist.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		slog.Warn("git ls-remote failed", "url", repoURL, "error", err, "stderr", stderr.String())
		return nil, fmt.Errorf("list remote branches: git ls-remote failed: %w", err)
	}

	var branches []string
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: <sha>\trefs/heads/<branch-name>
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		ref := parts[1]
		branch := strings.TrimPrefix(ref, "refs/heads/")
		if branch != ref {
			branches = append(branches, branch)
		}
	}

	return branches, nil
}

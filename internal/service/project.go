// Package service implements business logic on top of ports.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

// SpecDetector is an optional interface for detecting and importing roadmap specs
// during automated project setup. Implemented by RoadmapService via specDetectorAdapter.
type SpecDetector interface {
	DetectAndImport(ctx context.Context, projectID string) (detected bool, importErr error)
}

// ProjectService handles project business logic.
type ProjectService struct {
	store         database.Store
	workspaceRoot string
	specDetector  SpecDetector
	goalDiscovery *GoalDiscoveryService
}

// NewProjectService creates a new ProjectService.
func NewProjectService(store database.Store, workspaceRoot string) *ProjectService {
	return &ProjectService{store: store, workspaceRoot: workspaceRoot}
}

// SetSpecDetector sets the optional spec detector for automated setup.
func (s *ProjectService) SetSpecDetector(sd SpecDetector) {
	s.specDetector = sd
}

// SetGoalDiscovery sets the optional goal discovery service for automated setup.
func (s *ProjectService) SetGoalDiscovery(svc *GoalDiscoveryService) {
	s.goalDiscovery = svc
}

// resolveGitProvider creates a git provider for the given project.
// For local projects with an empty provider field, it defaults to "local".
func resolveGitProvider(p *project.Project) (gitprovider.Provider, error) {
	name := p.Provider
	if name == "" {
		name = "local"
	}
	return gitprovider.New(name, p.Config)
}

// List returns all projects.
func (s *ProjectService) List(ctx context.Context) ([]project.Project, error) {
	return s.store.ListProjects(ctx)
}

// Get returns a project by ID.
func (s *ProjectService) Get(ctx context.Context, id string) (*project.Project, error) {
	return s.store.GetProject(ctx, id)
}

// Create creates a new project after validating the request.
func (s *ProjectService) Create(ctx context.Context, req *project.CreateRequest) (*project.Project, error) {
	if err := project.ValidateCreateRequest(req, gitprovider.Available()); err != nil {
		return nil, err
	}
	return s.store.CreateProject(ctx, req)
}

// Update applies partial updates to a project.
func (s *ProjectService) Update(ctx context.Context, id string, req project.UpdateRequest) (*project.Project, error) {
	if err := project.ValidateUpdateRequest(req); err != nil {
		return nil, err
	}

	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		p.Name = *req.Name
	}
	if req.Description != nil {
		p.Description = *req.Description
	}
	if req.RepoURL != nil {
		p.RepoURL = *req.RepoURL
	}
	if req.Provider != nil {
		p.Provider = *req.Provider
	}
	if req.Config != nil {
		p.Config = req.Config
	}

	if err := s.store.UpdateProject(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// Delete removes a project and cleans up its workspace directory.
func (s *ProjectService) Delete(ctx context.Context, id string) error {
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return s.store.DeleteProject(ctx, id)
	}

	wsPath := p.WorkspacePath

	if err := s.store.DeleteProject(ctx, id); err != nil {
		return err
	}

	if wsPath != "" && s.isUnderWorkspaceRoot(wsPath) {
		if rmErr := os.RemoveAll(wsPath); rmErr != nil {
			slog.Warn("failed to remove workspace directory",
				"project_id", id,
				"path", wsPath,
				"error", rmErr,
			)
		}
	}

	return nil
}

// Clone clones a project's repository to the workspace directory.
// The tenantID is used to isolate workspaces per tenant.
// An optional branch can be specified to clone only that branch.
func (s *ProjectService) Clone(ctx context.Context, id, tenantID, branch string) (*project.Project, error) {
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if p.RepoURL == "" {
		return nil, fmt.Errorf("project %s has no repo_url", id)
	}

	gp, err := resolveGitProvider(p)
	if err != nil {
		return nil, fmt.Errorf("create git provider: %w", err)
	}

	var opts []gitprovider.CloneOption
	if branch != "" {
		opts = append(opts, gitprovider.WithBranch(branch))
	}

	destPath := filepath.Join(s.workspaceRoot, tenantID, p.ID)
	if err := gp.Clone(ctx, p.RepoURL, destPath, opts...); err != nil {
		return nil, fmt.Errorf("clone: %w", err)
	}

	p.WorkspacePath = destPath
	if err := s.store.UpdateProject(ctx, p); err != nil {
		return nil, fmt.Errorf("update project workspace: %w", err)
	}

	return p, nil
}

// Adopt sets an existing directory as the project's workspace without cloning.
func (s *ProjectService) Adopt(ctx context.Context, id, path string) (*project.Project, error) {
	if path == "" {
		return nil, fmt.Errorf("adopt: path is required")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("adopt: resolve path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("adopt: directory does not exist: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("adopt: %s is not a directory", absPath)
	}

	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	p.WorkspacePath = absPath
	if err := s.store.UpdateProject(ctx, p); err != nil {
		return nil, fmt.Errorf("update project workspace: %w", err)
	}

	return p, nil
}

// InitWorkspace creates an empty workspace directory with git init for projects
// that have no repo_url and no adopted path. The directory is created under
// {workspaceRoot}/{tenantID}/{projectID}.
func (s *ProjectService) InitWorkspace(ctx context.Context, id, tenantID string) (*project.Project, error) {
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	if p.WorkspacePath != "" {
		return nil, fmt.Errorf("project %s already has a workspace at %s", id, p.WorkspacePath)
	}

	destPath := filepath.Join(s.workspaceRoot, tenantID, p.ID)
	if err := os.MkdirAll(destPath, 0o750); err != nil {
		return nil, fmt.Errorf("create workspace directory: %w", err)
	}

	// Initialize a git repository so agents can work with version control.
	if gitErr := exec.CommandContext(ctx, "git", "init", destPath).Run(); gitErr != nil { //nolint:gosec // destPath is constructed from workspaceRoot/tenantID/projectID, not user input
		// Clean up on failure.
		_ = os.RemoveAll(destPath)
		return nil, fmt.Errorf("git init: %w", gitErr)
	}

	p.WorkspacePath = destPath
	if err := s.store.UpdateProject(ctx, p); err != nil {
		_ = os.RemoveAll(destPath)
		return nil, fmt.Errorf("update project workspace: %w", err)
	}

	return p, nil
}

// WorkspaceHealth returns health and status information about a project's workspace.
func (s *ProjectService) WorkspaceHealth(ctx context.Context, id string) (*project.WorkspaceInfo, error) {
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	info := &project.WorkspaceInfo{Path: p.WorkspacePath}
	if p.WorkspacePath == "" {
		return info, nil
	}

	stat, err := os.Stat(p.WorkspacePath)
	if err != nil {
		return info, nil
	}
	info.Exists = true
	info.LastModified = stat.ModTime()

	// Check for .git directory.
	if gitStat, gitErr := os.Stat(filepath.Join(p.WorkspacePath, ".git")); gitErr == nil && gitStat.IsDir() {
		info.GitRepo = true
	}

	// Compute disk usage.
	var totalSize int64
	_ = filepath.WalkDir(p.WorkspacePath, func(_ string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil //nolint:nilerr // skip unreadable entries
		}
		if !d.IsDir() {
			if fi, fiErr := d.Info(); fiErr == nil {
				totalSize += fi.Size()
			}
		}
		return nil
	})
	info.DiskUsageBytes = totalSize

	return info, nil
}

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

	return gp.Pull(ctx, p.WorkspacePath)
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

// DetectStack scans an existing project's workspace and returns stack detection results.
func (s *ProjectService) DetectStack(ctx context.Context, id string) (*project.StackDetectionResult, error) {
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if p.WorkspacePath == "" {
		return nil, fmt.Errorf("project %s has no workspace (not cloned)", id)
	}
	return project.ScanWorkspace(p.WorkspacePath)
}

// DetectStackByPath scans an arbitrary directory path for language detection.
func (s *ProjectService) DetectStackByPath(_ context.Context, path string) (*project.StackDetectionResult, error) {
	if path == "" {
		return nil, fmt.Errorf("detect stack: path is required")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("detect stack: resolve path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("detect stack: directory does not exist: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("detect stack: %s is not a directory", absPath)
	}

	return project.ScanWorkspace(absPath)
}

// isUnderWorkspaceRoot validates that the path is under the workspace root
// to prevent accidental deletion of unrelated directories.
func (s *ProjectService) isUnderWorkspaceRoot(path string) bool {
	absRoot, err := filepath.Abs(s.workspaceRoot)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return strings.HasPrefix(absPath, absRoot+string(filepath.Separator))
}

// SetupProject chains: clone -> detect stack -> detect specs -> import specs.
// Each step is idempotent; failures are logged but don't abort the chain.
// An optional branch can be specified to clone only that branch.
func (s *ProjectService) SetupProject(ctx context.Context, id, tenantID, branch string) (*project.SetupResult, error) {
	result := &project.SetupResult{}

	// Step 1: Clone (skip if workspace already exists).
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	switch {
	case p.WorkspacePath != "":
		result.Cloned = true
		result.Steps = append(result.Steps, project.SetupStep{
			Name:   "clone",
			Status: "skipped",
		})
	case p.RepoURL == "":
		inited, initErr := s.InitWorkspace(ctx, id, tenantID)
		if initErr != nil {
			slog.Warn("setup: init workspace failed", "project_id", id, "error", initErr)
			result.Steps = append(result.Steps, project.SetupStep{
				Name:   "init-workspace",
				Status: "failed",
				Error:  initErr.Error(),
			})
		} else {
			p = inited
			result.Steps = append(result.Steps, project.SetupStep{
				Name:   "init-workspace",
				Status: "completed",
			})
		}
	default:
		cloned, cloneErr := s.Clone(ctx, id, tenantID, branch)
		if cloneErr != nil {
			slog.Warn("setup: clone failed", "project_id", id, "error", cloneErr)
			result.Steps = append(result.Steps, project.SetupStep{
				Name:   "clone",
				Status: "failed",
				Error:  cloneErr.Error(),
			})
		} else {
			result.Cloned = true
			p = cloned
			result.Steps = append(result.Steps, project.SetupStep{
				Name:   "clone",
				Status: "completed",
			})
		}
	}

	// Step 2: Detect stack (requires workspace).
	if p.WorkspacePath != "" {
		stack, stackErr := project.ScanWorkspace(p.WorkspacePath)
		if stackErr != nil {
			slog.Warn("setup: stack detection failed", "project_id", id, "error", stackErr)
			result.Steps = append(result.Steps, project.SetupStep{
				Name:   "detect_stack",
				Status: "failed",
				Error:  stackErr.Error(),
			})
		} else {
			result.StackDetected = true
			result.Stack = stack
			result.Steps = append(result.Steps, project.SetupStep{
				Name:   "detect_stack",
				Status: "completed",
			})
		}
	} else {
		result.Steps = append(result.Steps, project.SetupStep{
			Name:   "detect_stack",
			Status: "skipped",
			Error:  "no workspace available",
		})
	}

	// Step 3: Detect and import specs (requires workspace + spec detector).
	switch {
	case p.WorkspacePath != "" && s.specDetector != nil:
		detected, importErr := s.specDetector.DetectAndImport(ctx, id)
		switch {
		case importErr != nil:
			slog.Warn("setup: spec import failed", "project_id", id, "error", importErr)
			result.Steps = append(result.Steps, project.SetupStep{
				Name:   "import_specs",
				Status: "failed",
				Error:  importErr.Error(),
			})
		case detected:
			result.SpecsDetected = true
			result.Steps = append(result.Steps, project.SetupStep{
				Name:   "import_specs",
				Status: "completed",
			})
		default:
			result.Steps = append(result.Steps, project.SetupStep{
				Name:   "import_specs",
				Status: "skipped",
				Error:  "no specs found",
			})
		}
	case s.specDetector == nil:
		result.Steps = append(result.Steps, project.SetupStep{
			Name:   "import_specs",
			Status: "skipped",
			Error:  "spec detector not configured",
		})
	default:
		result.Steps = append(result.Steps, project.SetupStep{
			Name:   "import_specs",
			Status: "skipped",
			Error:  "no workspace available",
		})
	}

	// Step 4: Discover project goals (requires workspace + goal discovery service).
	switch {
	case p.WorkspacePath != "" && s.goalDiscovery != nil:
		goalResult, goalErr := s.goalDiscovery.DetectAndImport(ctx, id, p.WorkspacePath)
		switch {
		case goalErr != nil:
			slog.Warn("setup: goal discovery failed", "project_id", id, "error", goalErr)
			result.Steps = append(result.Steps, project.SetupStep{
				Name:   "discover_goals",
				Status: "failed",
				Error:  goalErr.Error(),
			})
		case goalResult.GoalsCreated > 0:
			result.Steps = append(result.Steps, project.SetupStep{
				Name:   "discover_goals",
				Status: "completed",
			})
		default:
			result.Steps = append(result.Steps, project.SetupStep{
				Name:   "discover_goals",
				Status: "skipped",
				Error:  "no goal files found",
			})
		}
	case s.goalDiscovery == nil:
		result.Steps = append(result.Steps, project.SetupStep{
			Name:   "discover_goals",
			Status: "skipped",
			Error:  "goal discovery not configured",
		})
	default:
		result.Steps = append(result.Steps, project.SetupStep{
			Name:   "discover_goals",
			Status: "skipped",
			Error:  "no workspace available",
		})
	}

	return result, nil
}

// repoInfoClient is the shared HTTP client for repo info API calls.
var repoInfoClient = &http.Client{Timeout: 10 * time.Second}

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
func fetchJSON(ctx context.Context, url string, dest interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
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
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

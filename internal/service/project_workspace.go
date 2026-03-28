package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

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

	// Restrict adoption to within the workspace root.
	if s.workspaceRoot != "" {
		wsRoot, _ := filepath.Abs(s.workspaceRoot)
		if !strings.HasPrefix(absPath, wsRoot+string(filepath.Separator)) && absPath != wsRoot {
			return nil, fmt.Errorf("path must be within workspace root %s", wsRoot)
		}
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

	// Restrict to workspace root to prevent filesystem probing.
	if s.workspaceRoot != "" {
		wsRoot, _ := filepath.Abs(s.workspaceRoot)
		if !strings.HasPrefix(absPath, wsRoot+string(filepath.Separator)) && absPath != wsRoot {
			return nil, fmt.Errorf("detect stack: path must be within workspace root %s", wsRoot)
		}
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
// It uses EvalSymlinks to resolve symlinks and clean paths, guarding against
// symlink-based path traversal attacks.
func (s *ProjectService) isUnderWorkspaceRoot(wsPath string) bool {
	if wsPath == "" || s.workspaceRoot == "" {
		return false
	}
	// EvalSymlinks resolves symlinks AND cleans the path.
	resolvedPath, err := filepath.EvalSymlinks(wsPath)
	if err != nil {
		return false // path doesn't exist or can't be resolved — reject
	}
	resolvedRoot, err := filepath.EvalSymlinks(s.workspaceRoot)
	if err != nil {
		return false
	}
	return strings.HasPrefix(resolvedPath, resolvedRoot+string(filepath.Separator))
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
		result.RecordStepMsg("clone", "skipped", "")
	case p.RepoURL == "":
		inited, initErr := s.InitWorkspace(ctx, id, tenantID)
		if initErr != nil {
			slog.Warn("setup: init workspace failed", "project_id", id, "error", initErr)
			result.RecordStep("init-workspace", "failed", initErr)
		} else {
			p = inited
			result.RecordStep("init-workspace", "completed", nil)
		}
	default:
		cloned, cloneErr := s.Clone(ctx, id, tenantID, branch)
		if cloneErr != nil {
			slog.Warn("setup: clone failed", "project_id", id, "error", cloneErr)
			result.RecordStep("clone", "failed", cloneErr)
		} else {
			result.Cloned = true
			p = cloned
			result.RecordStep("clone", "completed", nil)
		}
	}

	// Step 2: Detect stack (requires workspace).
	if p.WorkspacePath != "" {
		stack, stackErr := project.ScanWorkspace(p.WorkspacePath)
		if stackErr != nil {
			slog.Warn("setup: stack detection failed", "project_id", id, "error", stackErr)
			result.RecordStep("detect_stack", "failed", stackErr)
		} else {
			result.StackDetected = true
			result.Stack = stack
			result.RecordStep("detect_stack", "completed", nil)

			// Persist detected languages to project config for onboarding pipeline.
			if len(stack.Languages) > 0 {
				langJSON, marshalErr := json.Marshal(stack.Languages)
				if marshalErr == nil {
					if p.Config == nil {
						p.Config = make(map[string]string)
					}
					p.Config["detected_languages"] = string(langJSON)
					if updateErr := s.store.UpdateProject(ctx, p); updateErr != nil {
						slog.Warn("setup: failed to persist detected languages",
							"project_id", id, "error", updateErr)
					}
				}
			}
		}
	} else {
		result.RecordStepMsg("detect_stack", "skipped", "no workspace available")
	}

	// Step 3: Detect and import specs (requires workspace + spec detector).
	switch {
	case p.WorkspacePath != "" && s.specDetector != nil:
		detected, importErr := s.specDetector.DetectAndImport(ctx, id)
		switch {
		case importErr != nil:
			slog.Warn("setup: spec import failed", "project_id", id, "error", importErr)
			result.RecordStep("import_specs", "failed", importErr)
		case detected:
			result.SpecsDetected = true
			result.RecordStep("import_specs", "completed", nil)
		default:
			result.RecordStepMsg("import_specs", "skipped", "no specs found")
		}
	case s.specDetector == nil:
		result.RecordStepMsg("import_specs", "skipped", "spec detector not configured")
	default:
		result.RecordStepMsg("import_specs", "skipped", "no workspace available")
	}

	// Step 4: Discover project goals (requires workspace + goal discovery service).
	switch {
	case p.WorkspacePath != "" && s.goalDiscovery != nil:
		goalResult, goalErr := s.goalDiscovery.DetectAndImport(ctx, id, p.WorkspacePath)
		switch {
		case goalErr != nil:
			slog.Warn("setup: goal discovery failed", "project_id", id, "error", goalErr)
			result.RecordStep("discover_goals", "failed", goalErr)
		case goalResult.GoalsCreated > 0:
			result.RecordStep("discover_goals", "completed", nil)
		default:
			result.RecordStepMsg("discover_goals", "skipped", "no goal files found")
		}
	case s.goalDiscovery == nil:
		result.RecordStepMsg("discover_goals", "skipped", "goal discovery not configured")
	default:
		result.RecordStepMsg("discover_goals", "skipped", "no workspace available")
	}

	return result, nil
}

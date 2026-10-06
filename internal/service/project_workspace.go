package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
	"github.com/Strob0t/CodeForge/internal/workspaceacl"
	"github.com/Strob0t/CodeForge/internal/workspacefs"
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
	if err := s.checkCloneSource(ctx, p.Provider, p.RepoURL); err != nil {
		return nil, err
	}

	gp, err := s.gitProvider(p)
	if err != nil {
		return nil, fmt.Errorf("create git provider: %w", err)
	}

	var opts []gitprovider.CloneOption
	if branch != "" {
		opts = append(opts, gitprovider.WithBranch(branch))
	}

	// The tenant directory exists with its ACLs before git could create it
	// with plain modes (KI-96).
	if err := s.ensureTenantDir(ctx, tenantID); err != nil {
		return nil, err
	}
	destPath := filepath.Join(s.workspaceRoot, tenantID, p.ID)
	// A concurrent clone of the same project would otherwise also see no
	// directory and, when it fails, remove the other clone's workspace.
	unlock, err := s.lockClone(ctx, destPath)
	if err != nil {
		return nil, fmt.Errorf("clone: %w", err)
	}
	defer unlock()
	_, statErr := os.Lstat(destPath)
	fresh := errors.Is(statErr, fs.ErrNotExist)
	if err := gp.Clone(ctx, p.RepoURL, destPath, opts...); err != nil {
		// A clone that failed or was cancelled half-way (the request ended,
		// git.operation_timeout passed) leaves a partial directory the next
		// clone would take for a repository (KI-213). Only what this clone
		// created goes; no project uses it yet.
		if fresh {
			if rmErr := os.RemoveAll(destPath); rmErr != nil {
				slog.Warn("clone: partial workspace not removed", "project_id", p.ID, "path", destPath, "error", rmErr)
			}
		}
		return nil, fmt.Errorf("clone: %w", err)
	}

	p.WorkspacePath = destPath
	if err := s.store.UpdateProject(ctx, p); err != nil {
		return nil, fmt.Errorf("update project workspace: %w", err)
	}

	return p, nil
}

// lockClone waits until no other clone into destPath runs, or ctx ends.
func (s *ProjectService) lockClone(ctx context.Context, destPath string) (unlock func(), err error) {
	s.cloneMu.Lock()
	if s.cloneSlots == nil {
		s.cloneSlots = make(map[string]chan struct{})
	}
	slot, ok := s.cloneSlots[destPath]
	if !ok {
		slot = make(chan struct{}, 1)
		s.cloneSlots[destPath] = slot
	}
	s.cloneMu.Unlock()

	select {
	case slot <- struct{}{}:
		return func() { <-slot }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SetAdoptRoots sets the operator-configured directories (workspace.adopt_roots)
// whose subdirectories admins may adopt or clone from, besides their
// tenant's workspace directory.
func (s *ProjectService) SetAdoptRoots(roots []string) {
	s.adoptRoots = nil
	for _, r := range roots {
		if resolved, err := filepath.EvalSymlinks(r); err == nil {
			s.adoptRoots = append(s.adoptRoots, resolved)
		}
	}
}

// Adopt sets an existing directory as the project's workspace without
// cloning (S3 follow-up 1f, S3-F review C4). The directory, with symlinks
// resolved, must be inside the caller's tenant directory of the workspace
// root (<root>/<tenant>/...), or - for platform admins only - inside a
// configured adopt root but outside the workspace root (localSourceAllowed).
// Another tenant's workspace, the rest of the workspace root and anything
// else are refused.
func (s *ProjectService) Adopt(ctx context.Context, id, path string, platformAdmin bool) (*project.Project, error) {
	if path == "" {
		return nil, fmt.Errorf("adopt: path is required")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("adopt: resolve path: %w", err)
	}
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return nil, fmt.Errorf("adopt: directory does not exist: %w", err)
	}
	info, err := os.Stat(realPath)
	if err != nil {
		return nil, fmt.Errorf("adopt: directory does not exist: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("adopt: %s is not a directory", absPath)
	}
	if !s.localSourceAllowed(ctx, realPath, platformAdmin) {
		return nil, fmt.Errorf("adopt: %s must be inside this tenant's workspace directory %s (platform admins: or inside a "+
			"workspace.adopt_roots directory outside the workspace root): %w", absPath, s.tenantArea(ctx), domain.ErrValidation)
	}

	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if err := s.adoptToolACLs(ctx, realPath); err != nil {
		return nil, err
	}

	p.WorkspacePath = realPath
	if err := s.store.UpdateProject(ctx, p); err != nil {
		return nil, fmt.Errorf("update project workspace: %w", err)
	}

	return p, nil
}

// gitProvider is the project's git provider (tests replace the resolver).
func (s *ProjectService) gitProvider(p *project.Project) (gitprovider.Provider, error) {
	if s.resolveProvider != nil {
		return s.resolveProvider(p)
	}
	return resolveGitProvider(p)
}

// WorkspaceRoot is the absolute workspace root (<root>/<tenant>/<project>).
func (s *ProjectService) WorkspaceRoot() string {
	return s.workspaceRoot
}

// ensureTenantDir gives the tenant directory its ACLs for the tenant's tool
// UID (allocated now if it has none) when workspace.tool_acls is required
// (KI-96); with off nothing happens and MkdirAll or git create it.
func (s *ProjectService) ensureTenantDir(ctx context.Context, tenantID string) error {
	if !s.toolUIDs.Required() {
		return nil
	}
	uid, err := s.toolUIDs.ToolUIDFor(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("tool uid: %w", err)
	}
	if err := workspaceacl.EnsureTenantDir(s.workspaceRoot, tenantID, uid); err != nil {
		return fmt.Errorf("tenant directory of tenant %s (tool uid %d): %w", tenantID, uid, err)
	}
	return nil
}

// adoptToolACLs prepares an adopted workspace for the tenant's tool UID
// (KI-96): inside the tenant area its tenant directory gets its ACLs; an
// adopted directory outside the workspace root gets the ACLs of a project
// when the Go Core owns it, and is refused otherwise, with the command an
// operator runs. Its content is migrated by the worker.
func (s *ProjectService) adoptToolACLs(ctx context.Context, realPath string) error {
	if !s.toolUIDs.Required() {
		return nil
	}
	tenantID := tenantctx.FromContext(ctx)
	if s.underWorkspaceRoot(realPath) {
		return s.ensureTenantDir(ctx, tenantID)
	}
	uid, err := s.toolUIDs.ToolUIDFor(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("tool uid: %w", err)
	}
	if err := workspaceacl.SetProjectACLs(realPath, uid); err != nil {
		return fmt.Errorf("adopt: %s needs ACLs for the tenant's tool uid %d (%s); as an operator run: %s: %w",
			realPath, uid, err.Error(), AdoptedWorkspaceCommand(realPath, uid), domain.ErrValidation)
	}
	return nil
}

// AdoptedWorkspaceCommand is the setfacl command that opens an adopted
// workspace to a tenant's tool UID and the workspace group (KI-96).
func AdoptedWorkspaceCommand(dir string, toolUID int) string {
	return fmt.Sprintf("setfacl -R -m u:%d:rwX -m d:u:%d:rwX -m g:%d:rwX -m d:g:%d:rwX %s",
		toolUID, toolUID, workspaceacl.WorkspaceGID, workspaceacl.WorkspaceGID, dir)
}

// tenantArea is the caller's tenant directory of the workspace root.
func (s *ProjectService) tenantArea(ctx context.Context) string {
	root := s.workspaceRoot
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return filepath.Join(root, tenantctx.FromContext(ctx))
}

// inTenantArea reports whether the resolved path lies strictly inside the
// caller's tenant directory.
func (s *ProjectService) inTenantArea(ctx context.Context, realPath string) bool {
	return s.workspaceRoot != "" && strictlyInside(realPath, s.tenantArea(ctx))
}

// localSourceAllowed reports whether a resolved local path may be adopted
// or cloned from: under the workspace root only in the caller's tenant
// area, whatever the adopt roots say (an adopt root may contain the
// workspace root); elsewhere only inside an adopt root, and - for
// adoption, where the caller chooses the path - only by a platform admin.
func (s *ProjectService) localSourceAllowed(ctx context.Context, realPath string, platformAdmin bool) bool {
	if s.underWorkspaceRoot(realPath) {
		return s.inTenantArea(ctx, realPath)
	}
	return platformAdmin && s.inAdoptRoot(realPath)
}

// underWorkspaceRoot reports whether the resolved path is the workspace
// root or lies inside it.
func (s *ProjectService) underWorkspaceRoot(realPath string) bool {
	if s.workspaceRoot == "" {
		return false
	}
	root := s.workspaceRoot
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return realPath == root || strictlyInside(realPath, root)
}

// inAdoptRoot reports whether the resolved path lies strictly inside a
// configured adopt root.
func (s *ProjectService) inAdoptRoot(realPath string) bool {
	for _, r := range s.adoptRoots {
		if strictlyInside(realPath, r) {
			return true
		}
	}
	return false
}

func strictlyInside(path, dir string) bool {
	return strings.HasPrefix(path, dir+string(filepath.Separator))
}

// checkCloneSource allows remote repository URLs of the provider
// (project.ValidateRepoURL) and local ones (a path or file://) only where
// localSourceAllowed allows them:
// repo_url validation keeps local paths out of new projects, and a stored
// one must not read another tenant's workspace (S3 follow-up 1f, S3-F
// review C4). The URL is the project's, not chosen at clone time, so the
// adopt roots apply whoever clones.
func (s *ProjectService) checkCloneSource(ctx context.Context, provider, url string) error {
	if project.ValidateRepoURL(provider, url) == nil {
		return nil
	}
	local := strings.TrimPrefix(url, "file://")
	if filepath.IsAbs(local) {
		if resolved, err := filepath.EvalSymlinks(local); err == nil && s.localSourceAllowed(ctx, resolved, true) {
			return nil
		}
	}
	return fmt.Errorf("repository %q: a local repository must be inside this tenant's workspace directory or a "+
		"workspace.adopt_roots directory outside the workspace root: %w", url, domain.ErrValidation)
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

	if err := s.ensureTenantDir(ctx, tenantID); err != nil {
		return nil, err
	}
	destPath := filepath.Join(s.workspaceRoot, tenantID, p.ID)
	if err := os.MkdirAll(destPath, project.WorkspaceDirPerm); err != nil { //nolint:gosec // G301: shared with the worker's tool user (KI-71)
		return nil, fmt.Errorf("create workspace directory: %w", err)
	}

	// Initialize a git repository so agents can work with version control.
	if _, gitErr := git.Run(ctx, "", "init", destPath); gitErr != nil {
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

	// Read through workspacefs (KI-95): the walk stays inside the workspace.
	ws, err := workspacefs.Open(p.WorkspacePath)
	if err != nil {
		return info, nil
	}
	defer func() { _ = ws.Close() }()
	stat, err := ws.Stat(".")
	if err != nil {
		return info, nil
	}
	info.Exists = true
	info.LastModified = stat.ModTime()

	// Check for the git directory.
	if gitStat, gitErr := ws.Stat(".git"); gitErr == nil && gitStat.IsDir() {
		info.GitRepo = true
	}

	// Compute disk usage.
	var totalSize int64
	_ = ws.WalkDir(".", func(_ string, d fs.DirEntry, walkErr error) error {
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

// DetectStackByPath scans a directory for language detection. The path
// follows Adopt's rule (KI-106): under the workspace root only inside the
// caller's tenant area, elsewhere only inside an adopt root and only for
// platform admins. It is resolved inside that area through workspacefs, so
// no symlink on the way leads out of it.
func (s *ProjectService) DetectStackByPath(ctx context.Context, path string, platformAdmin bool) (*project.StackDetectionResult, error) {
	if path == "" {
		return nil, fmt.Errorf("detect stack: path is required: %w", domain.ErrValidation)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("detect stack: resolve path: %w", err)
	}
	base := s.detectStackBase(ctx, absPath, platformAdmin)
	if base == "" {
		return nil, fmt.Errorf("detect stack: the path must be inside this tenant's workspace directory (platform "+
			"admins: or inside a workspace.adopt_roots directory outside the workspace root): %w", domain.ErrValidation)
	}
	ws, err := workspacefs.OpenBelow(base, absPath)
	if err != nil {
		if errors.Is(err, workspacefs.ErrLeavesWorkspace) {
			return nil, fmt.Errorf("detect stack: %w: %w", domain.ErrValidation, err)
		}
		return nil, fmt.Errorf("detect stack: directory does not exist: %w", err)
	}
	defer func() { _ = ws.Close() }()
	return project.ScanWorkspaceFS(ws.FS(), absPath)
}

// detectStackBase is the directory DetectStackByPath resolves absPath in:
// the caller's tenant area when absPath is under the workspace root (as
// given or resolved), an adopt root for platform admins outside it, else "".
func (s *ProjectService) detectStackBase(ctx context.Context, absPath string, platformAdmin bool) string {
	if s.workspaceRoot != "" {
		roots := []string{s.workspaceRoot}
		if resolved, err := filepath.EvalSymlinks(s.workspaceRoot); err == nil {
			roots = append(roots, resolved)
		}
		for _, root := range roots {
			absRoot, err := filepath.Abs(root)
			if err != nil {
				continue
			}
			if absPath == absRoot || strictlyInside(absPath, absRoot) {
				area := filepath.Join(absRoot, tenantctx.FromContext(ctx))
				if strictlyInside(absPath, area) {
					return area
				}
				return ""
			}
		}
	}
	if platformAdmin {
		for _, r := range s.adoptRoots {
			if strictlyInside(absPath, r) {
				return r
			}
		}
	}
	return ""
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

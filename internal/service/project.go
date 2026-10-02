// Package service implements business logic on top of ports.
package service

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// SpecDetector is an optional interface for detecting and importing roadmap specs
// during automated project setup. Implemented by RoadmapService via specDetectorAdapter.
type SpecDetector interface {
	DetectAndImport(ctx context.Context, projectID string) (detected bool, importErr error)
}

// RepoMapIndexer generates repository maps for a project.
type RepoMapIndexer interface {
	RequestGeneration(ctx context.Context, projectID string, activeFiles []string) error
}

// RetrievalIndexer builds retrieval indexes for a project workspace.
type RetrievalIndexer interface {
	RequestIndex(ctx context.Context, projectID, workspacePath, embeddingModel string) error
}

// GraphBuilder builds code graphs for a project workspace.
type GraphBuilder interface {
	RequestBuild(ctx context.Context, projectID, workspacePath string) error
}

// ReviewTriggerer starts the boundary analysis of an indexed project
// (ReviewTriggerService, contract-first review).
type ReviewTriggerer interface {
	TriggerBoundaryAnalysis(ctx context.Context, projectID string) (*plan.ExecutionPlan, error)
}

// projectStore defines the database operations needed by ProjectService.
// Consumer-defined interface following ISP (ADR-014).
type projectStore interface {
	database.ProjectStore
}

// ProjectService handles project business logic.
type ProjectService struct {
	toolUIDSource
	store           projectStore
	workspaceRoot   string
	adoptRoots      []string // resolved workspace.adopt_roots (admins adopt/clone from there)
	specDetector    SpecDetector
	goalDiscovery   *GoalDiscoveryService
	repoMap         RepoMapIndexer
	retrieval       RetrievalIndexer
	graph           GraphBuilder
	reviewTriggerer ReviewTriggerer
	// resolveProvider replaces resolveGitProvider in tests (nil: the registry).
	resolveProvider func(*project.Project) (gitprovider.Provider, error)
	// deletions removes deleted projects' workspaces through the worker (KI-96 D11).
	deletions workspaceDeleter
}

// workspaceDeleter deletes a project whose workspace the worker removes as
// the tenant's tool UID (WorkspaceDeletionService).
type workspaceDeleter interface {
	Delete(ctx context.Context, p *project.Project) error
}

// SetWorkspaceDeletions sets who deletes projects with tool ACLs required (KI-96 D11).
func (s *ProjectService) SetWorkspaceDeletions(d workspaceDeleter) {
	s.deletions = d
}

// NewProjectService creates a new ProjectService.
// workspaceRoot is resolved to an absolute path so that all downstream paths
// (NATS payloads, tool execution) are independent of the working directory.
func NewProjectService(store projectStore, workspaceRoot string) *ProjectService {
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		absRoot = workspaceRoot
	}
	return &ProjectService{store: store, workspaceRoot: absRoot}
}

// SetSpecDetector sets the optional spec detector for automated setup.
func (s *ProjectService) SetSpecDetector(sd SpecDetector) {
	s.specDetector = sd
}

// SetGoalDiscovery sets the optional goal discovery service for automated setup.
func (s *ProjectService) SetGoalDiscovery(svc *GoalDiscoveryService) {
	s.goalDiscovery = svc
}

// SetRepoMapIndexer sets the optional repo-map indexer for auto-indexing.
func (s *ProjectService) SetRepoMapIndexer(r RepoMapIndexer) {
	s.repoMap = r
}

// SetRetrievalIndexer sets the optional retrieval indexer for auto-indexing.
func (s *ProjectService) SetRetrievalIndexer(r RetrievalIndexer) {
	s.retrieval = r
}

// SetGraphBuilder sets the optional graph builder for auto-indexing.
func (s *ProjectService) SetGraphBuilder(g GraphBuilder) {
	s.graph = g
}

// SetReviewTriggerer sets the optional review triggerer for auto-indexing.
func (s *ProjectService) SetReviewTriggerer(rt ReviewTriggerer) {
	s.reviewTriggerer = rt
}

// AutoIndex triggers background indexing for all context sources.
// Called after clone, adopt, or setup to ensure agents get full context.
// Each index build is independent -- failures are logged but do not block.
func (s *ProjectService) AutoIndex(tenantID, projectID, workspacePath string) {
	if s.repoMap != nil {
		go func() {
			ctx := tenantctx.WithTenant(context.Background(), tenantID)
			if err := s.repoMap.RequestGeneration(ctx, projectID, nil); err != nil {
				slog.Error("auto repomap generation failed", "project_id", projectID, "error", err)
			}
		}()
	}

	if s.retrieval != nil {
		go func() {
			ctx := tenantctx.WithTenant(context.Background(), tenantID)
			if err := s.retrieval.RequestIndex(ctx, projectID, workspacePath, ""); err != nil {
				slog.Error("auto retrieval index failed", "project_id", projectID, "error", err)
			}
		}()
	}

	if s.graph != nil {
		go func() {
			ctx := tenantctx.WithTenant(context.Background(), tenantID)
			if err := s.graph.RequestBuild(ctx, projectID, workspacePath); err != nil {
				slog.Error("auto graph build failed", "project_id", projectID, "error", err)
			}
		}()
	}

	if s.reviewTriggerer != nil {
		go func() {
			ctx := tenantctx.WithTenant(context.Background(), tenantID)
			_, err := s.reviewTriggerer.TriggerBoundaryAnalysis(ctx, projectID)
			switch {
			case err == nil:
			case errors.Is(err, domain.ErrValidation):
				// E.g. no idle agent yet; the analysis can be started from the
				// boundaries panel later.
				slog.Info("auto boundary analysis not started", "project_id", projectID, "reason", err)
			default:
				slog.Error("auto boundary analysis failed", "project_id", projectID, "error", err)
			}
		}()
	}
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

// Update applies partial updates to a project. Config is merged key by key
// (see project.ConfigPatch), so a client that changes one setting cannot wipe
// the others, such as policy_preset or detected_languages.
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
	if len(req.Config) > 0 {
		p.Config = req.Config.Apply(p.Config)
	}

	if err := s.store.UpdateProject(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// SetPolicyProfile assigns a policy profile name to a project.
func (s *ProjectService) SetPolicyProfile(ctx context.Context, projectID, profileName string) error {
	p, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	p.PolicyProfile = profileName
	return s.store.UpdateProject(ctx, p)
}

// Delete removes a project and cleans up its workspace directory.
//
// With tool ACLs required (KI-96 D11) a workspace under the root is removed
// by the worker as the tenant's tool UID: tools can create entries nobody
// else can remove. The project row goes in one transaction with the
// deletion record, and not while the project has active work
// (project.ErrProjectBusy). With tool ACLs off (development) the Go Core
// removes the workspace itself.
func (s *ProjectService) Delete(ctx context.Context, id string) error {
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return s.store.DeleteProject(ctx, id)
	}

	wsPath := p.WorkspacePath
	if s.deletions != nil && s.toolUIDs.Required() && wsPath != "" && s.isUnderWorkspaceRoot(wsPath) {
		return s.deletions.Delete(ctx, p)
	}

	if err := s.store.DeleteProject(ctx, id); err != nil {
		return err
	}

	if wsPath != "" && s.isUnderWorkspaceRoot(wsPath) {
		if rmErr := os.RemoveAll(wsPath); rmErr != nil {
			slog.Error("failed to remove workspace directory",
				"project_id", id,
				"path", wsPath,
				"error", rmErr,
			)
		}
	}

	return nil
}

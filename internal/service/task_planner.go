package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// TaskPlannerService enhances feature decomposition with project context,
// complexity heuristics, and optional automatic team assembly.
type TaskPlannerService struct {
	meta    *MetaAgentService
	pool    *PoolManagerService
	store   database.Store
	orchCfg *config.Orchestrator
}

// NewTaskPlannerService creates a TaskPlannerService with all dependencies.
func NewTaskPlannerService(
	meta *MetaAgentService,
	pool *PoolManagerService,
	store database.Store,
	orchCfg *config.Orchestrator,
) *TaskPlannerService {
	return &TaskPlannerService{
		meta:    meta,
		pool:    pool,
		store:   store,
		orchCfg: orchCfg,
	}
}

// PlanFeature is the enhanced decomposition pipeline: it gathers project context,
// enriches the LLM prompt, delegates to MetaAgentService.DecomposeFeature,
// and optionally assembles a team.
func (s *TaskPlannerService) PlanFeature(ctx context.Context, req *plan.PlanFeatureRequest) (*plan.ExecutionPlan, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate plan feature request: %w", err)
	}

	// Gather project workspace context for richer LLM prompting.
	fileContext, err := s.gatherProjectContext(ctx, req.ProjectID)
	if err != nil {
		slog.Warn("could not gather project context", "project_id", req.ProjectID, "error", err)
		// Non-fatal: proceed with whatever context we have.
	}

	// Enrich context with file listing.
	enrichedContext := req.Context
	if fileContext != "" {
		if enrichedContext != "" {
			enrichedContext += "\n\n"
		}
		enrichedContext += "Project file structure:\n" + fileContext
	}

	// Delegate to MetaAgentService for actual LLM decomposition.
	decompReq := &plan.DecomposeRequest{
		ProjectID: req.ProjectID,
		Feature:   req.Feature,
		Context:   enrichedContext,
		Model:     req.Model,
		AutoStart: req.AutoStart,
	}

	p, err := s.meta.DecomposeFeature(ctx, decompReq)
	if err != nil {
		return nil, fmt.Errorf("decompose feature: %w", err)
	}

	// Auto-assemble team if requested.
	if req.AutoTeam && s.pool != nil {
		strategy := s.estimateComplexity(len(p.Steps))
		teamName := fmt.Sprintf("team-%s", p.Name)
		team, err := s.pool.AssembleTeamForStrategy(ctx, req.ProjectID, strategy, teamName)
		if err != nil {
			slog.Warn("auto-team assembly failed", "plan_id", p.ID, "error", err)
			// Non-fatal: plan still usable without a team.
		} else {
			slog.Info("team auto-assembled for plan",
				"plan_id", p.ID,
				"team_id", team.ID,
				"strategy", strategy,
				"members", len(team.Members),
			)
		}
	}

	return p, nil
}

// gatherProjectContext returns a text summary of the project workspace file structure.
func (s *TaskPlannerService) gatherProjectContext(ctx context.Context, projectID string) (string, error) {
	proj, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("get project: %w", err)
	}

	if proj.WorkspacePath == "" {
		return "", nil
	}

	entries, err := os.ReadDir(proj.WorkspacePath)
	if err != nil {
		return "", fmt.Errorf("read workspace dir: %w", err)
	}

	var b strings.Builder
	const maxEntries = 100
	count := 0

	for _, e := range entries {
		if count >= maxEntries {
			fmt.Fprintf(&b, "... (%d more entries)\n", len(entries)-count)
			break
		}

		name := e.Name()
		// Skip hidden files/dirs.
		if strings.HasPrefix(name, ".") {
			continue
		}

		if e.IsDir() {
			fmt.Fprintf(&b, "%s/\n", name)
			// List first-level contents of this subdirectory.
			subEntries, err := os.ReadDir(filepath.Join(proj.WorkspacePath, name))
			if err != nil {
				continue
			}
			for _, se := range subEntries {
				if count >= maxEntries {
					break
				}
				if strings.HasPrefix(se.Name(), ".") {
					continue
				}
				suffix := ""
				if se.IsDir() {
					suffix = "/"
				}
				fmt.Fprintf(&b, "  %s%s\n", se.Name(), suffix)
				count++
			}
		} else {
			fmt.Fprintf(&b, "%s\n", name)
		}
		count++
	}

	return b.String(), nil
}

// GatherProjectContextForTest exposes gatherProjectContext for testing.
func (s *TaskPlannerService) GatherProjectContextForTest(ctx context.Context, projectID string) (string, error) {
	return s.gatherProjectContext(ctx, projectID)
}

// EstimateComplexityForTest exposes estimateComplexity for testing.
func (s *TaskPlannerService) EstimateComplexityForTest(stepCount int) plan.AgentStrategy {
	return s.estimateComplexity(stepCount)
}

// estimateComplexity uses a simple heuristic based on subtask count to suggest a strategy.
func (s *TaskPlannerService) estimateComplexity(stepCount int) plan.AgentStrategy {
	switch {
	case stepCount <= 1:
		return plan.StrategySingle
	case stepCount == 2:
		return plan.StrategyPair
	default:
		return plan.StrategyTeam
	}
}

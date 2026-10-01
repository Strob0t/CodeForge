package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/boundary"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/pipeline"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/review"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
)

// Pipeline templates the review service starts (internal/domain/pipeline).
const (
	reviewRefactorTemplate   = "review-refactor"
	boundaryAnalysisTemplate = "boundary-analysis"
)

// Modes whose completed steps the review gate handles.
const (
	boundaryAnalyzerMode = "boundary_analyzer"
	refactorerMode       = "refactorer"
)

// ErrReviewNoAgents: the project has no agent to run the review pipeline.
var ErrReviewNoAgents = fmt.Errorf("%w: project has no agents: add an agent to run the review pipeline", domain.ErrValidation)

// reviewStepPrompts is the task prompt of each review pipeline step, by mode.
// Later steps read the earlier steps' output from the team's shared context.
var reviewStepPrompts = map[string]string{
	boundaryAnalyzerMode: "Identify the boundary files of this project: files that define contracts between " +
		"modules, services or languages (API schemas, data models and migrations, message and event " +
		"definitions, shared cross-language types). Answer with the BOUNDARIES.json array described in your " +
		"instructions and nothing else.",
	"contract_reviewer": "Review the contracts at the project's boundary files (see the boundary analysis in the " +
		"shared context): check that producers and consumers across layers and languages agree on names, " +
		"types, required fields and error cases. Report every inconsistency with file references as CONTRACT_REVIEW.md.",
	"reviewer": "Review the code quality within each layer of the project, taking the contract review in the " +
		"shared context into account. Report the findings with file references as REVIEW.md.",
	refactorerMode: "Apply the refactorings the contract review and the code review in the shared context call " +
		"for. Keep the behavior unchanged and the change as small as possible, and do not commit. Summarize " +
		"what you changed.",
}

// reviewPipelineStore is the part of the store the review pipeline uses.
type reviewPipelineStore interface {
	GetProject(ctx context.Context, id string) (*project.Project, error)
	ListAgents(ctx context.Context, projectID string) ([]agent.Agent, error)
	CreateTask(ctx context.Context, req task.CreateRequest) (*task.Task, error)
	GetRun(ctx context.Context, id string) (*run.Run, error)
	GetPlan(ctx context.Context, id string) (*plan.ExecutionPlan, error)
	GetProjectBoundaries(ctx context.Context, projectID string) (*boundary.ProjectBoundaryConfig, error)
	UpsertProjectBoundaries(ctx context.Context, cfg *boundary.ProjectBoundaryConfig) error
	CreateReviewPipeline(ctx context.Context, rp *review.Pipeline) error
	GetReviewPipeline(ctx context.Context, planID string) (*review.Pipeline, error)
}

// reviewPlanner creates, starts and decides the review plans.
type reviewPlanner interface {
	CreatePlan(ctx context.Context, req *plan.CreatePlanRequest) (*plan.ExecutionPlan, error)
	StartPlan(ctx context.Context, planID string) (*plan.ExecutionPlan, error)
	CancelPlan(ctx context.Context, planID string) error
	ApproveStep(ctx context.Context, planID, stepID string) error
	RejectStep(ctx context.Context, planID, stepID string) error
}

// reviewTeams creates the team whose shared context carries the review
// reports from step to step.
type reviewTeams interface {
	CreateTeam(ctx context.Context, req *agent.CreateTeamRequest) (*agent.Team, error)
}

// ReviewPipelineService runs the contract-first review pipelines (Phase 31):
// it starts the review-refactor pipeline and the boundary analysis as
// execution plans, stores the boundaries the analysis finds, and applies the
// threshold HITL to the refactoring step (GateStep, Decide).
type ReviewPipelineService struct {
	store     reviewPipelineStore
	pipelines *PipelineService
	plans     reviewPlanner
	teams     reviewTeams
	git       *git.Pool
	hub       broadcast.Broadcaster
	scorer    *DiffImpactScorer

	// decideMu serializes decisions, so an approval and a rejection of the
	// same step cannot both pass the waiting check (a rejection undoes the
	// workspace before the step fails).
	decideMu sync.Mutex
}

// NewReviewPipelineService creates a ReviewPipelineService.
func NewReviewPipelineService(
	store reviewPipelineStore,
	pipelines *PipelineService,
	plans reviewPlanner,
	teams reviewTeams,
	gitPool *git.Pool,
	hub broadcast.Broadcaster,
	impact DiffImpactConfig,
) *ReviewPipelineService {
	return &ReviewPipelineService{
		store: store, pipelines: pipelines, plans: plans, teams: teams,
		git: gitPool, hub: hub, scorer: NewDiffImpactScorer(impact),
	}
}

// StartReviewPipeline starts the review-refactor pipeline for the project and
// returns its plan.
func (s *ReviewPipelineService) StartReviewPipeline(ctx context.Context, projectID string) (*plan.ExecutionPlan, error) {
	return s.start(ctx, projectID, reviewRefactorTemplate)
}

// StartBoundaryAnalysis starts the boundary analysis for the project and
// returns its plan.
func (s *ReviewPipelineService) StartBoundaryAnalysis(ctx context.Context, projectID string) (*plan.ExecutionPlan, error) {
	return s.start(ctx, projectID, boundaryAnalysisTemplate)
}

// start instantiates the template with one task per step and an idle agent of
// the project, creates the plan and starts it. A pipeline that refactors gets
// a team (the reports reach later steps through its shared context) and a
// baseline of the workspace to measure and undo the refactoring. An error
// means no step is running.
func (s *ReviewPipelineService) start(ctx context.Context, projectID, templateID string) (*plan.ExecutionPlan, error) {
	proj, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if proj.WorkspacePath == "" {
		return nil, fmt.Errorf("%w: project has no workspace", domain.ErrValidation)
	}
	ag, err := s.pickAgent(ctx, projectID)
	if err != nil {
		return nil, err
	}
	tmpl, err := s.pipelines.Get(templateID)
	if err != nil {
		return nil, err
	}

	refactors := false
	bindings := make([]pipeline.StepBinding, len(tmpl.Steps))
	for i, st := range tmpl.Steps {
		refactors = refactors || st.ModeID == refactorerMode
		t, err := s.store.CreateTask(ctx, task.CreateRequest{
			ProjectID: projectID,
			Title:     tmpl.Name + ": " + st.Name,
			Prompt:    reviewStepPrompts[st.ModeID],
		})
		if err != nil {
			return nil, fmt.Errorf("create task for step %q: %w", st.Name, err)
		}
		bindings[i] = pipeline.StepBinding{TaskID: t.ID, AgentID: ag.ID}
	}

	var teamID string
	if len(tmpl.Steps) > 1 {
		team, err := s.teams.CreateTeam(ctx, &agent.CreateTeamRequest{
			ProjectID: projectID,
			Name:      tmpl.Name,
			Protocol:  string(tmpl.Protocol),
			Members:   []agent.CreateMemberRequest{{AgentID: ag.ID, Role: agent.RoleCoder}},
		})
		if err != nil {
			return nil, fmt.Errorf("create review team: %w", err)
		}
		teamID = team.ID
	}

	req, err := s.pipelines.Instantiate(ctx, templateID, pipeline.InstantiateRequest{
		ProjectID: projectID,
		TeamID:    teamID,
		PlanName:  fmt.Sprintf("%s %s", tmpl.Name, time.Now().UTC().Format("2006-01-02 15:04")),
		Bindings:  bindings,
	})
	if err != nil {
		return nil, err
	}
	p, err := s.plans.CreatePlan(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create review plan: %w", err)
	}

	rp := &review.Pipeline{PlanID: p.ID, ProjectID: projectID}
	if refactors {
		err := s.git.Run(ctx, func() error {
			repo, err := git.OpenRepo(ctx, proj.WorkspacePath)
			if err != nil {
				return err
			}
			rp.BaselineSHA, err = snapshotWorkspace(ctx, repo, p.ID)
			return err
		})
		if err != nil {
			logBestEffort(ctx, s.plans.CancelPlan(ctx, p.ID), "CancelPlan", slog.String("plan_id", p.ID))
			return nil, fmt.Errorf("record the workspace baseline: %w", err)
		}
	}
	if err := s.store.CreateReviewPipeline(ctx, rp); err != nil {
		logBestEffort(ctx, s.plans.CancelPlan(ctx, p.ID), "CancelPlan", slog.String("plan_id", p.ID))
		return nil, fmt.Errorf("record the review pipeline: %w", err)
	}

	started, err := s.plans.StartPlan(ctx, p.ID)
	if err != nil {
		return nil, fmt.Errorf("start review plan: %w", err)
	}
	if started.Status == plan.StatusFailed {
		return nil, fmt.Errorf("%w: review plan %s failed to start its first step: %s", domain.ErrValidation, started.ID, firstStepError(started))
	}
	slog.Info("review pipeline started", "plan_id", started.ID, "template", templateID, "project_id", projectID)
	return started, nil
}

// pickAgent returns an idle agent of the project.
func (s *ReviewPipelineService) pickAgent(ctx context.Context, projectID string) (*agent.Agent, error) {
	agents, err := s.store.ListAgents(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	if len(agents) == 0 {
		return nil, ErrReviewNoAgents
	}
	for i := range agents {
		if agents[i].Status == agent.StatusIdle {
			return &agents[i], nil
		}
	}
	return nil, fmt.Errorf("%w: every agent of the project is busy, try again when one is idle", domain.ErrValidation)
}

func firstStepError(p *plan.ExecutionPlan) string {
	for i := range p.Steps {
		if p.Steps[i].Error != "" {
			return p.Steps[i].Error
		}
	}
	return "unknown error"
}

// GateStep is the orchestrator's step gate (OrchestratorService.SetStepGate):
// it runs, under the plan scheduling lock, when a step's run completed. The
// boundary analyzer's result is stored as the project's boundaries; a review
// pipeline's refactoring is scored and, if its impact is high (or cannot be
// measured), the step waits for approval.
func (s *ReviewPipelineService) GateStep(ctx context.Context, step *plan.Step) plan.StepStatus {
	switch step.ModeID {
	case boundaryAnalyzerMode:
		s.storeBoundaries(ctx, step)
	case refactorerMode:
		return s.gateRefactoring(ctx, step)
	}
	return plan.StepStatusCompleted
}

// storeBoundaries replaces the auto-detected boundaries of the step's project
// with the BOUNDARIES.json array of the step's run output; manually added
// boundaries are kept. An output without a valid entry changes nothing.
func (s *ReviewPipelineService) storeBoundaries(ctx context.Context, step *plan.Step) {
	r, err := s.store.GetRun(ctx, step.RunID)
	if err != nil {
		slog.Warn("boundary analysis: run not found", "run_id", step.RunID, "error", err)
		return
	}
	detected := parseBoundaries(r.Output)
	if len(detected) == 0 {
		slog.Warn("boundary analysis: no valid BOUNDARIES.json in the output", "run_id", r.ID)
		return
	}
	cfg := &boundary.ProjectBoundaryConfig{ProjectID: r.ProjectID}
	existing, err := s.store.GetProjectBoundaries(ctx, r.ProjectID)
	switch {
	case err == nil:
		for _, b := range existing.Boundaries {
			if !b.AutoDetected {
				cfg.Boundaries = append(cfg.Boundaries, b)
			}
		}
	case !errors.Is(err, domain.ErrNotFound):
		slog.Warn("boundary analysis: load boundaries", "project_id", r.ProjectID, "error", err)
		return
	}
	cfg.Boundaries = append(cfg.Boundaries, detected...)
	cfg.LastAnalyzed = time.Now().UTC()
	if err := s.store.UpsertProjectBoundaries(ctx, cfg); err != nil {
		slog.Warn("boundary analysis: store boundaries", "project_id", r.ProjectID, "error", err)
		return
	}
	slog.Info("boundaries stored", "project_id", r.ProjectID, "detected", len(detected))
}

// parseBoundaries reads the BOUNDARIES.json array from a run output: the
// first JSON array in it, optionally in a code fence. Invalid entries are
// dropped; every entry is marked auto-detected.
func parseBoundaries(output string) []boundary.BoundaryFile {
	start, end := strings.Index(output, "["), strings.LastIndex(output, "]")
	if start < 0 || end <= start {
		return nil
	}
	var entries []boundary.BoundaryFile
	if err := json.Unmarshal([]byte(output[start:end+1]), &entries); err != nil {
		return nil
	}
	valid := entries[:0]
	for _, e := range entries {
		if e.Validate() != nil {
			continue
		}
		e.AutoDetected = true
		valid = append(valid, e)
	}
	return valid
}

// gateRefactoring scores the change a review pipeline's refactoring step made
// since the recorded baseline: low impact completes the step, medium
// completes it and notifies (review.refactor_applied), high makes it wait for
// approval (review.approval_required). A refactorer step of a plan the review
// pipeline did not start is not gated.
//
// Only the baseline recorded in the Go DB counts (V1): the workspace and its
// baseline ref are agent-writable. A missing record lookup, a missing or
// moved ref, a missing repository or a baseline commit that is gone means the
// change cannot be measured, and the step waits for approval (fail closed).
func (s *ReviewPipelineService) gateRefactoring(ctx context.Context, step *plan.Step) plan.StepStatus {
	ev := event.ReviewImpactEvent{RunID: step.RunID, PlanID: step.PlanID, StepID: step.ID, ImpactLevel: string(ImpactHigh)}
	rp, err := s.store.GetReviewPipeline(ctx, step.PlanID)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return plan.StepStatusCompleted
	case err != nil:
		return s.requireApproval(ctx, &ev, fmt.Sprintf("the review record could not be loaded: %v", err))
	}
	ev.ProjectID = rp.ProjectID
	proj, err := s.store.GetProject(ctx, rp.ProjectID)
	if err != nil {
		return s.requireApproval(ctx, &ev, fmt.Sprintf("the project could not be loaded: %v", err))
	}

	var change *workspaceChange
	err = s.git.Run(ctx, func() error {
		repo, err := git.OpenRepo(ctx, proj.WorkspacePath)
		if err != nil {
			return err
		}
		if err := checkBaseline(ctx, repo, rp.PlanID, rp.BaselineSHA); err != nil {
			return err
		}
		change, err = changeSince(ctx, repo, rp.BaselineSHA)
		return err
	})
	if err != nil {
		return s.requireApproval(ctx, &ev, fmt.Sprintf("the change could not be measured: %v", err))
	}

	stats := change.Stats
	stats.CrossLayer = s.touchesBoundary(ctx, rp.ProjectID, change.Paths)
	level := s.scorer.Score(stats)
	ev.ImpactLevel = string(level)
	ev.FilesChanged, ev.LinesAdded, ev.LinesRemoved = stats.FilesChanged, stats.LinesAdded, stats.LinesRemoved
	ev.CrossLayer, ev.Structural = stats.CrossLayer, stats.Structural
	slog.Info("review refactoring scored", "plan_id", rp.PlanID, "step_id", step.ID, "impact", level,
		"files", stats.FilesChanged, "lines_added", stats.LinesAdded, "lines_removed", stats.LinesRemoved)

	switch level {
	case ImpactLow:
		s.dropBaseline(ctx, proj.WorkspacePath, rp.PlanID)
		return plan.StepStatusCompleted
	case ImpactMedium:
		s.dropBaseline(ctx, proj.WorkspacePath, rp.PlanID)
		s.hub.BroadcastEvent(ctx, event.EventReviewRefactorApplied, ev)
		return plan.StepStatusCompleted
	default:
		return s.requireApproval(ctx, &ev, "")
	}
}

func (s *ReviewPipelineService) requireApproval(ctx context.Context, ev *event.ReviewImpactEvent, reason string) plan.StepStatus {
	ev.Reason = reason
	if reason != "" {
		slog.Warn("review refactoring needs approval", "plan_id", ev.PlanID, "step_id", ev.StepID, "reason", reason)
	}
	s.hub.BroadcastEvent(ctx, event.EventReviewApprovalRequired, *ev)
	return plan.StepStatusWaitingApproval
}

// touchesBoundary reports whether a changed path is one of the project's
// boundary files or their counterparts: the change crosses a layer contract.
func (s *ReviewPipelineService) touchesBoundary(ctx context.Context, projectID string, paths []string) bool {
	cfg, err := s.store.GetProjectBoundaries(ctx, projectID)
	if err != nil {
		return false
	}
	for _, b := range cfg.Boundaries {
		for _, p := range paths {
			if p == b.Path || (b.Counterpart != "" && p == b.Counterpart) {
				return true
			}
		}
	}
	return false
}

func (s *ReviewPipelineService) dropBaseline(ctx context.Context, dir, planID string) {
	err := s.git.Run(ctx, func() error {
		repo, err := git.OpenRepo(ctx, dir)
		if errors.Is(err, git.ErrNotRepository) {
			return nil
		}
		if err != nil {
			return err
		}
		return deleteBaseline(ctx, repo, planID)
	})
	logBestEffort(ctx, err, "delete review baseline", slog.String("plan_id", planID))
}

// Decide approves or rejects the refactoring of a review step that waits for
// approval; runID must be the step's run. A rejection first restores the
// workspace to the pipeline's baseline, so the refactoring is undone before
// the step fails; if the restore fails, the step keeps waiting.
func (s *ReviewPipelineService) Decide(ctx context.Context, runID, planID, stepID string, approve bool) error {
	s.decideMu.Lock()
	defer s.decideMu.Unlock()

	p, err := s.store.GetPlan(ctx, planID)
	if err != nil {
		return err
	}
	var step *plan.Step
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			step = &p.Steps[i]
		}
	}
	if step == nil || step.RunID != runID {
		return fmt.Errorf("run %s of step %s in plan %s: %w", runID, stepID, planID, domain.ErrNotFound)
	}
	if step.Status != plan.StepStatusWaitingApproval {
		return fmt.Errorf("%w: step %s is %s, not waiting for approval", domain.ErrValidation, stepID, step.Status)
	}
	proj, err := s.store.GetProject(ctx, p.ProjectID)
	if err != nil {
		return err
	}

	if !approve {
		rp, err := s.store.GetReviewPipeline(ctx, planID)
		switch {
		case errors.Is(err, domain.ErrNotFound) || (err == nil && rp.BaselineSHA == ""):
			return fmt.Errorf("%w: no baseline recorded to undo the refactoring; cancel the plan and revert the change by hand", domain.ErrValidation)
		case err != nil:
			return err
		}
		// The recorded baseline, never the agent-writable ref (V1).
		err = s.git.Run(ctx, func() error {
			repo, err := git.OpenRepo(ctx, proj.WorkspacePath)
			if err != nil {
				return err
			}
			if err := requireCommit(ctx, repo, rp.BaselineSHA); err != nil {
				return err
			}
			return restoreWorkspace(ctx, repo, rp.BaselineSHA)
		})
		if err != nil {
			return fmt.Errorf("undo the refactoring: %w", err)
		}
		if err := s.plans.RejectStep(ctx, planID, stepID); err != nil {
			return err
		}
	} else if err := s.plans.ApproveStep(ctx, planID, stepID); err != nil {
		return err
	}
	s.dropBaseline(ctx, proj.WorkspacePath, p.ID)
	slog.Info("review refactoring decided", "plan_id", planID, "step_id", stepID, "approved", approve)
	return nil
}

// PlanEnded is an orchestrator plan-end callback (AddOnPlanComplete): the
// baseline ref of a review plan that ended without a decided refactoring (a
// step failed, or the plan was cancelled) is dropped.
func (s *ReviewPipelineService) PlanEnded(ctx context.Context, planID, _ string) {
	rp, err := s.store.GetReviewPipeline(ctx, planID)
	if err != nil || rp.BaselineSHA == "" {
		return
	}
	proj, err := s.store.GetProject(ctx, rp.ProjectID)
	if err != nil || proj.WorkspacePath == "" {
		return
	}
	s.dropBaseline(ctx, proj.WorkspacePath, planID)
}

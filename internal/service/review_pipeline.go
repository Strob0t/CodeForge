package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"slices"
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
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
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

// ErrReviewNeedsGit: the review-refactor pipeline measures and undoes its
// refactoring with git, so its workspace must be a git repository.
var ErrReviewNeedsGit = fmt.Errorf("%w: the review pipeline needs a git workspace", domain.ErrValidation)

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
	UpdateTaskStatus(ctx context.Context, id string, status task.Status) error
	GetRun(ctx context.Context, id string) (*run.Run, error)
	GetPlan(ctx context.Context, id string) (*plan.ExecutionPlan, error)
	GetProjectBoundaries(ctx context.Context, projectID string) (*boundary.ProjectBoundaryConfig, error)
	UpsertProjectBoundaries(ctx context.Context, cfg *boundary.ProjectBoundaryConfig) error
	CreateReviewPipeline(ctx context.Context, rp *review.Pipeline) error
	GetReviewPipeline(ctx context.Context, planID string) (*review.Pipeline, error)
	UpdateReviewPipeline(ctx context.Context, rp *review.Pipeline, from review.PipelineState) error
	ListPendingReviewDecisions(ctx context.Context, projectID string) ([]review.Pipeline, error)
	HasActiveReviewPipeline(ctx context.Context, projectID string) (bool, error)
	GetPlanStepByRunID(ctx context.Context, runID string) (*plan.Step, error)
	ListPlansByProject(ctx context.Context, projectID string) ([]plan.ExecutionPlan, error)
	ListReviewUserEdits(ctx context.Context, planID string, limit int) ([]review.UserEdit, int, error)
	DeleteReviewUserEdits(ctx context.Context, planID string) error
}

// maxListedUserEdits caps the user edits an approval request lists (KI-94),
// so a bulk change through the file API cannot make the event or the dialog
// large; the request counts them all (UserEditsTotal).
const maxListedUserEdits = 100

// reviewPlanner creates, starts and decides the review plans.
type reviewPlanner interface {
	CreatePlan(ctx context.Context, req *plan.CreatePlanRequest) (*plan.ExecutionPlan, error)
	StartPlan(ctx context.Context, planID string) (*plan.ExecutionPlan, error)
	CancelPlan(ctx context.Context, planID string) error
	ApproveStep(ctx context.Context, planID, stepID string) error
	RejectStep(ctx context.Context, planID, stepID string) error
}

// reviewRunEnds tells whether the worker of a run the control plane stopped
// may still write to the workspace (RuntimeService, S6-F review 4).
type reviewRunEnds interface {
	WorkerMayStillWrite(runID string) bool
	WorkerStopGrace() time.Duration
}

// endedRefactoringLister finds the review pipelines whose refactoring is not
// decided although their plan ended, across tenants
// (postgres.Store.ListEndedReviewRefactorings).
type endedRefactoringLister interface {
	ListEndedReviewRefactorings(ctx context.Context, endedBefore time.Time, limit int) ([]database.EndedReviewRefactoring, error)
}

// reviewTeams creates the team whose shared context carries the review
// reports from step to step.
type reviewTeams interface {
	CreateTeam(ctx context.Context, req *agent.CreateTeamRequest) (*agent.Team, error)
	CleanupTeam(ctx context.Context, teamID string, failed bool) error
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

	// runEnds, when set, defers measuring a refactoring whose run was stopped
	// until its worker stopped writing.
	runEnds reviewRunEnds

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

// SetRunEnds lets the pipeline wait for the worker of a stopped refactoring
// run before it measures the change (S6-F review 4).
func (s *ReviewPipelineService) SetRunEnds(runs reviewRunEnds) {
	s.runEnds = runs
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
// the project, creates the plan and starts it. A pipeline with several steps
// gets a team (the reports reach later steps through its shared context). A
// pipeline that refactors needs a git workspace: its refactoring is measured
// and undone against a baseline taken when the refactorer step starts
// (PrepareStep). An error means no step is running, and what the start
// created is undone (S6-F 13).
func (s *ReviewPipelineService) start(ctx context.Context, projectID, templateID string) (*plan.ExecutionPlan, error) {
	proj, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if proj.WorkspacePath == "" {
		return nil, fmt.Errorf("%w: project has no workspace", domain.ErrValidation)
	}
	// One review pipeline per project (S6-F 7); CreateReviewPipeline guards
	// it again when the plan is recorded.
	active, err := s.store.HasActiveReviewPipeline(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if active {
		return nil, fmt.Errorf("%w: %w", domain.ErrConflict, review.ErrPipelineActive)
	}
	tmpl, err := s.pipelines.Get(templateID)
	if err != nil {
		return nil, err
	}
	refactors := slices.ContainsFunc(tmpl.Steps, func(st pipeline.Step) bool { return st.ModeID == refactorerMode })
	if refactors {
		if err := s.requireGitWorkspace(ctx, proj.WorkspacePath); err != nil {
			return nil, err
		}
	}
	ag, err := s.pickAgent(ctx, projectID)
	if err != nil {
		return nil, err
	}

	created := &reviewStart{}
	p, err := s.createAndStart(ctx, proj, ag, tmpl, created)
	if err != nil {
		s.undoStart(ctx, created)
		return nil, err
	}
	slog.Info("review pipeline started", "plan_id", p.ID, "template", templateID, "project_id", projectID)
	return p, nil
}

// reviewStart is what a review pipeline start created so far.
type reviewStart struct {
	taskIDs   []string
	teamID    string
	planID    string
	planEnded bool // the plan failed when it started
}

// undoStart removes what a failed start created: a plan that did not end is
// cancelled, the tasks are cancelled and the team is ended.
func (s *ReviewPipelineService) undoStart(ctx context.Context, created *reviewStart) {
	ctx = context.WithoutCancel(ctx)
	if created.planID != "" && !created.planEnded {
		logBestEffort(ctx, s.plans.CancelPlan(ctx, created.planID), "CancelPlan", slog.String("plan_id", created.planID))
	}
	for _, id := range created.taskIDs {
		logBestEffort(ctx, s.store.UpdateTaskStatus(ctx, id, task.StatusCancelled), "UpdateTaskStatus", slog.String("task_id", id))
	}
	if created.teamID != "" {
		logBestEffort(ctx, s.teams.CleanupTeam(ctx, created.teamID, true), "CleanupTeam", slog.String("team_id", created.teamID))
	}
}

// createAndStart creates the pipeline's tasks, team and plan, records the
// pipeline and starts the plan, noting in created what it created.
func (s *ReviewPipelineService) createAndStart(
	ctx context.Context, proj *project.Project, ag *agent.Agent, tmpl *pipeline.Template, created *reviewStart,
) (*plan.ExecutionPlan, error) {
	bindings := make([]pipeline.StepBinding, len(tmpl.Steps))
	for i, st := range tmpl.Steps {
		t, err := s.store.CreateTask(ctx, task.CreateRequest{
			ProjectID: proj.ID,
			Title:     tmpl.Name + ": " + st.Name,
			Prompt:    reviewStepPrompts[st.ModeID],
		})
		if err != nil {
			return nil, fmt.Errorf("create task for step %q: %w", st.Name, err)
		}
		created.taskIDs = append(created.taskIDs, t.ID)
		bindings[i] = pipeline.StepBinding{TaskID: t.ID, AgentID: ag.ID}
	}

	if len(tmpl.Steps) > 1 {
		team, err := s.teams.CreateTeam(ctx, &agent.CreateTeamRequest{
			ProjectID: proj.ID,
			Name:      tmpl.Name,
			Protocol:  string(tmpl.Protocol),
			Members:   []agent.CreateMemberRequest{{AgentID: ag.ID, Role: agent.RoleCoder}},
		})
		if err != nil {
			return nil, fmt.Errorf("create review team: %w", err)
		}
		created.teamID = team.ID
	}

	req, err := s.pipelines.Instantiate(ctx, tmpl.ID, pipeline.InstantiateRequest{
		ProjectID: proj.ID,
		TeamID:    created.teamID,
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
	created.planID = p.ID

	// The baseline is recorded when the refactorer step starts (PrepareStep).
	rp := &review.Pipeline{PlanID: p.ID, ProjectID: proj.ID, State: review.PipelinePending}
	if err := s.store.CreateReviewPipeline(ctx, rp); err != nil {
		return nil, fmt.Errorf("record the review pipeline: %w", err)
	}

	started, err := s.plans.StartPlan(ctx, p.ID)
	if err != nil {
		return nil, fmt.Errorf("start review plan: %w", err)
	}
	if started.Status == plan.StatusFailed {
		created.planEnded = true
		return nil, fmt.Errorf("%w: review plan %s failed to start its first step: %s", domain.ErrValidation, started.ID, firstStepError(started))
	}
	return started, nil
}

// requireGitWorkspace fails with ErrReviewNeedsGit unless dir is a git
// repository.
func (s *ReviewPipelineService) requireGitWorkspace(ctx context.Context, dir string) error {
	err := s.git.Run(ctx, func() error {
		_, err := git.OpenRepo(ctx, dir)
		return err
	})
	if errors.Is(err, git.ErrNotRepository) {
		return ErrReviewNeedsGit
	}
	if err != nil {
		return fmt.Errorf("open the workspace repository: %w", err)
	}
	return nil
}

// pickAgent returns an idle agent of the project that no plan which has not
// ended uses (an agent is idle between a plan's steps): two plans never share
// the agent (S6-F 7). CreateReviewPipeline checks it again when the plan is
// recorded.
func (s *ReviewPipelineService) pickAgent(ctx context.Context, projectID string) (*agent.Agent, error) {
	agents, err := s.store.ListAgents(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	if len(agents) == 0 {
		return nil, ErrReviewNoAgents
	}
	reserved, err := s.agentsOfActivePlans(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for i := range agents {
		if agents[i].Status == agent.StatusIdle && !reserved[agents[i].ID] {
			return &agents[i], nil
		}
	}
	return nil, fmt.Errorf("%w: every agent of the project is busy or belongs to a plan that has not ended, try again when one is free", domain.ErrValidation)
}

// agentsOfActivePlans returns the agents assigned to steps of the project's
// plans that have not ended.
func (s *ReviewPipelineService) agentsOfActivePlans(ctx context.Context, projectID string) (map[string]bool, error) {
	plans, err := s.store.ListPlansByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	reserved := map[string]bool{}
	for i := range plans {
		if plans[i].Status.IsTerminal() {
			continue
		}
		p, err := s.store.GetPlan(ctx, plans[i].ID)
		if err != nil {
			return nil, fmt.Errorf("get plan %s: %w", plans[i].ID, err)
		}
		for j := range p.Steps {
			reserved[p.Steps[j].AgentID] = true
		}
	}
	return reserved, nil
}

func firstStepError(p *plan.ExecutionPlan) string {
	for i := range p.Steps {
		if p.Steps[i].Error != "" {
			return p.Steps[i].Error
		}
	}
	return "unknown error"
}

// NeedsPreparation is the orchestrator's step preparer check
// (OrchestratorService.SetStepPreparer): a refactoring step is prepared.
func (s *ReviewPipelineService) NeedsPreparation(step *plan.Step) bool {
	return step.ModeID == refactorerMode
}

// PrepareStep records the workspace baseline of a review pipeline's
// refactoring step before its run starts (S6-F 2): the refactoring is
// measured and undone against the workspace as it is then, so edits made
// while the reports were written are kept. It runs once per pipeline: a
// re-planned refactoring step keeps the first baseline. A refactorer step of
// a plan the review pipeline did not start needs nothing. An error fails the
// step: the refactoring could be neither measured nor undone.
func (s *ReviewPipelineService) PrepareStep(ctx context.Context, step *plan.Step) error {
	rp, err := s.store.GetReviewPipeline(ctx, step.PlanID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load the review record: %w", err)
	}
	if rp.State != review.PipelinePending {
		return nil
	}
	proj, err := s.store.GetProject(ctx, rp.ProjectID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}
	err = s.git.Run(ctx, func() error {
		repo, err := git.OpenRepo(ctx, proj.WorkspacePath)
		if err != nil {
			return err
		}
		rp.BaselineSHA, err = snapshotWorkspace(ctx, repo, rp.PlanID, reviewBaselineRef(rp.PlanID), "codeforge-review-baseline")
		return err
	})
	if err != nil {
		return fmt.Errorf("record the workspace baseline: %w", err)
	}
	ours := rp.BaselineSHA
	rp.State, rp.StepID = review.PipelineRefactoring, step.ID
	err = s.store.UpdateReviewPipeline(ctx, rp, review.PipelinePending)
	if errors.Is(err, domain.ErrConflict) {
		// The pipeline moved on meanwhile - its plan ended (PlanEnded already
		// dropped the refs) or another attempt recorded its baseline: the ref
		// written here must not stay (S6-F review 8).
		s.releaseBaseline(ctx, proj.WorkspacePath, rp.PlanID, ours)
		slog.Info("review baseline not recorded: the review pipeline moved on", "plan_id", rp.PlanID, "step_id", step.ID)
		return nil
	}
	if err != nil {
		s.releaseBaseline(ctx, proj.WorkspacePath, rp.PlanID, ours)
		return fmt.Errorf("record the workspace baseline: %w", err)
	}
	slog.Info("review refactoring baseline recorded", "plan_id", rp.PlanID, "step_id", step.ID, "baseline", rp.BaselineSHA)
	return nil
}

// releaseBaseline undoes a baseline ref this process wrote but could not
// record: the ref goes back to the baseline the record holds, or is deleted
// when it holds none - each a compare-and-swap on the commit written here, so
// a ref that moved on is left alone.
func (s *ReviewPipelineService) releaseBaseline(ctx context.Context, dir, planID, written string) {
	recorded := ""
	if rp, err := s.store.GetReviewPipeline(ctx, planID); err == nil && rp.BaselineSHA != written &&
		(rp.State == review.PipelineRefactoring || rp.State == review.PipelineAwaitingDecision) {
		recorded = rp.BaselineSHA
	}
	err := s.git.Run(ctx, func() error {
		repo, err := git.OpenRepo(ctx, dir)
		if err != nil {
			return err
		}
		ref := reviewBaselineRef(planID)
		if recorded != "" {
			_, err = repo.Run(ctx, nil, "update-ref", "-m", "codeforge review", ref, recorded, written)
		} else {
			_, err = repo.Run(ctx, nil, "update-ref", "-d", ref, written)
		}
		return err
	})
	if err != nil {
		slog.Info("review baseline ref not released (moved on or gone)", "plan_id", planID, "error", err)
	}
}

// GateStep is the orchestrator's step gate (OrchestratorService.SetStepGate):
// it runs, without the plan scheduling lock, when a step's run completed. The
// boundary analyzer's result is stored as the project's boundaries; a review
// pipeline's refactoring is scored and, if its impact is high (or cannot be
// measured), the step waits for approval.
func (s *ReviewPipelineService) GateStep(ctx context.Context, step *plan.Step) (status plan.StepStatus, apply func(context.Context)) {
	switch step.ModeID {
	case boundaryAnalyzerMode:
		s.storeBoundaries(ctx, step)
	case refactorerMode:
		return s.gateRefactoring(ctx, step)
	}
	return plan.StepStatusCompleted, nil
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

// parseBoundaries reads the BOUNDARIES.json array from a run output
// (boundary.FromOutput). Invalid entries are dropped; every entry is marked
// auto-detected.
func parseBoundaries(output string) []boundary.BoundaryFile {
	entries, err := boundary.FromOutput(output)
	if err != nil {
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
// since its baseline: low impact completes the step, medium completes it and
// notifies (review.refactor_applied), high makes it wait for a decision
// (review.approval_required). A refactorer step of a plan the review
// pipeline did not start is not gated.
//
// Only the commits recorded in the Go DB count (V1): the workspace and its
// refs are agent-writable. A failed record lookup, no recorded baseline, a
// missing or moved baseline ref, a missing repository or a baseline commit
// that is gone means the change cannot be measured, and the step waits for a
// decision (fail closed).
//
// What the answer records - the decision that waits, or the finished
// pipeline - and announces is returned as apply, which the orchestrator runs
// once the step status is stored (S6-F review 9).
func (s *ReviewPipelineService) gateRefactoring(ctx context.Context, step *plan.Step) (status plan.StepStatus, apply func(context.Context)) {
	ev := event.ReviewImpactEvent{RunID: step.RunID, PlanID: step.PlanID, StepID: step.ID, ImpactLevel: string(ImpactHigh)}
	rp, err := s.store.GetReviewPipeline(ctx, step.PlanID)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return plan.StepStatusCompleted, nil
	case err != nil:
		// The dialog shows the request only with its project: take it from
		// the plan (S6-F 6). The step waits for approval either way.
		if p, perr := s.store.GetPlan(ctx, step.PlanID); perr == nil {
			ev.ProjectID = p.ProjectID
		} else {
			slog.Error("review approval request without a project: neither the review record nor the plan loads",
				"plan_id", step.PlanID, "step_id", step.ID, "error", perr)
		}
		return s.requireApproval(ctx, &ev, nil, fmt.Sprintf("the review record could not be loaded: %v", err))
	}
	ev.ProjectID = rp.ProjectID
	if rp.State != review.PipelineRefactoring {
		return s.requireApproval(ctx, &ev, rp, fmt.Sprintf("no baseline was recorded when the refactoring started (%s)", rp.State))
	}
	proj, err := s.store.GetProject(ctx, rp.ProjectID)
	if err != nil {
		return s.requireApproval(ctx, &ev, rp, fmt.Sprintf("the project could not be loaded: %v", err))
	}

	change, err := s.measure(ctx, proj.WorkspacePath, rp)
	if err != nil {
		return s.requireApproval(ctx, &ev, rp, fmt.Sprintf("the change could not be measured: %v", err))
	}
	stats := change.Stats
	stats.CrossLayer, err = s.touchesBoundary(ctx, rp.ProjectID, change.Paths)
	if err != nil {
		return s.requireApproval(ctx, &ev, rp, fmt.Sprintf("the project's boundaries could not be loaded: %v", err))
	}
	level := s.scorer.Score(stats)
	ev.ImpactLevel = string(level)
	ev.FilesChanged, ev.LinesAdded, ev.LinesRemoved = stats.FilesChanged, stats.LinesAdded, stats.LinesRemoved
	ev.CrossLayer, ev.Structural = stats.CrossLayer, stats.Structural
	slog.Info("review refactoring scored", "plan_id", rp.PlanID, "step_id", step.ID, "impact", level,
		"files", stats.FilesChanged, "lines_added", stats.LinesAdded, "lines_removed", stats.LinesRemoved)

	switch level {
	case ImpactLow, ImpactMedium:
		return plan.StepStatusCompleted, func(ctx context.Context) {
			s.finish(ctx, rp, proj.WorkspacePath, review.PipelineRefactoring)
			if level == ImpactMedium {
				s.hub.BroadcastEvent(ctx, event.EventReviewRefactorApplied, ev)
			}
		}
	default:
		return s.requireApproval(ctx, &ev, rp, "")
	}
}

// measure records the workspace now as the refactoring's result (also when
// the change cannot be measured, so it can still be undone) and measures the
// change since the recorded baseline. rp.ResultSHA is set when the result was
// recorded.
func (s *ReviewPipelineService) measure(ctx context.Context, dir string, rp *review.Pipeline) (*workspaceChange, error) {
	var change *workspaceChange
	err := s.git.Run(ctx, func() error {
		repo, err := git.OpenRepo(ctx, dir)
		if err != nil {
			return err
		}
		if rp.ResultSHA, err = snapshotWorkspace(ctx, repo, rp.PlanID, reviewResultRef(rp.PlanID), "codeforge-review-result"); err != nil {
			return err
		}
		if err := checkBaseline(ctx, repo, rp.PlanID, rp.BaselineSHA); err != nil {
			return err
		}
		change, err = changeBetween(ctx, repo, rp.BaselineSHA, rp.ResultSHA)
		return err
	})
	return change, err
}

// requireApproval makes the step wait for a decision. Its apply records the
// measured impact with the review (awaiting_decision, when rp is still
// refactoring) and broadcasts review.approval_required - once the step
// waits, so a decision is never offered for a step that does not wait yet.
func (s *ReviewPipelineService) requireApproval(_ context.Context, ev *event.ReviewImpactEvent, rp *review.Pipeline, reason string) (status plan.StepStatus, apply func(context.Context)) {
	ev.Reason = reason
	if reason != "" {
		slog.Warn("review refactoring needs approval", "plan_id", ev.PlanID, "step_id", ev.StepID, "reason", reason)
	}
	announced := *ev
	return plan.StepStatusWaitingApproval, func(ctx context.Context) {
		if rp != nil && rp.State == review.PipelineRefactoring && !s.awaitDecision(ctx, rp, &announced) {
			return
		}
		s.announceApproval(ctx, &announced)
	}
}

// announceApproval broadcasts review.approval_required with the paths users
// changed while the refactoring ran (KI-94). They are read once the decision
// is recorded, when no more are recorded for it. A failed read is logged and
// the request is announced without them: the dialog loads them again with
// the pending decisions (PendingDecisions).
func (s *ReviewPipelineService) announceApproval(ctx context.Context, ev *event.ReviewImpactEvent) {
	edits, total, err := s.store.ListReviewUserEdits(ctx, ev.PlanID, maxListedUserEdits)
	logBestEffort(ctx, err, "ListReviewUserEdits: approval request announced without the user edits",
		slog.String("plan_id", ev.PlanID))
	ev.UserEdits, ev.UserEditsTotal = edits, total
	s.hub.BroadcastEvent(ctx, event.EventReviewApprovalRequired, *ev)
}

// awaitDecision records that the measured refactoring of rp waits for the
// user's keep or undo, and reports whether it should be announced: false
// when the refactoring was decided meanwhile (the record left refactoring);
// a failed write is logged and still announced (fail closed).
func (s *ReviewPipelineService) awaitDecision(ctx context.Context, rp *review.Pipeline, ev *event.ReviewImpactEvent) bool {
	rp.State, rp.StepID, rp.RunID = review.PipelineAwaitingDecision, ev.StepID, ev.RunID
	rp.Impact = &review.Impact{
		Level: ev.ImpactLevel, FilesChanged: ev.FilesChanged, LinesAdded: ev.LinesAdded, LinesRemoved: ev.LinesRemoved,
		CrossLayer: ev.CrossLayer, Structural: ev.Structural, Reason: ev.Reason,
	}
	err := s.store.UpdateReviewPipeline(ctx, rp, review.PipelineRefactoring)
	if errors.Is(err, domain.ErrConflict) {
		slog.Info("review refactoring decided meanwhile, no decision requested", "plan_id", rp.PlanID)
		return false
	}
	logBestEffort(ctx, err, "UpdateReviewPipeline: refactoring awaits a decision", slog.String("plan_id", rp.PlanID))
	return true
}

// touchesBoundary reports whether a changed path is one of the project's
// boundary files or their counterparts: the change crosses a layer contract.
// A project without recorded boundaries has none to cross; any other lookup
// error is returned, so the caller fails closed. Paths are compared
// normalised (normalizeRepoPath): the boundary analyzer writes them freely.
func (s *ReviewPipelineService) touchesBoundary(ctx context.Context, projectID string, paths []string) (bool, error) {
	cfg, err := s.store.GetProjectBoundaries(ctx, projectID)
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	changed := make(map[string]bool, len(paths))
	for _, p := range paths {
		changed[normalizeRepoPath(p)] = true
	}
	for _, b := range cfg.Boundaries {
		if changed[normalizeRepoPath(b.Path)] || (b.Counterpart != "" && changed[normalizeRepoPath(b.Counterpart)]) {
			return true, nil
		}
	}
	return false, nil
}

// normalizeRepoPath makes a repository-relative path comparable: forward
// slashes, no "./" prefix, no "." or ".." segments.
func normalizeRepoPath(p string) string {
	return path.Clean(strings.ReplaceAll(p, "\\", "/")) // Clean also drops a "./" prefix
}

// finish marks the review pipeline done (compare-and-swap from state from)
// and drops its refs and its recorded user edits. A record that moved on
// meanwhile (another path asks for a decision, or finished it) keeps them.
func (s *ReviewPipelineService) finish(ctx context.Context, rp *review.Pipeline, dir string, from review.PipelineState) {
	rp.State = review.PipelineDone
	err := s.store.UpdateReviewPipeline(ctx, rp, from)
	if errors.Is(err, domain.ErrConflict) {
		slog.Info("review pipeline moved on, not finished here", "plan_id", rp.PlanID, "from", from)
		return
	}
	if err != nil {
		logBestEffort(ctx, err, "UpdateReviewPipeline: review done", slog.String("plan_id", rp.PlanID))
		return
	}
	// The user edits recorded during the refactoring (KI-94) were for its
	// decision; left behind they are never shown and go with the plan.
	logBestEffort(ctx, s.store.DeleteReviewUserEdits(ctx, rp.PlanID), "DeleteReviewUserEdits: review done",
		slog.String("plan_id", rp.PlanID))
	s.dropRefs(ctx, dir, rp.PlanID)
}

func (s *ReviewPipelineService) dropRefs(ctx context.Context, dir, planID string) {
	if dir == "" {
		return
	}
	err := s.git.Run(ctx, func() error {
		repo, err := git.OpenRepo(ctx, dir)
		if errors.Is(err, git.ErrNotRepository) {
			return nil
		}
		if err != nil {
			return err
		}
		return deleteReviewRefs(ctx, repo, planID)
	})
	logBestEffort(ctx, err, "delete review refs", slog.String("plan_id", planID))
}

// PendingReviewDecision is a refactoring that waits for a keep or undo
// decision, with what the approval dialog shows: the impact (as in
// review.approval_required) and where its step and plan stand.
type PendingReviewDecision struct {
	event.ReviewImpactEvent
	StepStatus string    `json:"step_status"` // waiting_approval, or failed / cancelled (S6-F 4)
	PlanStatus string    `json:"plan_status"`
	Since      time.Time `json:"since"`
}

// PendingDecisions lists the refactorings of a project that wait for a keep
// or undo decision, oldest first (S6-F 6): the dialog loads them when it
// opens and when the WebSocket reconnects, so a decision is not lost with a
// missed event. They wait until decided; there is no timeout. Each lists the
// paths users changed while it ran (KI-94); when those cannot be read the
// listing fails, as the dialog would offer an undo that sets them back
// unannounced. The project must belong to the tenant in ctx.
func (s *ReviewPipelineService) PendingDecisions(ctx context.Context, projectID string) ([]PendingReviewDecision, error) {
	if _, err := s.store.GetProject(ctx, projectID); err != nil {
		return nil, err
	}
	pending, err := s.store.ListPendingReviewDecisions(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]PendingReviewDecision, 0, len(pending))
	for i := range pending {
		rp := &pending[i]
		d := PendingReviewDecision{
			ReviewImpactEvent: event.ReviewImpactEvent{RunID: rp.RunID, PlanID: rp.PlanID, StepID: rp.StepID, ProjectID: rp.ProjectID},
			Since:             rp.UpdatedAt,
		}
		if im := rp.Impact; im != nil {
			d.ImpactLevel, d.FilesChanged, d.LinesAdded, d.LinesRemoved = im.Level, im.FilesChanged, im.LinesAdded, im.LinesRemoved
			d.CrossLayer, d.Structural, d.Reason = im.CrossLayer, im.Structural, im.Reason
		}
		if d.UserEdits, d.UserEditsTotal, err = s.store.ListReviewUserEdits(ctx, rp.PlanID, maxListedUserEdits); err != nil {
			return nil, err
		}
		if p, err := s.store.GetPlan(ctx, rp.PlanID); err == nil {
			d.PlanStatus = string(p.Status)
			for j := range p.Steps {
				if p.Steps[j].ID == rp.StepID {
					d.StepStatus = string(p.Steps[j].Status)
				}
			}
		} else {
			logBestEffort(ctx, err, "GetPlan: pending review decision without its step status", slog.String("plan_id", rp.PlanID))
		}
		out = append(out, d)
	}
	return out, nil
}

// ReviewDecision is the answer to a keep or undo decision.
type ReviewDecision struct {
	Status string `json:"status"` // approved (kept) or rejected (undone)
	// HeadRestored: the refactoring had moved HEAD (it committed) and HEAD
	// was moved back to the commit checked out at the baseline.
	HeadRestored bool `json:"head_restored"`
	// Message says what the undo could not do, e.g. why HEAD was left.
	Message       string   `json:"message,omitempty"`
	RestoredPaths []string `json:"restored_paths,omitempty"`
}

// Decide keeps (approve) or undoes (reject) the refactoring of a review
// step; runID must be the step's run. A step waiting for approval is
// approved or rejected with it. The undo is path-scoped (undoRefactoring):
// only what the refactoring changed is set back, and HEAD moves back only
// with a compare-and-swap. If the undo fails, nothing is decided.
func (s *ReviewPipelineService) Decide(ctx context.Context, runID, planID, stepID string, approve bool) (*ReviewDecision, error) {
	s.decideMu.Lock()
	defer s.decideMu.Unlock()

	p, err := s.store.GetPlan(ctx, planID)
	if err != nil {
		return nil, err
	}
	var step *plan.Step
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			step = &p.Steps[i]
		}
	}
	rp, err := s.store.GetReviewPipeline(ctx, planID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	// A pending decision is matched on the review record, which keeps the
	// run: the step's own run reference is cleared when retention purges the
	// run (S6-F review 7), and the decision must stay decidable.
	pending := step != nil && rp != nil && rp.State == review.PipelineAwaitingDecision && rp.StepID == stepID && rp.RunID == runID
	if step == nil || (step.RunID != runID && !pending) {
		return nil, fmt.Errorf("run %s of step %s in plan %s: %w", runID, stepID, planID, domain.ErrNotFound)
	}
	waiting := step.Status == plan.StepStatusWaitingApproval
	// The step waits but the gate has not recorded the decision yet (the
	// orchestrator stores the step status first): it can be kept, not undone.
	recording := waiting && !pending && rp != nil && rp.State == review.PipelineRefactoring && rp.StepID == stepID
	if !waiting && !pending {
		return nil, fmt.Errorf("%w: step %s is %s, no refactoring of it waits for a decision", domain.ErrValidation, stepID, step.Status)
	}
	proj, err := s.store.GetProject(ctx, p.ProjectID)
	if err != nil {
		return nil, err
	}

	decision := &ReviewDecision{Status: "approved"}
	if !approve {
		decision.Status = "rejected"
		if recording {
			return nil, fmt.Errorf("%w: the refactoring's measurement is still being recorded; try again in a moment", domain.ErrConflict)
		}
		if !pending || rp.BaselineSHA == "" || rp.ResultSHA == "" {
			return nil, fmt.Errorf("%w: no measured refactoring recorded to undo; keep it, or cancel the plan and revert the change by hand", domain.ErrValidation)
		}
		var out *undoOutcome
		err := s.git.Run(ctx, func() error {
			repo, err := git.OpenRepo(ctx, proj.WorkspacePath)
			if err != nil {
				return err
			}
			out, err = undoRefactoring(ctx, repo, rp.BaselineSHA, rp.ResultSHA)
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("undo the refactoring: %w", err)
		}
		decision.HeadRestored, decision.Message, decision.RestoredPaths = out.HeadRestored, out.HeadNote, out.Restored
		if waiting {
			if err := s.plans.RejectStep(ctx, planID, stepID); err != nil {
				return nil, err
			}
		}
	} else if waiting {
		if err := s.plans.ApproveStep(ctx, planID, stepID); err != nil {
			return nil, err
		}
	}
	switch {
	case pending:
		s.finish(ctx, rp, proj.WorkspacePath, review.PipelineAwaitingDecision)
	case recording && approve:
		s.finish(ctx, rp, proj.WorkspacePath, review.PipelineRefactoring)
	}
	slog.Info("review refactoring decided", "plan_id", planID, "step_id", stepID, "approved", approve,
		"head_restored", decision.HeadRestored, "restored_paths", len(decision.RestoredPaths))
	return decision, nil
}

// PlanEnded is an orchestrator plan-end callback (AddOnPlanComplete). A
// review plan whose refactoring started but was not decided - its step
// failed or was cancelled (S6-F 4) - asks keep or undo when the refactoring
// changed the workspace: the change is measured and waits for a decision
// through the same endpoints and dialog as a high-impact refactoring; the
// plan stays as it ended, and the refs are dropped only after the decision.
// A refactoring run the control plane stopped is measured only once its
// worker stopped writing (WorkerStopped, or EndUndecidedRefactorings after
// the grace; S6-F review 4). Any other review plan without a decision
// waiting is done and its refs are dropped.
func (s *ReviewPipelineService) PlanEnded(ctx context.Context, planID, status string) {
	rp, err := s.store.GetReviewPipeline(ctx, planID)
	if errors.Is(err, domain.ErrNotFound) {
		return // not a review plan
	}
	if err != nil {
		logBestEffort(ctx, err, "GetReviewPipeline: review pipeline not ended", slog.String("plan_id", planID))
		return
	}
	if rp.State == review.PipelineAwaitingDecision || rp.State == review.PipelineDone {
		return
	}
	if rp.State == review.PipelineRefactoring && status != string(plan.StatusCompleted) && s.runEnds != nil {
		if step := s.refactoringStep(ctx, rp); step != nil && step.RunID != "" && s.runEnds.WorkerMayStillWrite(step.RunID) {
			slog.Info("ended refactoring is measured once its worker stopped", "plan_id", planID, "run_id", step.RunID)
			return
		}
	}
	s.endPipeline(ctx, rp, status)
}

// WorkerStopped is the runtime's callback (RuntimeService.SetOnWorkerStopped)
// when the worker of a stopped run confirmed the stop: an undecided
// refactoring of that run whose plan ended is measured now.
func (s *ReviewPipelineService) WorkerStopped(ctx context.Context, runID string) {
	step, err := s.store.GetPlanStepByRunID(ctx, runID)
	if err != nil || step.ModeID != refactorerMode {
		return
	}
	rp, err := s.store.GetReviewPipeline(ctx, step.PlanID)
	if err != nil || rp.State != review.PipelineRefactoring || rp.StepID != step.ID {
		return
	}
	p, err := s.store.GetPlan(ctx, step.PlanID)
	if err != nil {
		logBestEffort(ctx, err, "GetPlan: stopped refactoring not measured", slog.String("plan_id", step.PlanID))
		return
	}
	if !p.Status.IsTerminal() {
		return // the plan goes on: its gate or its end measures the refactoring
	}
	s.endPipeline(ctx, rp, string(p.Status))
}

// endedRefactoringBatch limits the refactorings one watchdog check measures.
const endedRefactoringBatch = 50

// EndUndecidedRefactorings is a stuck-work watchdog check: refactorings whose
// plan ended more than the worker-stop grace ago and that are still not
// decided - the worker never confirmed the stop, or Go Core restarted - are
// measured now, each in its own tenant. It returns how many it handled.
func (s *ReviewPipelineService) EndUndecidedRefactorings(ctx context.Context, lister endedRefactoringLister) (int, error) {
	grace := defaultWorkerStopGrace
	if s.runEnds != nil {
		grace = s.runEnds.WorkerStopGrace()
	}
	ended, err := lister.ListEndedReviewRefactorings(ctx, time.Now().Add(-grace), endedRefactoringBatch)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range ended {
		tctx := tenantctx.WithTenant(ctx, e.TenantID)
		rp, err := s.store.GetReviewPipeline(tctx, e.PlanID)
		if err != nil {
			logBestEffort(tctx, err, "GetReviewPipeline: ended refactoring not measured", slog.String("plan_id", e.PlanID))
			continue
		}
		if rp.State != review.PipelineRefactoring {
			continue
		}
		s.endPipeline(tctx, rp, e.PlanStatus)
		n++
	}
	return n, nil
}

// refactoringStep returns the review pipeline's refactoring step, nil when
// it cannot be loaded.
func (s *ReviewPipelineService) refactoringStep(ctx context.Context, rp *review.Pipeline) *plan.Step {
	p, err := s.store.GetPlan(ctx, rp.PlanID)
	if err != nil {
		logBestEffort(ctx, err, "GetPlan: refactoring step not loaded", slog.String("plan_id", rp.PlanID))
		return nil
	}
	for i := range p.Steps {
		if p.Steps[i].ID == rp.StepID {
			return &p.Steps[i]
		}
	}
	return nil
}

// endPipeline ends a review pipeline whose plan ended: an undecided
// refactoring that changed the workspace asks keep or undo, anything else is
// done and its refs are dropped.
func (s *ReviewPipelineService) endPipeline(ctx context.Context, rp *review.Pipeline, status string) {
	proj, err := s.store.GetProject(ctx, rp.ProjectID)
	if err != nil {
		logBestEffort(ctx, err, "GetProject: review refs not dropped", slog.String("plan_id", rp.PlanID))
		return
	}
	if rp.State == review.PipelineRefactoring && status != string(plan.StatusCompleted) && s.askAfterEnd(ctx, rp, proj.WorkspacePath, status) {
		return
	}
	s.finish(ctx, rp, proj.WorkspacePath, rp.State)
}

// askAfterEnd measures the change of a refactoring whose plan ended without
// deciding it and, when there is one (or it cannot be measured), records a
// keep/undo decision and announces it (review.approval_required). It
// reports whether a decision waits.
func (s *ReviewPipelineService) askAfterEnd(ctx context.Context, rp *review.Pipeline, dir, status string) bool {
	step := s.refactoringStep(ctx, rp)
	if step == nil || step.RunID == "" {
		return false // the refactoring never ran
	}
	ev := event.ReviewImpactEvent{RunID: step.RunID, PlanID: rp.PlanID, StepID: step.ID, ProjectID: rp.ProjectID, ImpactLevel: string(ImpactHigh)}
	ev.Reason = fmt.Sprintf("the refactoring step ended %s (plan %s) after changing the workspace: keep or undo its change", step.Status, status)
	change, err := s.measure(ctx, dir, rp)
	if err != nil {
		ev.Reason = fmt.Sprintf("the refactoring step ended %s (plan %s) and its change could not be measured: %v", step.Status, status, err)
	} else {
		if change.Stats.FilesChanged == 0 {
			return false
		}
		stats := change.Stats
		stats.CrossLayer, err = s.touchesBoundary(ctx, rp.ProjectID, change.Paths)
		if err != nil {
			slog.Warn("ended refactoring: boundaries not loaded, counted as cross-layer", "plan_id", rp.PlanID, "error", err)
			stats.CrossLayer = true
		}
		ev.ImpactLevel = string(s.scorer.Score(stats))
		ev.FilesChanged, ev.LinesAdded, ev.LinesRemoved = stats.FilesChanged, stats.LinesAdded, stats.LinesRemoved
		ev.CrossLayer, ev.Structural = stats.CrossLayer, stats.Structural
	}
	slog.Warn("ended refactoring waits for keep or undo", "plan_id", rp.PlanID, "step_id", step.ID, "reason", ev.Reason)
	if s.awaitDecision(ctx, rp, &ev) {
		s.announceApproval(ctx, &ev)
	}
	return true
}

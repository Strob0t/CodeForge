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
	rp.State, rp.StepID = review.PipelineRefactoring, step.ID
	if err := s.store.UpdateReviewPipeline(ctx, rp, review.PipelinePending); err != nil && !errors.Is(err, domain.ErrConflict) {
		return fmt.Errorf("record the workspace baseline: %w", err)
	}
	slog.Info("review refactoring baseline recorded", "plan_id", rp.PlanID, "step_id", step.ID, "baseline", rp.BaselineSHA)
	return nil
}

// GateStep is the orchestrator's step gate (OrchestratorService.SetStepGate):
// it runs, without the plan scheduling lock, when a step's run completed. The
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
func (s *ReviewPipelineService) gateRefactoring(ctx context.Context, step *plan.Step) plan.StepStatus {
	ev := event.ReviewImpactEvent{RunID: step.RunID, PlanID: step.PlanID, StepID: step.ID, ImpactLevel: string(ImpactHigh)}
	rp, err := s.store.GetReviewPipeline(ctx, step.PlanID)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return plan.StepStatusCompleted
	case err != nil:
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
		s.finish(ctx, rp, proj.WorkspacePath, review.PipelineRefactoring)
		if level == ImpactMedium {
			s.hub.BroadcastEvent(ctx, event.EventReviewRefactorApplied, ev)
		}
		return plan.StepStatusCompleted
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

// requireApproval makes the step wait for a decision: the measured impact is
// recorded with the review (awaiting_decision, when rp is still refactoring)
// and review.approval_required is broadcast.
func (s *ReviewPipelineService) requireApproval(ctx context.Context, ev *event.ReviewImpactEvent, rp *review.Pipeline, reason string) plan.StepStatus {
	ev.Reason = reason
	if reason != "" {
		slog.Warn("review refactoring needs approval", "plan_id", ev.PlanID, "step_id", ev.StepID, "reason", reason)
	}
	if rp != nil && rp.State == review.PipelineRefactoring {
		s.awaitDecision(ctx, rp, ev)
	}
	s.hub.BroadcastEvent(ctx, event.EventReviewApprovalRequired, *ev)
	return plan.StepStatusWaitingApproval
}

// awaitDecision records that the measured refactoring of rp waits for the
// user's keep or undo.
func (s *ReviewPipelineService) awaitDecision(ctx context.Context, rp *review.Pipeline, ev *event.ReviewImpactEvent) {
	rp.State, rp.StepID, rp.RunID = review.PipelineAwaitingDecision, ev.StepID, ev.RunID
	rp.Impact = &review.Impact{
		Level: ev.ImpactLevel, FilesChanged: ev.FilesChanged, LinesAdded: ev.LinesAdded, LinesRemoved: ev.LinesRemoved,
		CrossLayer: ev.CrossLayer, Structural: ev.Structural, Reason: ev.Reason,
	}
	logBestEffort(ctx, s.store.UpdateReviewPipeline(ctx, rp, review.PipelineRefactoring),
		"UpdateReviewPipeline: refactoring awaits a decision", slog.String("plan_id", rp.PlanID))
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
// and drops its refs.
func (s *ReviewPipelineService) finish(ctx context.Context, rp *review.Pipeline, dir string, from review.PipelineState) {
	rp.State = review.PipelineDone
	logBestEffort(ctx, s.store.UpdateReviewPipeline(ctx, rp, from), "UpdateReviewPipeline: review done", slog.String("plan_id", rp.PlanID))
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
	if step == nil || step.RunID != runID {
		return nil, fmt.Errorf("run %s of step %s in plan %s: %w", runID, stepID, planID, domain.ErrNotFound)
	}
	rp, err := s.store.GetReviewPipeline(ctx, planID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	pending := rp != nil && rp.State == review.PipelineAwaitingDecision && rp.StepID == stepID && rp.RunID == runID
	waiting := step.Status == plan.StepStatusWaitingApproval
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
	if pending {
		s.finish(ctx, rp, proj.WorkspacePath, review.PipelineAwaitingDecision)
	}
	slog.Info("review refactoring decided", "plan_id", planID, "step_id", stepID, "approved", approve,
		"head_restored", decision.HeadRestored, "restored_paths", len(decision.RestoredPaths))
	return decision, nil
}

// PlanEnded is an orchestrator plan-end callback (AddOnPlanComplete): a
// review plan that ended without a refactoring waiting for a decision is
// done and its refs are dropped; a decision that waits stays.
func (s *ReviewPipelineService) PlanEnded(ctx context.Context, planID, _ string) {
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
	proj, err := s.store.GetProject(ctx, rp.ProjectID)
	if err != nil {
		logBestEffort(ctx, err, "GetProject: review refs not dropped", slog.String("plan_id", planID))
		proj = &project.Project{}
	}
	s.finish(ctx, rp, proj.WorkspacePath, rp.State)
}

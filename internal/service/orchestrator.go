package service

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"

	"github.com/Strob0t/CodeForge/internal/config"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
)

// debateState tracks a debate sub-plan back to the parent plan step that triggered it.
type debateState struct {
	ParentPlanID string
	ParentStepID string
	ProjectID    string
}

// OrchestratorService manages execution plans — multi-agent DAGs with scheduling protocols.
type OrchestratorService struct {
	store                   database.Store
	hub                     broadcast.Broadcaster
	events                  eventstore.Store
	runtime                 *RuntimeService
	orchCfg                 *config.Orchestrator
	sharedCtx               *SharedContextService
	reviewRouter            *ReviewRouterService
	stepGate                StepGate
	preparer                StepPreparer
	onPlanCompleteCallbacks []func(ctx context.Context, planID string, status string)
	// mu serializes plan scheduling: plan and step decisions are made and
	// steps are started under it. Functions named ...Locked expect it held;
	// the debate path (startStep -> startDebate -> a sub-plan's start, and a
	// debate's end -> handleDebateComplete -> the parent's advance) stays
	// under the lock it runs in and never takes it again. It is released
	// with unlock, which runs the plan-end callbacks queued meanwhile.
	mu sync.Mutex
	// planEnds are the plan-end callbacks of plans that ended while mu was
	// held; unlock runs them after releasing it (S6-F 12).
	planEnds []func()
	// gating holds the steps whose gate runs (without mu): a completion of
	// their run delivered again meanwhile is skipped.
	gating map[string]bool

	// Phase 21D: debate tracking — maps debate planID -> parent step info.
	debateMu       sync.Mutex
	debateSteps    map[string]debateState
	debatedStepIDs map[string]bool // steps that already completed a debate (skip re-evaluation)
	planMaxRounds  map[string]int  // per-plan PingPongMaxRounds override (debate sub-plans)

	// Review router decisions (KI-76): the router's LLM call runs outside mu,
	// a step waits pending until its review is decided.
	reviewMu        sync.Mutex
	reviewsInFlight map[string]bool // steps whose review is being decided
	reviewDecisions map[string]bool // step ID -> routed to a debate, decided and not yet used
	reviews         sync.WaitGroup  // review goroutines (decideReview)

	// Step preparations (S6-F 2): PrepareStep runs outside mu, a step waits
	// pending until it is prepared.
	prepMu    sync.Mutex
	preparing map[string]bool        // steps being prepared
	prepared  map[string]preparation // step ID -> outcome, not yet used
}

// AddOnPlanComplete appends a callback invoked when a plan completes or
// fails. Callbacks run after the scheduling lock is released (S6-F 12): they
// may do slow work and call back into the orchestrator.
func (s *OrchestratorService) AddOnPlanComplete(fn func(ctx context.Context, planID string, status string)) {
	s.mu.Lock()
	s.onPlanCompleteCallbacks = append(s.onPlanCompleteCallbacks, fn)
	s.mu.Unlock()
}

// SetOnPlanComplete registers a callback (backward-compatible alias for AddOnPlanComplete).
func (s *OrchestratorService) SetOnPlanComplete(fn func(ctx context.Context, planID string, status string)) {
	s.AddOnPlanComplete(fn)
}

// SetSharedContext sets the shared context service for auto-populating run outputs.
func (s *OrchestratorService) SetSharedContext(sc *SharedContextService) {
	s.sharedCtx = sc
}

// StepGate decides the status of a plan step whose run completed
// successfully: plan.StepStatusCompleted, or plan.StepStatusWaitingApproval
// to hold the plan until ApproveStep or RejectStep. It runs without the
// scheduling lock (S6-F 12), so it may do git and database work; its answer
// is dropped when the plan ended meanwhile.
type StepGate func(ctx context.Context, step *plan.Step) plan.StepStatus

// SetStepGate installs the step gate (the review pipeline's threshold HITL,
// KI-17).
func (s *OrchestratorService) SetStepGate(gate StepGate) {
	s.mu.Lock()
	s.stepGate = gate
	s.mu.Unlock()
}

// SetReviewRouter sets the review router service for confidence-based step evaluation.
func (s *OrchestratorService) SetReviewRouter(rr *ReviewRouterService) {
	s.reviewRouter = rr
}

// NewOrchestratorService creates an OrchestratorService with all dependencies.
func NewOrchestratorService(
	store database.Store,
	hub broadcast.Broadcaster,
	events eventstore.Store,
	runtime *RuntimeService,
	orchCfg *config.Orchestrator,
) *OrchestratorService {
	svc := &OrchestratorService{
		store:          store,
		hub:            hub,
		events:         events,
		runtime:        runtime,
		orchCfg:        orchCfg,
		debateSteps:    make(map[string]debateState),
		debatedStepIDs: make(map[string]bool),
		planMaxRounds:  make(map[string]int),

		reviewsInFlight: make(map[string]bool),
		reviewDecisions: make(map[string]bool),
		gating:          make(map[string]bool),
		preparing:       make(map[string]bool),
		prepared:        make(map[string]preparation),
	}
	return svc
}

// unlock releases the scheduling lock, then runs the plan-end callbacks of
// the plans that ended while it was held.
func (s *OrchestratorService) unlock() {
	ends := s.planEnds
	s.planEnds = nil
	s.mu.Unlock()
	for _, end := range ends {
		end()
	}
}

// CreatePlan validates and persists a new execution plan.
func (s *OrchestratorService) CreatePlan(ctx context.Context, req *plan.CreatePlanRequest) (*plan.ExecutionPlan, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate plan: %w", err)
	}

	maxParallel := req.MaxParallel
	if maxParallel == 0 {
		maxParallel = s.orchCfg.MaxParallel
	}

	p := &plan.ExecutionPlan{
		ProjectID:   req.ProjectID,
		TeamID:      req.TeamID,
		Name:        req.Name,
		Description: req.Description,
		Protocol:    req.Protocol,
		Status:      plan.StatusPending,
		MaxParallel: maxParallel,
	}

	// Build steps with correct initial state
	for _, sr := range req.Steps {
		p.Steps = append(p.Steps, plan.Step{
			TaskID:        sr.TaskID,
			AgentID:       sr.AgentID,
			ModeID:        sr.ModeID,
			PolicyProfile: sr.PolicyProfile,
			DeliverMode:   sr.DeliverMode,
			DependsOn:     sr.DependsOn, // indices; DB adapter remaps to UUIDs
			Status:        plan.StepStatusPending,
		})
	}

	if err := s.store.CreatePlan(ctx, p); err != nil {
		return nil, fmt.Errorf("store plan: %w", err)
	}

	s.appendPlanEvent(ctx, event.TypePlanCreated, p)
	s.broadcastPlanStatus(ctx, p)

	slog.Info("plan created", "plan_id", p.ID, "protocol", p.Protocol, "steps", len(p.Steps))
	return p, nil
}

// StartPlan transitions the plan to running and triggers the first scheduling round.
func (s *OrchestratorService) StartPlan(ctx context.Context, planID string) (*plan.ExecutionPlan, error) {
	s.mu.Lock()
	defer s.unlock()
	return s.startPlanLocked(ctx, planID)
}

// startPlanLocked is StartPlan; the caller holds s.mu.
func (s *OrchestratorService) startPlanLocked(ctx context.Context, planID string) (*plan.ExecutionPlan, error) {
	p, err := s.store.GetPlan(ctx, planID)
	if err != nil {
		return nil, err
	}
	if p.Status != plan.StatusPending {
		return nil, fmt.Errorf("plan %s is %s, expected pending", planID, p.Status)
	}

	if err := s.store.UpdatePlanStatus(ctx, planID, plan.StatusRunning); err != nil {
		return nil, err
	}
	p.Status = plan.StatusRunning

	s.appendPlanEvent(ctx, event.TypePlanStarted, p)
	s.broadcastPlanStatus(ctx, p)

	slog.Info("plan started", "plan_id", p.ID, "protocol", p.Protocol)

	s.advancePlanLocked(ctx, p)
	return p, nil
}

// GetPlan retrieves a plan with its steps.
func (s *OrchestratorService) GetPlan(ctx context.Context, id string) (*plan.ExecutionPlan, error) {
	return s.store.GetPlan(ctx, id)
}

// GetPlanGraph retrieves a plan and returns it as a frontend-friendly DAG.
func (s *OrchestratorService) GetPlanGraph(ctx context.Context, planID string) (*plan.Graph, error) {
	p, err := s.store.GetPlan(ctx, planID)
	if err != nil {
		return nil, err
	}
	return p.BuildGraph(), nil
}

// ListPlans returns all plans for a project.
func (s *OrchestratorService) ListPlans(ctx context.Context, projectID string) ([]plan.ExecutionPlan, error) {
	return s.store.ListPlansByProject(ctx, projectID)
}

// CancelPlan cancels a running plan: skips pending steps, cancels running
// runs, then ends the plan like a completed or failed one (S6-F 5): the
// debate handler and the plan-end callbacks run.
func (s *OrchestratorService) CancelPlan(ctx context.Context, planID string) error {
	p, err := s.markPlanCancelled(ctx, planID)
	if err != nil {
		return err
	}

	for i := range p.Steps {
		switch p.Steps[i].Status {
		case plan.StepStatusPending:
			logBestEffort(ctx, s.store.UpdatePlanStepStatus(ctx, p.Steps[i].ID, plan.StepStatusSkipped, "", "plan cancelled"), "UpdatePlanStepStatus", slog.String("step_id", p.Steps[i].ID))
			s.broadcastStepStatus(ctx, p, &p.Steps[i], plan.StepStatusSkipped)
		case plan.StepStatusRunning:
			if p.Steps[i].RunID != "" {
				logBestEffort(ctx, s.runtime.CancelRun(ctx, p.Steps[i].RunID), "CancelRun", slog.String("run_id", p.Steps[i].RunID))
			}
			logBestEffort(ctx, s.store.UpdatePlanStepStatus(ctx, p.Steps[i].ID, plan.StepStatusCancelled, "", "plan cancelled"), "UpdatePlanStepStatus", slog.String("step_id", p.Steps[i].ID))
			s.broadcastStepStatus(ctx, p, &p.Steps[i], plan.StepStatusCancelled)
		case plan.StepStatusWaitingApproval:
			logBestEffort(ctx, s.store.UpdatePlanStepStatus(ctx, p.Steps[i].ID, plan.StepStatusCancelled, "", "plan cancelled"), "UpdatePlanStepStatus", slog.String("step_id", p.Steps[i].ID))
			s.broadcastStepStatus(ctx, p, &p.Steps[i], plan.StepStatusCancelled)
		}
	}

	s.appendPlanEvent(ctx, event.TypePlanCancelled, p)
	s.broadcastPlanStatus(ctx, p)

	s.mu.Lock()
	s.planEnded(ctx, p.ID, plan.StatusCancelled)
	s.unlock()

	slog.Info("plan cancelled", "plan_id", planID)
	return nil
}

// markPlanCancelled cancels an active plan and returns it with the steps as
// stored at that moment. It holds the scheduling lock: a step that
// advancePlan started is in the returned steps (and gets cancelled), and
// advancePlan starts no step afterwards. The plan is cancelled before its
// runs: cancelling a run reports it through HandleRunCompleted, which must not
// start the remaining steps.
func (s *OrchestratorService) markPlanCancelled(ctx context.Context, planID string) (*plan.ExecutionPlan, error) {
	s.mu.Lock()
	defer s.unlock()

	p, err := s.store.GetPlan(ctx, planID)
	if err != nil {
		return nil, err
	}
	if p.Status != plan.StatusRunning && p.Status != plan.StatusPending {
		return nil, fmt.Errorf("plan %s is %s, cannot cancel", planID, p.Status)
	}
	if err := s.store.UpdatePlanStatus(ctx, planID, plan.StatusCancelled); err != nil {
		return nil, err
	}
	p.Status = plan.StatusCancelled
	s.forgetReviewDecisions(p.Steps, true)
	return p, nil
}

// ApproveStep transitions a step from waiting_approval to completed and resumes the plan.
func (s *OrchestratorService) ApproveStep(ctx context.Context, planID, stepID string) error {
	p, err := s.store.GetPlan(ctx, planID)
	if err != nil {
		return err
	}

	var step *plan.Step
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			step = &p.Steps[i]
			break
		}
	}
	if step == nil {
		return fmt.Errorf("step %s not found in plan %s", stepID, planID)
	}
	if step.Status != plan.StepStatusWaitingApproval {
		return fmt.Errorf("step %s is %s, not waiting_approval", stepID, step.Status)
	}

	if err := s.store.UpdatePlanStepStatus(ctx, stepID, plan.StepStatusCompleted, "", ""); err != nil {
		return err
	}

	s.broadcastStepStatus(ctx, p, step, plan.StepStatusCompleted)
	s.advancePlan(ctx, p)
	return nil
}

// RejectStep transitions a step from waiting_approval to failed and fails the plan.
func (s *OrchestratorService) RejectStep(ctx context.Context, planID, stepID string) error {
	p, err := s.store.GetPlan(ctx, planID)
	if err != nil {
		return err
	}

	var step *plan.Step
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			step = &p.Steps[i]
			break
		}
	}
	if step == nil {
		return fmt.Errorf("step %s not found in plan %s", stepID, planID)
	}
	if step.Status != plan.StepStatusWaitingApproval {
		return fmt.Errorf("step %s is %s, not waiting_approval", stepID, step.Status)
	}

	if err := s.store.UpdatePlanStepStatus(ctx, stepID, plan.StepStatusFailed, "", "rejected by user"); err != nil {
		return err
	}

	s.broadcastStepStatus(ctx, p, step, plan.StepStatusFailed)
	s.advancePlan(ctx, p)
	return nil
}

// HandleRunCompleted is the callback invoked by RuntimeService when a run finishes.
// It finds the corresponding plan step and advances the plan.
//
// A run that no plan step runs is ignored before the scheduling lock is
// taken: a step's run that fails to start ends inside startStep, under the
// lock, before the step is linked to it. Under the lock the step is read
// again and moved only while it is still running this run: a re-planned
// step runs another run, and a step that already ended keeps its status. The
// step gate runs without the lock (gateLocked).
func (s *OrchestratorService) HandleRunCompleted(ctx context.Context, runID string, status run.Status) {
	if _, err := s.store.GetPlanStepByRunID(ctx, runID); err != nil {
		// Run is not part of a plan — normal, ignore silently
		return
	}

	s.mu.Lock()
	defer s.unlock()

	step, ok := s.waitingStepLocked(ctx, runID)
	if !ok {
		return
	}

	stepStatus := plan.StepStatusCompleted
	errMsg := ""
	switch status {
	case run.StatusFailed, run.StatusTimeout:
		stepStatus = plan.StepStatusFailed
		r, err := s.store.GetRun(ctx, runID)
		if err == nil {
			errMsg = r.Error
			if s.replanStalledLocked(ctx, step, r) {
				return
			}
		}
	case run.StatusCancelled:
		stepStatus = plan.StepStatusCancelled
	}
	if stepStatus == plan.StepStatusCompleted && s.stepGate != nil {
		if stepStatus, step, ok = s.gateLocked(ctx, step, runID); !ok {
			return
		}
	}

	if err := s.store.UpdatePlanStepStatus(ctx, step.ID, stepStatus, "", errMsg); err != nil {
		slog.Error("update plan step status", "step_id", step.ID, "error", err)
		return
	}

	p, err := s.store.GetPlan(ctx, step.PlanID)
	if err != nil {
		slog.Error("get plan for advancement", "plan_id", step.PlanID, "error", err)
		return
	}

	// Auto-populate SharedContext with run output for downstream agents.
	if s.sharedCtx != nil && stepStatus == plan.StepStatusCompleted {
		r, err := s.store.GetRun(ctx, runID)
		if err == nil && r.TeamID != "" && r.Output != "" {
			_, _ = s.sharedCtx.AddItem(ctx, cfcontext.AddSharedItemRequest{
				TeamID: r.TeamID,
				Key:    "step_output:" + step.ID,
				Value:  r.Output,
				Author: r.AgentID,
			})
		}
	}

	s.broadcastStepStatus(ctx, p, step, stepStatus)
	s.advancePlanLocked(ctx, p)
}

// waitingStepLocked returns the plan step that still waits for the
// completion of runID: running this run and not being gated. The caller
// holds s.mu.
func (s *OrchestratorService) waitingStepLocked(ctx context.Context, runID string) (*plan.Step, bool) {
	step, err := s.store.GetPlanStepByRunID(ctx, runID)
	if err != nil {
		return nil, false
	}
	if step.RunID != runID || step.Status != plan.StepStatusRunning {
		slog.Info("completion of a run its plan step no longer waits for, skipped",
			"run_id", runID, "step_id", step.ID, "step_status", step.Status)
		return nil, false
	}
	if s.gating[step.ID] {
		slog.Info("completion of a run whose step is being gated, skipped", "run_id", runID, "step_id", step.ID)
		return nil, false
	}
	return step, true
}

// gateLocked runs the step gate without the scheduling lock (S6-F 12): the
// step is marked as being gated, so a completion of its run delivered again
// meanwhile is skipped; s.mu is released for the gate's git and database
// work and taken again. The step is read again: ok is false when it no
// longer waits for this run (its plan was cancelled meanwhile), and the
// gate's answer is dropped. The caller holds s.mu.
func (s *OrchestratorService) gateLocked(ctx context.Context, step *plan.Step, runID string) (plan.StepStatus, *plan.Step, bool) {
	gate := s.stepGate
	s.gating[step.ID] = true
	s.unlock()
	gated := gate(ctx, step)
	s.mu.Lock()
	delete(s.gating, step.ID)

	current, ok := s.waitingStepLocked(ctx, runID)
	if !ok {
		slog.Info("gated plan step ended meanwhile, gate result dropped", "run_id", runID, "step_id", step.ID, "gate", gated)
		return "", nil, false
	}
	return gated, current, true
}

// advancePlan is the core scheduling loop. It checks the current state of all steps
// and dispatches to the appropriate protocol handler.
func (s *OrchestratorService) advancePlan(ctx context.Context, p *plan.ExecutionPlan) {
	s.mu.Lock()
	defer s.unlock()
	s.advancePlanLocked(ctx, p)
}

// advancePlanLocked is advancePlan; the caller holds s.mu. A plan that
// cannot be read is logged and not advanced.
func (s *OrchestratorService) advancePlanLocked(ctx context.Context, p *plan.ExecutionPlan) {
	if err := s.reloadAndAdvanceLocked(ctx, p); err != nil {
		slog.Error("reload plan", "plan_id", p.ID, "error", err)
	}
}

// reloadAndAdvanceLocked advances the plan as stored now and returns the
// error of a plan that cannot be read; the caller holds s.mu. When a step
// could not be started (it ended failed), the plan is decided again at once:
// nothing else would advance it when no other step runs. Every round turns a
// pending step into a failed one, so the rounds are bounded by the steps.
func (s *OrchestratorService) reloadAndAdvanceLocked(ctx context.Context, p *plan.ExecutionPlan) error {
	for range len(p.Steps) + 1 {
		// Decide from the plan as stored now, read under the lock: CancelPlan
		// writes its status under the same lock, so no step starts after a cancel.
		stored, err := s.store.GetPlan(ctx, p.ID)
		if err != nil {
			return err
		}
		p.Status = stored.Status
		p.Steps = stored.Steps
		s.forgetReviewDecisions(p.Steps, p.Status != plan.StatusRunning)

		// Check if plan is already terminal
		if p.Status != plan.StatusRunning {
			return nil
		}

		s.skipBlockedSteps(ctx, p)

		if !s.advanceProtocol(ctx, p) {
			return nil
		}
	}
	return nil
}

// advanceProtocol makes one scheduling decision by the plan's protocol and
// reports whether a step could not be started.
func (s *OrchestratorService) advanceProtocol(ctx context.Context, p *plan.ExecutionPlan) (startFailed bool) {
	switch p.Protocol {
	case plan.ProtocolSequential:
		return s.advanceSequential(ctx, p)
	case plan.ProtocolParallel:
		return s.advanceParallel(ctx, p)
	case plan.ProtocolPingPong:
		return s.advancePingPong(ctx, p)
	case plan.ProtocolConsensus:
		return s.advanceConsensus(ctx, p)
	}
	return false
}

// blockedStepError is the error of a step skipped because a dependency did
// not complete.
const blockedStepError = "a dependency did not complete"

// skipBlockedSteps skips the pending steps that can never run because a
// dependency failed, was cancelled or was skipped; without it they would stay
// pending and the plan running forever.
func (s *OrchestratorService) skipBlockedSteps(ctx context.Context, p *plan.ExecutionPlan) {
	blocked := plan.BlockedSteps(p.Steps)
	for i := range p.Steps {
		step := &p.Steps[i]
		if !slices.Contains(blocked, step.ID) {
			continue
		}
		logBestEffort(ctx, s.store.UpdatePlanStepStatus(ctx, step.ID, plan.StepStatusSkipped, "", blockedStepError), "UpdatePlanStepStatus", slog.String("step_id", step.ID))
		step.Status = plan.StepStatusSkipped
		step.Error = blockedStepError
		s.broadcastStepStatus(ctx, p, step, plan.StepStatusSkipped)
	}
}

// advanceSequential: one step at a time. A failed or cancelled step stops the
// plan.
func (s *OrchestratorService) advanceSequential(ctx context.Context, p *plan.ExecutionPlan) (startFailed bool) {
	if plan.AnyUnsuccessful(p.Steps) {
		s.failPlan(ctx, p)
		return false
	}
	if plan.AllTerminal(p.Steps) {
		s.completePlan(ctx, p)
		return false
	}
	if plan.RunningCount(p.Steps) > 0 {
		return false // wait for current step
	}

	// If any step is waiting for approval, do not advance
	for i := range p.Steps {
		if p.Steps[i].Status == plan.StepStatusWaitingApproval {
			return false // blocked, waiting for user decision
		}
	}

	ready := plan.ReadySteps(p.Steps)
	if len(ready) > 0 {
		return !s.startStep(ctx, p, ready[0])
	}
	return false
}

// advanceParallel: start all ready steps up to MaxParallel.
func (s *OrchestratorService) advanceParallel(ctx context.Context, p *plan.ExecutionPlan) (startFailed bool) {
	if plan.AllTerminal(p.Steps) {
		if plan.AnyUnsuccessful(p.Steps) {
			s.failPlan(ctx, p)
		} else {
			s.completePlan(ctx, p)
		}
		return false
	}

	// If any step is waiting for approval, do not start new steps
	for i := range p.Steps {
		if p.Steps[i].Status == plan.StepStatusWaitingApproval {
			return false
		}
	}

	running := plan.RunningCount(p.Steps)
	maxP := p.MaxParallel
	if maxP == 0 {
		maxP = s.orchCfg.MaxParallel
	}

	ready := plan.ReadySteps(p.Steps)
	for _, stepID := range ready {
		if running >= maxP {
			break
		}
		if s.startStep(ctx, p, stepID) {
			running++
		} else {
			startFailed = true
		}
	}
	return startFailed
}

// advancePingPong: alternate between 2 steps for PingPongMaxRounds each.
func (s *OrchestratorService) advancePingPong(ctx context.Context, p *plan.ExecutionPlan) (startFailed bool) {
	if len(p.Steps) != 2 {
		slog.Error("ping_pong requires exactly 2 steps", "plan_id", p.ID)
		s.failPlan(ctx, p)
		return false
	}

	// Check for per-plan override (set by debate sub-plans), falling back to global config.
	s.debateMu.Lock()
	maxRounds, hasOverride := s.planMaxRounds[p.ID]
	s.debateMu.Unlock()
	if !hasOverride {
		maxRounds = s.orchCfg.PingPongMaxRounds
	}
	if maxRounds <= 0 {
		maxRounds = 3
	}

	s0 := &p.Steps[0]
	s1 := &p.Steps[1]

	if s0.Status.Unsuccessful() || s1.Status.Unsuccessful() {
		s.failPlan(ctx, p)
		return false
	}

	// Check if both have completed their rounds
	if s0.Round >= maxRounds && s1.Round >= maxRounds &&
		s0.Status.IsTerminal() && s1.Status.IsTerminal() {
		s.completePlan(ctx, p)
		return false
	}

	if plan.RunningCount(p.Steps) > 0 {
		return false // wait for current step
	}

	// A step whose round began but that did not start yet (its review is
	// decided in the background, or it waits for its run after its debate)
	// starts in that round. Its round counts as begun: taking it as done
	// would switch to the other step and end the plan without running it.
	for _, st := range []*plan.Step{s0, s1} {
		if st.Status == plan.StepStatusPending && st.Round > 0 {
			return !s.startStep(ctx, p, st.ID)
		}
	}

	// Determine which step goes next: alternate, starting with step 0
	// Step 0 goes on rounds: 1, 3, 5, ... ; Step 1 goes on rounds: 2, 4, 6, ...
	totalCompleted := s0.Round + s1.Round
	var next *plan.Step
	if totalCompleted%2 == 0 {
		next = s0
	} else {
		next = s1
	}

	if next.Round >= maxRounds {
		// This step is done, check the other
		if next == s0 {
			next = s1
		} else {
			next = s0
		}
	}

	if next.Round >= maxRounds {
		// Both at max rounds
		s.completePlan(ctx, p)
		return false
	}

	// Reset step to pending for next round
	newRound := next.Round + 1
	if err := s.store.UpdatePlanStepRound(ctx, next.ID, newRound); err != nil {
		slog.Error("update step round", "step_id", next.ID, "error", err)
		return false
	}
	if err := s.store.UpdatePlanStepStatus(ctx, next.ID, plan.StepStatusPending, "", ""); err != nil {
		slog.Error("reset step to pending", "step_id", next.ID, "error", err)
		return false
	}

	return !s.startStep(ctx, p, next.ID)
}

// advanceConsensus: launch all steps in parallel, evaluate quorum when all done.

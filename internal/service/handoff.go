package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/mode"
	"github.com/Strob0t/CodeForge/internal/domain/orchestration"
	"github.com/Strob0t/CodeForge/internal/domain/quarantine"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/domain/trust"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// Handoff statuses of the handoff.status event (War Room).
const (
	handoffInitiated     = "initiated"
	handoffQuarantined   = "quarantined"
	handoffRejected      = "rejected"
	handoffFailed        = "failed"
	handoffA2ADelegated  = "a2a_delegated"
	handoffTitleMaxRunes = 80

	// Claim stages (S2-G fix, 3): a handoff's request and its approval
	// after the quarantine are each carried out once.
	handoffStageRequest  = "request"
	handoffStageApproved = "approved"

	handoffDeadLettered = "the handoff could not be carried out: its retries ran out (dead-lettered)"
)

// retryableError marks a handoff failure that a retry may cure (the store,
// starting the run): the message is redelivered instead of the handoff
// being refused.
type retryableError struct{ err error }

func (e *retryableError) Error() string { return e.err.Error() }
func (e *retryableError) Unwrap() error { return e.err }

func retryable(err error) error { return &retryableError{err: err} }

func isRetryable(err error) bool {
	var r *retryableError
	return errors.As(err, &r)
}

// storeReadError is the error of a store read: retryable unless the record
// does not exist in the caller's tenant (a refusal).
func storeReadError(err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return err
	}
	return retryable(err)
}

// handoffIdentity is a handoff's ID: the worker's handoff_id or, for a
// message without one (an older worker, a handoff held before handoffs had
// IDs), one derived from the message, which its redeliveries share.
func handoffIdentity(id string, data []byte) string {
	if id != "" {
		return id
	}
	sum := sha256.Sum256(data)
	return "msg-" + hex.EncodeToString(sum[:16])
}

// handoffRunStarter starts the run a handoff hands over (RuntimeService).
type handoffRunStarter interface {
	StartRun(ctx context.Context, req *run.StartRequest) (*run.Run, error)
}

// handoffModes looks up the modes a handoff run may run in (ModeService).
type handoffModes interface {
	Get(id string) (*mode.Mode, error)
}

// HandoffService hands work from one agent to another (Phase 23B, KI-15).
// The Go Core owns the handoff: a worker's handoff_to call arrives as
// handoff.request; the source and the target are checked in the request's
// tenant and project, the handoff is screened by the quarantine, and the
// target agent gets a task of its own and a run the Go Core tracks like any
// other, an inbox message and a handoff.status event.
type HandoffService struct {
	db         database.Store
	queue      messagequeue.Queue
	hub        broadcast.Broadcaster
	quarantine *QuarantineService
	a2a        *A2AService
	runs       handoffRunStarter
	modes      handoffModes
}

// SetQuarantineService injects the quarantine evaluator (circular-dep breaker).
func (s *HandoffService) SetQuarantineService(qs *QuarantineService) { s.quarantine = qs }

// SetA2AService injects the A2A service for outbound federation (Phase 27M).
func (s *HandoffService) SetA2AService(svc *A2AService) { s.a2a = svc }

// SetRunStarter sets what starts the target agent's run (the RuntimeService).
func (s *HandoffService) SetRunStarter(rs handoffRunStarter) { s.runs = rs }

// SetModeService sets the modes a handoff's run mode is checked against.
func (s *HandoffService) SetModeService(ms handoffModes) { s.modes = ms }

// NewHandoffService creates a HandoffService. The optional hub parameter enables
// WS broadcasting for the War Room (Phase 23D).
func NewHandoffService(db database.Store, queue messagequeue.Queue, hub ...broadcast.Broadcaster) *HandoffService {
	svc := &HandoffService{db: db, queue: queue}
	if len(hub) > 0 {
		svc.hub = hub[0]
	}
	return svc
}

// CreateHandoff hands work over to another agent: msg.Context becomes the
// prompt of a new run of the target agent in msg's project, or a task of a
// remote A2A agent for an "a2a://<id>" target. The handoff belongs to the
// caller's tenant. A message without trust annotation comes from an
// internal agent. With a quarantine, the handoff is screened first: a held
// handoff starts when an admin approves it (HandleApprovedHandoff), a
// rejected one never.
func (s *HandoffService) CreateHandoff(ctx context.Context, msg *orchestration.HandoffMessage) error {
	if err := msg.Validate(); err != nil {
		return err
	}

	// The handoff belongs to the caller's tenant; a message cannot move it to another one.
	if tenantID, ok := tenantctx.Lookup(ctx); ok {
		msg.TenantID = tenantID
	} else if msg.TenantID != "" {
		ctx = tenantctx.WithTenant(ctx, msg.TenantID)
	}

	if msg.Trust == nil {
		msg.Trust = trust.Internal(msg.SourceAgentID)
	}

	verdict, err := s.screen(ctx, msg)
	switch {
	case err != nil:
		return retryable(fmt.Errorf("screen handoff: %w", err))
	case verdict == quarantine.VerdictRejected:
		s.broadcastStatus(ctx, msg, handoffRejected, "", msg.Context)
		return fmt.Errorf("handoff from %s to %s rejected by the quarantine", msg.SourceAgentID, msg.TargetAgentID)
	case verdict == quarantine.VerdictHeld:
		slog.InfoContext(ctx, "handoff quarantined", "source", msg.SourceAgentID, "target", msg.TargetAgentID)
		s.broadcastStatus(ctx, msg, handoffQuarantined, "", msg.Context)
		return nil
	}
	return s.dispatch(ctx, msg)
}

// screen returns the quarantine's verdict on a handoff (pass without a
// quarantine). A held handoff is replayed to handoff.approved when an
// admin approves it.
func (s *HandoffService) screen(ctx context.Context, msg *orchestration.HandoffMessage) (quarantine.Verdict, error) {
	if s.quarantine == nil {
		return quarantine.VerdictPass, nil
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return quarantine.VerdictRejected, fmt.Errorf("marshal handoff: %w", err)
	}
	return s.quarantine.Screen(ctx, msg.Trust, messagequeue.SubjectHandoffApproved, data, msg.ProjectID)
}

// dispatch carries out a handoff that passed the quarantine. A refusal is
// announced; a retryable failure is not (its message is redelivered).
func (s *HandoffService) dispatch(ctx context.Context, msg *orchestration.HandoffMessage) error {
	// A2A routing (Phase 27M): if target is "a2a://<remoteAgentID>", delegate to A2A.
	if strings.HasPrefix(msg.TargetAgentID, "a2a://") {
		if err := s.routeToA2A(ctx, msg); err != nil {
			s.broadcastStatus(ctx, msg, handoffFailed, "", err.Error())
			return err
		}
		return nil
	}

	r, err := s.startRun(ctx, msg)
	if err != nil {
		if !isRetryable(err) {
			s.broadcastStatus(ctx, msg, handoffFailed, "", err.Error())
		}
		return err
	}

	// Deliver inbox message to target agent.
	inboxMsg := &agent.InboxMessage{
		AgentID:   msg.TargetAgentID,
		FromAgent: msg.SourceAgentID,
		Content:   fmt.Sprintf("Handoff: %s", msg.Context),
		Priority:  1,
	}
	if err := s.db.SendAgentMessage(ctx, inboxMsg); err != nil {
		slog.WarnContext(ctx, "failed to deliver handoff inbox message", "target", msg.TargetAgentID, "error", err)
	}

	s.broadcastStatus(ctx, msg, handoffInitiated, r.ID, msg.Context)
	slog.InfoContext(ctx, "handoff dispatched",
		"source", msg.SourceAgentID,
		"target", msg.TargetAgentID,
		"run_id", r.ID,
		"plan_id", msg.PlanID,
	)
	return nil
}

// startRun gives the target agent a task with the handoff's context and
// starts its run: a run of the store, in the handoff's tenant (S2-G 1b).
// The target must be an agent of the handoff's project in its tenant.
func (s *HandoffService) startRun(ctx context.Context, msg *orchestration.HandoffMessage) (*run.Run, error) {
	if s.runs == nil {
		return nil, errors.New("handoff runs are not configured")
	}
	if msg.ProjectID == "" {
		return nil, errors.New("handoff without project")
	}
	target, err := s.db.GetAgent(ctx, msg.TargetAgentID)
	if err != nil {
		return nil, storeReadError(fmt.Errorf("handoff target agent %s: %w", msg.TargetAgentID, err))
	}
	if err := requireProject("agent", target.ID, target.ProjectID, msg.ProjectID); err != nil {
		return nil, fmt.Errorf("handoff target: %w", err)
	}
	modeID, err := s.handoffMode(target, msg.TargetModeID)
	if err != nil {
		return nil, err
	}
	t, err := s.db.CreateTask(ctx, task.CreateRequest{
		ProjectID: msg.ProjectID,
		Title:     handoffTitle(msg),
		Prompt:    handoffPrompt(msg),
	})
	if err != nil {
		return nil, retryable(fmt.Errorf("create handoff task: %w", err))
	}
	r, err := s.runs.StartRun(ctx, &run.StartRequest{
		TaskID:    t.ID,
		AgentID:   target.ID,
		ProjectID: msg.ProjectID,
		ModeID:    modeID,
	})
	if err != nil {
		// The retry creates a task of its own; this one never runs.
		logBestEffort(ctx, s.db.UpdateTaskStatus(ctx, t.ID, task.StatusFailed), "UpdateTaskStatus", slog.String("task_id", t.ID))
		return nil, retryable(fmt.Errorf("start handoff run: %w", err))
	}
	return r, nil
}

// handoffMode is the mode of a handoff's run (S2-G fix, 7): the target
// agent's configured mode; the requested one (the LLM's target_mode) only
// for an agent without a mode; "" (the run's default) when neither is set.
// The mode must be known: an unknown one refuses the handoff instead of
// running without mode. Without modes to check against, a requested mode is
// refused and a configured one is used as for any run of the agent.
func (s *HandoffService) handoffMode(target *agent.Agent, requested string) (string, error) {
	modeID, source := target.ModeID, "configured"
	if modeID == "" {
		modeID, source = requested, "requested"
	}
	if modeID == "" {
		return "", nil
	}
	if s.modes == nil {
		if source == "configured" {
			return modeID, nil
		}
		return "", fmt.Errorf("handoff mode %q cannot be checked: no modes configured", modeID)
	}
	if _, err := s.modes.Get(modeID); err != nil {
		return "", fmt.Errorf("handoff %s mode %q is unknown", source, modeID)
	}
	return modeID, nil
}

// handoffPrompt is the target run's prompt: the handoff's context and artifacts.
func handoffPrompt(msg *orchestration.HandoffMessage) string {
	prompt := fmt.Sprintf("[Handoff from %s]\n\n%s", msg.SourceAgentID, msg.Context)
	if len(msg.Artifacts) > 0 {
		prompt += "\n\nArtifacts: " + strings.Join(msg.Artifacts, ", ")
	}
	return prompt
}

// handoffTitle is the title of the handoff's task.
func handoffTitle(msg *orchestration.HandoffMessage) string {
	title := []rune("Handoff: " + strings.Join(strings.Fields(msg.Context), " "))
	if len(title) > handoffTitleMaxRunes {
		title = append(title[:handoffTitleMaxRunes-3], []rune("...")...)
	}
	return string(title)
}

// broadcastStatus announces a handoff's progress to the War Room.
func (s *HandoffService) broadcastStatus(ctx context.Context, msg *orchestration.HandoffMessage, status, runID, detail string) {
	if s.hub == nil {
		return
	}
	s.hub.BroadcastEvent(ctx, event.EventHandoffStatus, event.HandoffStatusEvent{
		SourceAgentID: msg.SourceAgentID,
		TargetAgentID: msg.TargetAgentID,
		PlanID:        msg.PlanID,
		StepID:        msg.StepID,
		RunID:         runID,
		Status:        status,
		Context:       detail,
	})
}

// routeToA2A delegates a handoff to a remote A2A agent (Phase 27M).
func (s *HandoffService) routeToA2A(ctx context.Context, msg *orchestration.HandoffMessage) error {
	if s.a2a == nil {
		return fmt.Errorf("a2a service not configured for target %s", msg.TargetAgentID)
	}

	remoteAgentID := strings.TrimPrefix(msg.TargetAgentID, "a2a://")
	if remoteAgentID == "" {
		return fmt.Errorf("empty remote agent ID in a2a:// target")
	}

	dt, err := s.a2a.SendTask(ctx, remoteAgentID, msg.StepID, msg.Context)
	if err != nil {
		return fmt.Errorf("a2a handoff to %s: %w", remoteAgentID, err)
	}

	s.broadcastStatus(ctx, msg, handoffA2ADelegated, "", fmt.Sprintf("A2A task %s created", dt.ID))
	slog.InfoContext(ctx, "handoff delegated to a2a",
		"source", msg.SourceAgentID,
		"target", msg.TargetAgentID,
		"remote_agent", remoteAgentID,
		"a2a_task", dt.ID,
	)
	return nil
}

// HandleHandoffRequest handles a worker's handoff.request (its handoff_to
// tool call). The request is checked in its tenant: its source (the
// conversation or run that called the tool) must be of the request's
// project. The handoff's content was written by an LLM, so it is screened
// with partial trust whatever the worker stamped on it. The request is
// carried out once (see once): a refused handoff is announced
// (handoff.status) and not retried, a transient failure is retried.
func (s *HandoffService) HandleHandoffRequest(ctx context.Context, data []byte) error {
	var req messagequeue.HandoffRequestPayload
	if err := json.Unmarshal(data, &req); err != nil {
		slog.Warn("unreadable handoff request, dropped", "error", err)
		return nil
	}
	ctx = withPayloadTenant(ctx, req.TenantID)
	handoffID := handoffIdentity(req.HandoffID, data)
	// Until the source is known, the handoff is announced from the
	// conversation or run that called handoff_to.
	announced := &orchestration.HandoffMessage{
		HandoffID: handoffID, SourceAgentID: req.SourceRunID, TargetAgentID: req.TargetAgentID, PlanID: req.PlanID, StepID: req.StepID,
	}
	return s.once(ctx, handoffID, handoffStageRequest, announced, func() error {
		return s.handleRequest(ctx, &req, announced)
	})
}

// handleRequest checks a worker's handoff request and hands the work over.
func (s *HandoffService) handleRequest(ctx context.Context, req *messagequeue.HandoffRequestPayload, announced *orchestration.HandoffMessage) error {
	source, err := s.handoffSource(ctx, req.SourceRunID, req.ProjectID)
	if err != nil {
		if !isRetryable(err) {
			s.broadcastStatus(ctx, announced, handoffFailed, "", "handoff refused: "+err.Error())
		}
		return err
	}
	msg := &orchestration.HandoffMessage{
		HandoffID:     announced.HandoffID,
		TenantID:      tenantctx.FromContext(ctx),
		ProjectID:     req.ProjectID,
		SourceAgentID: source,
		TargetAgentID: req.TargetAgentID,
		TargetModeID:  req.TargetModeID,
		Context:       req.Context,
		Artifacts:     req.Artifacts,
		PlanID:        req.PlanID,
		StepID:        req.StepID,
		Metadata:      req.Metadata,
		Trust: &trust.Annotation{
			Origin:     "handoff",
			TrustLevel: trust.LevelPartial,
			SourceID:   req.SourceRunID,
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
		},
	}
	return s.CreateHandoff(ctx, msg)
}

// once carries out a stage of the handoff handoffID at most once (S2-G fix,
// 3): the stage is claimed before anything starts, so a redelivered message
// does nothing. A refusal (carryOut announced it) settles the message. A
// retryable failure releases the claim and is returned, so the message is
// redelivered; its last delivery is dead-lettered and announced failed
// (HandleDeadLetteredHandoff). A claim that cannot be released would turn
// the retry into a no-op: the handoff fails at once then.
func (s *HandoffService) once(ctx context.Context, handoffID, stage string, announced *orchestration.HandoffMessage, carryOut func() error) error {
	claimed, err := s.db.ClaimHandoff(ctx, handoffID, stage)
	if err != nil {
		return fmt.Errorf("claim handoff %s: %w", handoffID, err)
	}
	if !claimed {
		slog.InfoContext(ctx, "handoff already carried out, redelivery ignored", "handoff_id", handoffID, "stage", stage)
		return nil
	}
	err = carryOut()
	if err == nil {
		return nil
	}
	if !isRetryable(err) {
		slog.WarnContext(ctx, "handoff refused", "handoff_id", handoffID, "stage", stage, "error", err)
		return nil
	}
	if rerr := s.db.ReleaseHandoff(ctx, handoffID, stage); rerr != nil {
		slog.ErrorContext(ctx, "handoff failed and its claim could not be released for a retry",
			"handoff_id", handoffID, "stage", stage, "error", err, "release_error", rerr)
		s.broadcastStatus(ctx, announced, handoffFailed, "", err.Error())
		return nil
	}
	slog.WarnContext(ctx, "handoff failed, retried", "handoff_id", handoffID, "stage", stage, "error", err)
	return err
}

// handoffSource returns the agent that hands over: the conversation that
// called handoff_to (its ID: a conversation has no agent), or the agent of
// the run that called it. The source must exist in ctx's tenant and belong
// to projectID.
func (s *HandoffService) handoffSource(ctx context.Context, sourceRunID, projectID string) (string, error) {
	if sourceRunID == "" {
		return "", errors.New("handoff request without source_run_id")
	}
	conv, err := s.db.GetConversation(ctx, sourceRunID)
	switch {
	case err == nil && conv != nil:
		if err := requireProject("conversation", conv.ID, conv.ProjectID, projectID); err != nil {
			return "", err
		}
		return conv.ID, nil
	case err != nil && !errors.Is(err, domain.ErrNotFound):
		return "", retryable(fmt.Errorf("handoff source %s: %w", sourceRunID, err))
	}
	r, err := s.db.GetRun(ctx, sourceRunID)
	if err != nil {
		return "", storeReadError(fmt.Errorf("handoff source %s: %w", sourceRunID, err))
	}
	if err := requireProject("run", r.ID, r.ProjectID, projectID); err != nil {
		return "", err
	}
	return r.AgentID, nil
}

// HandleApprovedHandoff carries out a handoff an admin released from the
// quarantine (Approve replays it to handoff.approved). It is checked like
// any handoff in its tenant but not screened again, and carried out once
// (see once).
func (s *HandoffService) HandleApprovedHandoff(ctx context.Context, data []byte) error {
	var msg orchestration.HandoffMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		slog.Warn("unreadable approved handoff, dropped", "error", err)
		return nil
	}
	if err := msg.Validate(); err != nil {
		slog.Warn("invalid approved handoff, dropped", "error", err)
		return nil
	}
	ctx = withPayloadTenant(ctx, msg.TenantID)
	msg.HandoffID = handoffIdentity(msg.HandoffID, data)
	return s.once(ctx, msg.HandoffID, handoffStageApproved, &msg, func() error {
		return s.dispatch(ctx, &msg)
	})
}

// HandleDeadLetteredHandoff announces a handoff whose retries ran out
// (handoff.request.dlq, handoff.approved.dlq) as failed, in its tenant. A
// message that cannot be read is dropped.
func (s *HandoffService) HandleDeadLetteredHandoff(ctx context.Context, subject string, data []byte) error {
	msg, ok := deadLetteredHandoff(subject, data)
	if !ok {
		slog.Warn("unreadable dead-lettered handoff, dropped", "subject", subject)
		return nil
	}
	ctx = withPayloadTenant(ctx, msg.TenantID)
	slog.WarnContext(ctx, "handoff dead-lettered, announced failed",
		"subject", subject, "handoff_id", msg.HandoffID, "target", msg.TargetAgentID)
	s.broadcastStatus(ctx, msg, handoffFailed, "", handoffDeadLettered)
	return nil
}

// deadLetteredHandoff reads a dead-lettered handoff message of subject.
func deadLetteredHandoff(subject string, data []byte) (*orchestration.HandoffMessage, bool) {
	switch strings.TrimSuffix(subject, deadLetterSuffix) {
	case messagequeue.SubjectHandoffRequest:
		var req messagequeue.HandoffRequestPayload
		if json.Unmarshal(data, &req) != nil {
			return nil, false
		}
		return &orchestration.HandoffMessage{
			HandoffID: req.HandoffID, TenantID: req.TenantID, SourceAgentID: req.SourceRunID,
			TargetAgentID: req.TargetAgentID, PlanID: req.PlanID, StepID: req.StepID,
		}, true
	case messagequeue.SubjectHandoffApproved:
		var msg orchestration.HandoffMessage
		if json.Unmarshal(data, &msg) != nil {
			return nil, false
		}
		return &msg, true
	}
	return nil, false
}

// StartSubscribers subscribes to the workers' handoff requests, to the
// handoffs released from the quarantine and to both dead-letter subjects.
func (s *HandoffService) StartSubscribers(ctx context.Context) (cancel func(), err error) {
	deadLettered := func(msgCtx context.Context, subject string, data []byte) error {
		return s.HandleDeadLetteredHandoff(msgCtx, subject, data)
	}
	subscriptions := []struct {
		subject string
		handler messagequeue.Handler
	}{
		{messagequeue.SubjectHandoffRequest, func(msgCtx context.Context, _ string, data []byte) error {
			return s.HandleHandoffRequest(msgCtx, data)
		}},
		{messagequeue.SubjectHandoffApproved, func(msgCtx context.Context, _ string, data []byte) error {
			return s.HandleApprovedHandoff(msgCtx, data)
		}},
		{messagequeue.SubjectHandoffRequest + deadLetterSuffix, deadLettered},
		{messagequeue.SubjectHandoffApproved + deadLetterSuffix, deadLettered},
	}
	var cancels []func()
	cancelAll := func() {
		for _, c := range cancels {
			c()
		}
	}
	for _, sub := range subscriptions {
		c, err := s.queue.Subscribe(ctx, sub.subject, sub.handler)
		if err != nil {
			cancelAll()
			return nil, fmt.Errorf("subscribe %s: %w", sub.subject, err)
		}
		cancels = append(cancels, c)
	}
	return cancelAll, nil
}

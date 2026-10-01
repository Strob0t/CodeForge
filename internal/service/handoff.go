package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/event"
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
)

// handoffRunStarter starts the run a handoff hands over (RuntimeService).
type handoffRunStarter interface {
	StartRun(ctx context.Context, req *run.StartRequest) (*run.Run, error)
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
}

// SetQuarantineService injects the quarantine evaluator (circular-dep breaker).
func (s *HandoffService) SetQuarantineService(qs *QuarantineService) { s.quarantine = qs }

// SetA2AService injects the A2A service for outbound federation (Phase 27M).
func (s *HandoffService) SetA2AService(svc *A2AService) { s.a2a = svc }

// SetRunStarter sets what starts the target agent's run (the RuntimeService).
func (s *HandoffService) SetRunStarter(rs handoffRunStarter) { s.runs = rs }

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
	case err != nil || verdict == quarantine.VerdictRejected:
		s.broadcastStatus(ctx, msg, handoffRejected, "", msg.Context)
		if err != nil {
			return fmt.Errorf("handoff rejected: screening failed: %w", err)
		}
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

// dispatch carries out a handoff that passed the quarantine.
func (s *HandoffService) dispatch(ctx context.Context, msg *orchestration.HandoffMessage) error {
	// A2A routing (Phase 27M): if target is "a2a://<remoteAgentID>", delegate to A2A.
	if strings.HasPrefix(msg.TargetAgentID, "a2a://") {
		return s.routeToA2A(ctx, msg)
	}

	r, err := s.startRun(ctx, msg)
	if err != nil {
		s.broadcastStatus(ctx, msg, handoffFailed, "", err.Error())
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
		return nil, fmt.Errorf("handoff target agent %s: %w", msg.TargetAgentID, err)
	}
	if err := requireProject("agent", target.ID, target.ProjectID, msg.ProjectID); err != nil {
		return nil, fmt.Errorf("handoff target: %w", err)
	}
	t, err := s.db.CreateTask(ctx, task.CreateRequest{
		ProjectID: msg.ProjectID,
		Title:     handoffTitle(msg),
		Prompt:    handoffPrompt(msg),
	})
	if err != nil {
		return nil, fmt.Errorf("create handoff task: %w", err)
	}
	r, err := s.runs.StartRun(ctx, &run.StartRequest{
		TaskID:    t.ID,
		AgentID:   target.ID,
		ProjectID: msg.ProjectID,
		ModeID:    msg.TargetModeID,
	})
	if err != nil {
		return nil, fmt.Errorf("start handoff run: %w", err)
	}
	return r, nil
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
// handled at most once: a refused or failed handoff is logged and announced
// (handoff.status), not retried.
func (s *HandoffService) HandleHandoffRequest(ctx context.Context, data []byte) error {
	var req messagequeue.HandoffRequestPayload
	if err := json.Unmarshal(data, &req); err != nil {
		slog.Warn("unreadable handoff request, dropped", "error", err)
		return nil
	}
	ctx = withPayloadTenant(ctx, req.TenantID)

	source, err := s.handoffSource(ctx, req.SourceRunID, req.ProjectID)
	if err != nil {
		slog.WarnContext(ctx, "handoff request refused", "source_run_id", req.SourceRunID, "target", req.TargetAgentID, "error", err)
		return nil
	}
	msg := &orchestration.HandoffMessage{
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
	if err := s.CreateHandoff(ctx, msg); err != nil {
		slog.WarnContext(ctx, "handoff request failed", "source", source, "target", req.TargetAgentID, "error", err)
	}
	return nil
}

// handoffSource returns the agent that hands over: the conversation that
// called handoff_to (its ID: a conversation has no agent), or the agent of
// the run that called it. The source must exist in ctx's tenant and belong
// to projectID.
func (s *HandoffService) handoffSource(ctx context.Context, sourceRunID, projectID string) (string, error) {
	if sourceRunID == "" {
		return "", errors.New("handoff request without source_run_id")
	}
	if conv, err := s.db.GetConversation(ctx, sourceRunID); err == nil && conv != nil {
		if err := requireProject("conversation", conv.ID, conv.ProjectID, projectID); err != nil {
			return "", err
		}
		return conv.ID, nil
	}
	r, err := s.db.GetRun(ctx, sourceRunID)
	if err != nil {
		return "", fmt.Errorf("handoff source %s: %w", sourceRunID, err)
	}
	if err := requireProject("run", r.ID, r.ProjectID, projectID); err != nil {
		return "", err
	}
	return r.AgentID, nil
}

// HandleApprovedHandoff carries out a handoff an admin released from the
// quarantine (Approve replays it to handoff.approved). It is checked like
// any handoff in its tenant but not screened again. Handled at most once.
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
	if err := s.dispatch(ctx, &msg); err != nil {
		slog.WarnContext(ctx, "approved handoff failed", "source", msg.SourceAgentID, "target", msg.TargetAgentID, "error", err)
	}
	return nil
}

// StartSubscribers subscribes to the workers' handoff requests and to the
// handoffs released from the quarantine.
func (s *HandoffService) StartSubscribers(ctx context.Context) (cancel func(), err error) {
	cancelRequests, err := s.queue.Subscribe(ctx, messagequeue.SubjectHandoffRequest, func(msgCtx context.Context, _ string, data []byte) error {
		return s.HandleHandoffRequest(msgCtx, data)
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe %s: %w", messagequeue.SubjectHandoffRequest, err)
	}
	cancelApproved, err := s.queue.Subscribe(ctx, messagequeue.SubjectHandoffApproved, func(msgCtx context.Context, _ string, data []byte) error {
		return s.HandleApprovedHandoff(msgCtx, data)
	})
	if err != nil {
		cancelRequests()
		return nil, fmt.Errorf("subscribe %s: %w", messagequeue.SubjectHandoffApproved, err)
	}
	return func() {
		cancelRequests()
		cancelApproved()
	}, nil
}

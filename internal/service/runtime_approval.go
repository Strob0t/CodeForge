package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/feedback"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	feedbackPort "github.com/Strob0t/CodeForge/internal/port/feedback"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// --- HITL (Human-in-the-Loop) approval ---

// approvalTimeoutSeconds is the approval timeout sent to the worker with a
// run start (runs and agentic conversation runs).
func approvalTimeoutSeconds(cfg *config.Runtime) int {
	return int(cfg.ApprovalTimeout() / time.Second)
}

// approvalKey builds the key of a pending approval. It names the tenant that
// owns the run: only that tenant can resolve the approval (KI-63). The run
// ID comes first so that run cleanup finds the run's approvals.
func approvalKey(tenantID, runID, callID string) string {
	return runID + ":" + callID + "@" + tenantID
}

// waitForApproval broadcasts a permission request to the frontend and all registered
// feedback providers, then blocks until the first response (via ResolveApproval or
// provider callback) or the timeout expires. Returns the final decision.
func (s *RuntimeService) waitForApproval(ctx context.Context, req *event.AGUIPermissionRequestEvent) policy.Decision {
	runID, callID, tool, command, path := req.RunID, req.CallID, req.Tool, req.Command, req.Path

	// The worker got the same timeout with the run start and waits for the
	// response longer than this (KI-21).
	timeout := s.runtimeCfg.ApprovalTimeout()

	ch := make(chan string, 1)
	// ctx is scoped to the tenant that owns the run or conversation.
	key := approvalKey(tenantctx.FromContext(ctx), runID, callID)
	s.state.SetPendingApproval(key, ch)
	s.state.SetPendingApprovalRequest(key, tenantctx.FromContext(ctx), req)
	defer s.state.DeletePendingApproval(key)
	defer s.serveApproval(key)()

	// Broadcast permission request to connected WebSocket clients.
	s.hub.BroadcastEvent(ctx, event.AGUIPermissionRequest, *req)

	// Fan out to registered feedback providers (Slack, Email, etc.).
	// First response wins — the channel `ch` has buffer=1 so only the first write lands.
	s.feedbackProvidersMu.RLock()
	providers := s.feedbackProviders
	s.feedbackProvidersMu.RUnlock()
	for _, p := range providers {
		go func(provider feedbackPort.Provider) {
			tenantID, _ := tenantctx.Lookup(ctx)
			fbReq := feedback.FeedbackRequest{
				TenantID:         tenantID,
				RunID:            runID,
				CallID:           callID,
				Tool:             tool,
				Command:          command,
				Path:             path,
				Profile:          req.Profile,
				ArgumentsPreview: req.ArgumentsPreview,
			}
			result, err := provider.RequestFeedback(ctx, fbReq)
			if err != nil {
				slog.Warn("feedback provider failed",
					"provider", provider.Name(),
					"error", err,
				)
				return
			}
			if result.Decision != "" {
				select {
				case ch <- string(result.Decision):
					slog.Info("feedback received from provider",
						"provider", provider.Name(),
						"decision", result.Decision,
					)
				default:
				}
			}
		}(p)
	}

	slog.Info("HITL approval requested",
		"run_id", runID,
		"call_id", callID,
		"tool", tool,
		"timeout", timeout,
		"providers", len(providers),
	)

	select {
	case decision := <-ch:
		if decision == "allow" {
			return policy.DecisionAllow
		}
		return policy.DecisionDeny
	case <-time.After(timeout):
		slog.Warn("HITL approval timed out, denying",
			"run_id", runID,
			"call_id", callID,
			"tool", tool,
		)
		return policy.DecisionDeny
	case <-ctx.Done():
		return policy.DecisionDeny
	}
}

// PendingApproval returns the tool call of runID/callID that awaits a
// decision, if it does and the run belongs to the caller's tenant
// (domain.ErrNotFound otherwise: answered, timed out, the run ended, or
// another tenant's). The web UI's approval page shows it.
func (s *RuntimeService) PendingApproval(ctx context.Context, runID, callID string) (*event.AGUIPermissionRequestEvent, error) {
	tenant := tenantctx.FromContext(ctx)
	key := approvalKey(tenant, runID, callID)
	if tenantID, req, ok := s.state.PendingApprovalRequest(key); ok && tenantID == tenant {
		return &req, nil
	}
	if answer := s.relayApproval(ctx, key, approvalLookup); answer != nil {
		var req event.AGUIPermissionRequestEvent
		if err := json.Unmarshal(answer, &req); err == nil {
			return &req, nil
		}
	}
	return nil, fmt.Errorf("no pending approval for run %s call %s: %w", runID, callID, domain.ErrNotFound)
}

// ResolveApproval is called from the HTTP handler when a user approves or denies
// a pending tool call. Returns true if a pending approval of the caller's
// tenant (from ctx) was found and resolved - on this replica or, through the
// relay, on the replica whose run waits for it (KI-86); another tenant's
// approval is not found.
func (s *RuntimeService) ResolveApproval(ctx context.Context, runID, callID, decision string) bool {
	if decision != approvalAllow && decision != approvalDeny {
		return false
	}
	key := approvalKey(tenantctx.FromContext(ctx), runID, callID)
	return s.resolveLocalApproval(key, decision) || s.relayApproval(ctx, key, decision) != nil
}

// Approval decisions, and the relay request that asks for the pending call.
const (
	approvalAllow  = "allow"
	approvalDeny   = "deny"
	approvalLookup = "lookup"
)

// approvalRelayKey is the relay key of a pending approval; key names the
// tenant, so a decision or lookup of another tenant meets no waiter.
func approvalRelayKey(key string) string {
	return "approval:" + key
}

// resolveLocalApproval hands decision to the approval of key that waits on
// this replica; once.
func (s *RuntimeService) resolveLocalApproval(key, decision string) bool {
	ch, ok := s.state.LoadAndDeletePendingApproval(key)
	if !ok {
		return false
	}
	select {
	case ch <- decision:
		return true
	default:
		return false
	}
}

// serveApproval lets the other Go Core replica decide or look up the
// approval of key while it waits here (KI-86) and returns the stop.
func (s *RuntimeService) serveApproval(key string) func() {
	relay := messagequeue.RelayOf(s.queue)
	if relay == nil {
		return func() {}
	}
	stop, err := relay.Serve(approvalRelayKey(key), func(data []byte) []byte {
		switch msg := string(data); msg {
		case approvalLookup:
			if _, req, ok := s.state.PendingApprovalRequest(key); ok {
				if answer, err := json.Marshal(req); err == nil {
					return answer
				}
			}
		case approvalAllow, approvalDeny:
			if s.resolveLocalApproval(key, msg) {
				return []byte("resolved")
			}
		}
		return nil
	})
	if err != nil {
		slog.Warn("approval decidable on this replica only", "error", err)
		return func() {}
	}
	return stop
}

// relayApproval sends msg (a decision or the lookup) to the replica whose
// run waits for the approval of key and returns its answer, nil when none
// waits or the relay failed.
func (s *RuntimeService) relayApproval(ctx context.Context, key, msg string) []byte {
	relay := messagequeue.RelayOf(s.queue)
	if relay == nil {
		return nil
	}
	answer, err := relay.Request(ctx, approvalRelayKey(key), []byte(msg))
	if err != nil {
		slog.Warn("approval relay failed", "error", err)
		return nil
	}
	return answer
}

// LogFeedbackAudit records a feedback decision in the audit trail.
func (s *RuntimeService) LogFeedbackAudit(ctx context.Context, runID, callID, tool, provider, decision, responder string) error {
	a := &feedback.AuditEntry{
		RunID:     runID,
		CallID:    callID,
		Tool:      tool,
		Provider:  feedback.Provider(provider),
		Decision:  feedback.Decision(decision),
		Responder: responder,
	}
	return s.store.CreateFeedbackAudit(ctx, a)
}

// ListFeedbackAudit returns the feedback audit trail for a run.
func (s *RuntimeService) ListFeedbackAudit(ctx context.Context, runID string) ([]feedback.AuditEntry, error) {
	return s.store.ListFeedbackByRun(ctx, runID)
}

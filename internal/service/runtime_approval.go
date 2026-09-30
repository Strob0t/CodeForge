package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/feedback"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	feedbackPort "github.com/Strob0t/CodeForge/internal/port/feedback"
)

// --- HITL (Human-in-the-Loop) approval ---

// approvalTimeoutSeconds is the approval timeout sent to the worker with a
// run start (runs and agentic conversation runs).
func approvalTimeoutSeconds(cfg *config.Runtime) int {
	return int(cfg.ApprovalTimeout() / time.Second)
}

// approvalKey builds a unique key for pending approval channels.
func approvalKey(runID, callID string) string {
	return runID + ":" + callID
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
	key := approvalKey(runID, callID)
	s.state.SetPendingApproval(key, ch)
	defer s.state.DeletePendingApproval(key)

	// Broadcast permission request to connected WebSocket clients.
	s.hub.BroadcastEvent(ctx, event.AGUIPermissionRequest, *req)

	// Fan out to registered feedback providers (Slack, Email, etc.).
	// First response wins — the channel `ch` has buffer=1 so only the first write lands.
	s.feedbackProvidersMu.RLock()
	providers := s.feedbackProviders
	s.feedbackProvidersMu.RUnlock()
	for _, p := range providers {
		go func(provider feedbackPort.Provider) {
			fbReq := feedback.FeedbackRequest{
				RunID:   runID,
				CallID:  callID,
				Tool:    tool,
				Command: command,
				Path:    path,
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

// ResolveApproval is called from the HTTP handler when a user approves or denies
// a pending tool call. Returns true if a pending approval was found and resolved.
func (s *RuntimeService) ResolveApproval(runID, callID, decision string) bool {
	key := approvalKey(runID, callID)
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

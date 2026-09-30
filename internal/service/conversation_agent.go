package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// reminderTemplateData holds the data passed to Go text/template for reminder entries.
type reminderTemplateData struct {
	TurnCount       int
	BudgetPercent   float64
	BudgetUsed      string
	BudgetLimit     string
	StallIterations int
}

// buildSessionMeta extracts session operation metadata (resume/fork/rewind) from a Session's
// Metadata JSON field and returns a SessionMetaPayload for the NATS payload. Returns nil
// if there is no meaningful session operation context.
func buildSessionMeta(sess *run.Session) *messagequeue.SessionMetaPayload {
	if sess.Metadata == "" || sess.Metadata == "{}" {
		return nil
	}
	var meta map[string]string
	if err := json.Unmarshal([]byte(sess.Metadata), &meta); err != nil {
		return nil
	}
	sm := &messagequeue.SessionMetaPayload{
		ParentSessionID: sess.ParentSessionID,
		ParentRunID:     sess.ParentRunID,
	}
	switch {
	case meta["resumed_from"] != "":
		sm.Operation = "resume"
	case meta["forked_from"] != "" || meta["forked_from_conversation"] != "":
		sm.Operation = "fork"
		sm.ForkEventID = meta["from_event"]
	case meta["rewound_from"] != "":
		sm.Operation = "rewind"
		sm.RewindEventID = meta["to_event"]
	}
	if sm.Operation == "" {
		return nil
	}
	return sm
}

func (s *ConversationService) IsAgentic(ctx context.Context, conversationID string, req *conversation.SendMessageRequest) bool {
	if req.Agentic != nil {
		return *req.Agentic
	}
	// No queue means no worker dispatch capability.
	if s.queue == nil {
		return false
	}
	// Default from agent config.
	if s.agentCfg == nil || !s.agentCfg.AgenticByDefault {
		return false
	}
	// Agentic mode requires a workspace path on the project.
	conv, err := s.db.GetConversation(ctx, conversationID)
	if err != nil {
		return false
	}
	proj, err := s.db.GetProject(ctx, conv.ProjectID)
	if err != nil {
		return false
	}
	return proj.WorkspacePath != ""
}

// toolOutputMaxChars returns agent.tool_output_max_chars, or 0 (the
// worker's default) when agentCfg is nil.
func (s *ConversationService) toolOutputMaxChars() int {
	if s.agentCfg != nil {
		return s.agentCfg.ToolOutputMaxChars
	}
	return 0
}

// summarizeThreshold returns the configured auto-summarization threshold,
// or 0 (disabled) when agentCfg is nil.
func (s *ConversationService) summarizeThreshold() int {
	if s.agentCfg != nil {
		return s.agentCfg.SummarizeThreshold
	}
	return 0
}

// HandleConversationRunComplete processes the completion message from the Python worker.
// It stores the assistant message and intermediate tool messages, then broadcasts the
// run finished event.
func (s *ConversationService) HandleConversationRunComplete(ctx context.Context, _ string, data []byte) error {
	var payload messagequeue.ConversationRunCompletePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("unmarshal conversation run complete: %w", err)
	}

	// Idempotency is handled by unique Nats-Msg-Id headers on the Python side.
	// No application-level dedup here — RunID equals ConversationID, so a map-based
	// guard would block legitimate follow-up completions in the same conversation.

	// Inject tenant context from NATS payload (background consumer has no tenant).
	if payload.TenantID != "" {
		ctx = tenantctx.WithTenant(ctx, payload.TenantID)
	}

	// The run ended: the conversation takes its next run. Recorded before the
	// waiters are woken, which may start that run right away.
	activeRun := s.runTracker != nil && payload.TurnID != "" &&
		s.runTracker.IsActiveConversationRun(payload.ConversationID, payload.TurnID)
	if s.runTracker != nil {
		s.runTracker.EndConversationRun(payload.ConversationID, payload.TurnID)
	}
	storedActive, err := s.db.EndConversationTurn(ctx, payload.ConversationID, payload.TurnID)
	logBestEffort(ctx, err, "EndConversationTurn", slog.String("conversation_id", payload.ConversationID))

	// Only the completion of the conversation's active turn - the turn this
	// process dispatched, or the stored one (a restart, another replica) - is
	// processed, once. A turn that already ended (stopped, ended by the
	// stuck-work watchdog or after its start was dead-lettered) was completed
	// then; its late completion would store its messages after the next
	// turn's, announce the next turn as finished and wake its waiter. A
	// completion without turn (a worker that sends none) counts as the active
	// turn's.
	if payload.TurnID != "" && !activeRun && !storedActive {
		slog.Info("completion of a conversation turn that already ended, dropped",
			"conversation_id", payload.ConversationID, "turn_id", payload.TurnID, "status", payload.Status)
		return nil
	}

	slog.Info("conversation run complete received",
		"run_id", payload.RunID,
		"conversation_id", payload.ConversationID,
		"session_id", payload.SessionID,
		"status", payload.Status,
		"steps", payload.StepCount,
		"cost", payload.CostUSD,
	)

	// Store intermediate tool messages (assistant messages with tool_calls + tool results).
	if len(payload.ToolMessages) > 0 {
		toolMsgs := make([]conversation.Message, 0, len(payload.ToolMessages))
		for _, tm := range payload.ToolMessages {
			msg := conversation.Message{
				ConversationID: payload.ConversationID,
				Role:           tm.Role,
				Content:        tm.Content,
				ToolCallID:     tm.ToolCallID,
				ToolName:       tm.Name,
			}
			// Serialize tool_calls for assistant messages.
			if len(tm.ToolCalls) > 0 {
				tcJSON, err := json.Marshal(tm.ToolCalls)
				if err == nil {
					msg.ToolCalls = tcJSON
				}
			}
			toolMsgs = append(toolMsgs, msg)
		}
		if err := s.db.CreateToolMessages(ctx, payload.ConversationID, toolMsgs); err != nil {
			slog.Error("failed to store tool messages", "conversation_id", payload.ConversationID, "error", err)
		}
	}

	// Store final assistant message.
	if payload.AssistantContent != "" || payload.Status == "completed" {
		assistantMsg := &conversation.Message{
			ConversationID: payload.ConversationID,
			Role:           "assistant",
			Content:        payload.AssistantContent,
			TokensIn:       payload.TokensIn,
			TokensOut:      payload.TokensOut,
			Model:          payload.Model,
		}
		if _, err := s.db.CreateMessage(ctx, assistantMsg); err != nil {
			slog.Error("failed to store assistant message", "conversation_id", payload.ConversationID, "error", err)
		}
	}

	// Determine WS status.
	wsStatus := "completed"
	if payload.Status != "completed" {
		wsStatus = "failed"
	}

	if s.metrics != nil {
		metricAttrs := []string{"type", "conversation_agentic", "status", wsStatus}
		if wsStatus == "completed" {
			s.metrics.RecordRunCompleted(ctx, metricAttrs...)
		} else {
			s.metrics.RecordRunFailed(ctx, metricAttrs...)
		}
		if payload.CostUSD > 0 {
			s.metrics.RecordRunCost(ctx, payload.CostUSD, metricAttrs...)
		}
	}

	s.hub.BroadcastEvent(ctx, event.AGUIRunFinished, event.AGUIRunFinishedEvent{
		RunID:     payload.RunID,
		Status:    wsStatus,
		Error:     payload.Error,
		Model:     payload.Model,
		CostUSD:   payload.CostUSD,
		TokensIn:  payload.TokensIn,
		TokensOut: payload.TokensOut,
		Steps:     payload.StepCount,
	})

	// Notify in-process waiters (e.g. autoagent).
	s.notifyCompletionWaiter(payload.ConversationID, CompletionResult{Status: payload.Status, Error: payload.Error, CostUSD: payload.CostUSD})

	// Record prompt scores for evolution tracking.
	if s.scoreCollector != nil && payload.Model != "" {
		tenantID := tenantctx.FromContext(ctx)
		fingerprint := ""
		if s.promptAssembler != nil {
			conv, convErr := s.db.GetConversation(ctx, payload.ConversationID)
			if convErr == nil && conv.Mode != "" {
				fingerprint = s.promptAssembler.FingerprintForMode(conv.Mode)
			}
		}
		if fingerprint != "" {
			modelFamily := ExtractModelFamily(payload.Model)
			succeeded := payload.Status == "completed"
			if err := s.scoreCollector.RecordSuccessScore(ctx, tenantID, fingerprint,
				"", modelFamily, payload.RunID, succeeded); err != nil {
				logBestEffort(ctx, err, "record success score")
			}
			if payload.CostUSD > 0 && payload.TokensOut > 0 {
				qualityPerDollar := float64(payload.TokensOut) / payload.CostUSD
				if err := s.scoreCollector.RecordCostScore(ctx, tenantID, fingerprint,
					"", modelFamily, payload.RunID, qualityPerDollar); err != nil {
					logBestEffort(ctx, err, "record cost score")
				}
			}
		}
	}

	return nil
}

// CompletionWaiter receives the end of a conversation's next run. Register
// it with ExpectCompletion before the run is dispatched, so that a run that
// ends before the caller waits is not missed (KI-76); Close releases it.
type CompletionWaiter struct {
	svc            *ConversationService
	conversationID string
	ch             chan CompletionResult
	closeOnce      sync.Once
}

// ExpectCompletion registers a waiter for the end of the conversation's next
// run. A conversation has one waiter at a time.
func (s *ConversationService) ExpectCompletion(conversationID string) (*CompletionWaiter, error) {
	w := &CompletionWaiter{svc: s, conversationID: conversationID, ch: make(chan CompletionResult, 1)}
	s.completionWaitersMu.Lock()
	defer s.completionWaitersMu.Unlock()
	if _, exists := s.completionWaiters[conversationID]; exists {
		return nil, fmt.Errorf("a waiter already exists for conversation %s", conversationID)
	}
	s.completionWaiters[conversationID] = w.ch
	return w, nil
}

// Wait blocks until the run ended or ctx is done.
func (w *CompletionWaiter) Wait(ctx context.Context) (CompletionResult, error) {
	select {
	case result := <-w.ch:
		return result, nil
	case <-ctx.Done():
		return CompletionResult{}, ctx.Err()
	}
}

// Close releases the waiter; safe to call more than once.
func (w *CompletionWaiter) Close() {
	w.closeOnce.Do(func() {
		w.svc.completionWaitersMu.Lock()
		defer w.svc.completionWaitersMu.Unlock()
		if w.svc.completionWaiters[w.conversationID] == w.ch {
			delete(w.svc.completionWaiters, w.conversationID)
		}
	})
}

// notifyCompletionWaiter hands result to the conversation's waiter. A waiter
// takes one result; a further one is dropped instead of blocking under the
// lock.
func (s *ConversationService) notifyCompletionWaiter(conversationID string, result CompletionResult) {
	s.completionWaitersMu.Lock()
	defer s.completionWaitersMu.Unlock()
	if ch, ok := s.completionWaiters[conversationID]; ok {
		select {
		case ch <- result:
		default:
		}
	}
}

// WaitForCompletion blocks until the conversation run finishes or the context
// is cancelled. It misses a run that ended before it was called: to dispatch
// and wait, register with ExpectCompletion first.
func (s *ConversationService) WaitForCompletion(ctx context.Context, conversationID string) (CompletionResult, error) {
	w, err := s.ExpectCompletion(conversationID)
	if err != nil {
		return CompletionResult{}, err
	}
	defer w.Close()
	return w.Wait(ctx)
}

// StopConversation cancels an active agentic run by publishing a cancel
// message to NATS. The conversation must be one of the caller's tenant
// (the store is tenant-scoped): the cancel reaches every worker by ID.
func (s *ConversationService) StopConversation(ctx context.Context, conversationID string) error {
	if s.queue == nil {
		return errors.New("stop requires NATS queue")
	}
	if _, err := s.db.GetConversation(ctx, conversationID); err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}

	if err := s.publishConversationCancel(ctx, conversationID); err != nil {
		return err
	}
	// The run ends now: its remaining tool calls are rejected and the
	// conversation takes its next message. Its own completion no longer
	// reaches a waiter (it is not the active run), so the stop ends the wait.
	if s.runTracker != nil {
		s.runTracker.MarkConversationRunCancelled(conversationID)
	}
	s.notifyCompletionWaiter(conversationID, CompletionResult{Status: "cancelled", Error: "stopped"})
	_, err := s.db.EndConversationTurn(ctx, conversationID, "")
	logBestEffort(ctx, err, "EndConversationTurn", slog.String("conversation_id", conversationID))

	s.hub.BroadcastEvent(ctx, event.AGUIRunFinished, event.AGUIRunFinishedEvent{
		RunID:  conversationID,
		Status: "cancelled",
	})

	slog.Info("conversation run cancel requested", "conversation_id", conversationID)
	return nil
}

// publishConversationCancel tells the workers to stop the conversation's run.
func (s *ConversationService) publishConversationCancel(ctx context.Context, conversationID string) error {
	data, err := json.Marshal(struct {
		RunID string `json:"run_id"`
	}{RunID: conversationID})
	if err != nil {
		return fmt.Errorf("marshal cancel payload: %w", err)
	}
	if err := s.queue.Publish(ctx, messagequeue.SubjectConversationRunCancel, data); err != nil {
		return fmt.Errorf("publish conversation run cancel: %w", err)
	}
	return nil
}

// StartCompletionSubscriber subscribes to conversation.run.complete on NATS.
// Returns a cancel function to stop the subscription.
func (s *ConversationService) StartCompletionSubscriber(ctx context.Context) (func(), error) {
	if s.queue == nil {
		return func() {}, nil
	}
	return s.queue.Subscribe(ctx, messagequeue.SubjectConversationRunComplete, s.HandleConversationRunComplete)
}

// historyToPayload converts domain messages to protocol payload messages.
// Delegates to the shared HistoryToPayload function.
func (s *ConversationService) historyToPayload(messages []conversation.Message) []messagequeue.ConversationMessagePayload {
	return HistoryToPayload(messages)
}

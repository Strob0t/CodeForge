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
	// An explicit request is honoured: an agentic turn on a project without
	// a workspace is refused by the dispatch (HTTP 400, KI-193) rather than
	// silently answered as plain chat.
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
	return s.completeConversationRun(ctx, &payload, true)
}

// completeConversationRun processes the completion of a conversation turn:
// the worker's (fromWorker), or one the Go Core reports itself for a turn it
// ended (the stuck-work watchdog, a dead-lettered start). The worker's
// completion of a turn is kept once (S2-G fix 2, 2): it is delivered at
// least once, and a redelivery must not store the turn's messages and cost
// again. The Go Core's own completion stores nothing and claims nothing, so
// the worker's late completion of that turn is still kept.
func (s *ConversationService) completeConversationRun(ctx context.Context, payload *messagequeue.ConversationRunCompletePayload, fromWorker bool) error {
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
	// then; its late completion keeps only the turn's work (see
	// keepEndedTurnCompletion). A completion without turn (a worker that
	// sends none) counts as the active turn's.
	if payload.TurnID != "" && !activeRun && !storedActive {
		if first, err := s.firstTurnCompletion(ctx, payload, fromWorker); err != nil || !first {
			return err
		}
		s.keepEndedTurnCompletion(ctx, payload)
		return nil
	}
	if first, err := s.firstTurnCompletion(ctx, payload, fromWorker); err != nil || !first {
		return err
	}

	slog.Info("conversation run complete received",
		"run_id", payload.RunID,
		"conversation_id", payload.ConversationID,
		"session_id", payload.SessionID,
		"status", payload.Status,
		"steps", payload.StepCount,
		"cost", payload.CostUSD,
	)

	s.storeCompletionMessages(ctx, payload)

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
	s.notifyCompletionWaiter(ctx, payload.ConversationID, CompletionResult{Status: payload.Status, Error: payload.Error, CostUSD: payload.CostUSD})

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

// firstTurnCompletion claims the worker's completion of its turn and
// reports whether it is the first (see completeConversationRun). A
// completion without turn (an older worker) and the Go Core's own
// completion are not claimed; a claim that fails is returned, so the
// completion is retried.
func (s *ConversationService) firstTurnCompletion(ctx context.Context, payload *messagequeue.ConversationRunCompletePayload, fromWorker bool) (bool, error) {
	if !fromWorker || payload.TurnID == "" {
		return true, nil
	}
	first, err := s.db.ClaimConversationTurnCompletion(ctx, payload.ConversationID, payload.TurnID)
	if err != nil {
		return false, fmt.Errorf("claim conversation turn completion: %w", err)
	}
	if !first {
		slog.InfoContext(ctx, "repeated completion of a conversation turn, ignored",
			"conversation_id", payload.ConversationID, "turn_id", payload.TurnID)
	}
	return first, nil
}

// storeCompletionMessages stores what a turn produced: its intermediate tool
// messages (assistant messages with tool calls, tool results) and its final
// or partial assistant answer.
func (s *ConversationService) storeCompletionMessages(ctx context.Context, payload *messagequeue.ConversationRunCompletePayload) {
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
}

// keepEndedTurnCompletion handles the completion of a turn that already
// ended (S2-G fix, 5): stopped, ended by the stuck-work watchdog or after
// its start was dead-lettered. The end was announced then and the
// conversation released, so nothing is broadcast, no waiter is woken and no
// turn becomes active. While no newer turn has started, the turn's messages
// (the work it did before it ended) and its cost are kept; once a newer
// turn is active, only its cost is: its messages would land after the
// newer turn's.
func (s *ConversationService) keepEndedTurnCompletion(ctx context.Context, payload *messagequeue.ConversationRunCompletePayload) {
	newer := s.newerTurnActive(ctx, payload.ConversationID)
	slog.Info("completion of a conversation turn that already ended",
		"conversation_id", payload.ConversationID, "turn_id", payload.TurnID, "status", payload.Status,
		"messages_kept", !newer, "cost", payload.CostUSD)
	if !newer {
		s.storeCompletionMessages(ctx, payload)
	}
	if s.metrics != nil && payload.CostUSD > 0 {
		s.metrics.RecordRunCost(ctx, payload.CostUSD, "type", "conversation_agentic", "status", payload.Status)
	}
}

// newerTurnActive reports whether a turn of the conversation is active,
// dispatched by this process or stored; it is called for a turn that is
// not, so an active turn is a newer one. A conversation that cannot be read
// counts as having one: messages are dropped rather than stored out of
// order.
func (s *ConversationService) newerTurnActive(ctx context.Context, conversationID string) bool {
	if s.runTracker != nil && s.runTracker.ActiveConversationRun(conversationID) != "" {
		return true
	}
	conv, err := s.db.GetConversation(ctx, conversationID)
	if err != nil {
		logBestEffort(ctx, err, "GetConversation", slog.String("conversation_id", conversationID))
		return true
	}
	return conv.ActiveTurnID != ""
}

// CompletionWaiter receives the end of a conversation's next run. Register
// it with ExpectCompletion before the run is dispatched, so that a run that
// ends before the caller waits is not missed (KI-76); Close releases it.
type CompletionWaiter struct {
	svc            *ConversationService
	conversationID string
	ch             chan CompletionResult
	stopRelay      func()
	closeOnce      sync.Once
}

// ExpectCompletion registers a waiter for the end of the conversation's next
// run. A conversation has one waiter at a time. The run's completion may be
// taken by another Go Core replica, which relays it here (KI-86).
func (s *ConversationService) ExpectCompletion(conversationID string) (*CompletionWaiter, error) {
	w := &CompletionWaiter{svc: s, conversationID: conversationID, ch: make(chan CompletionResult, 1), stopRelay: func() {}}
	s.completionWaitersMu.Lock()
	if _, exists := s.completionWaiters[conversationID]; exists {
		s.completionWaitersMu.Unlock()
		return nil, fmt.Errorf("a waiter already exists for conversation %s", conversationID)
	}
	s.completionWaiters[conversationID] = w.ch
	s.completionWaitersMu.Unlock()

	if relay := messagequeue.RelayOf(s.queue); relay != nil {
		stop, err := relay.Serve(completionRelayKey(conversationID), func(data []byte) []byte {
			var result CompletionResult
			if err := json.Unmarshal(data, &result); err != nil || !s.notifyLocalCompletion(conversationID, result) {
				return nil
			}
			return relayTaken
		})
		if err != nil {
			slog.Warn("waiting for the conversation run on this replica only", "conversation_id", conversationID, "error", err)
		} else {
			w.stopRelay = stop
		}
	}
	return w, nil
}

// completionRelayKey is the relay key of a conversation's completion waiter.
func completionRelayKey(conversationID string) string {
	return "conversation-completion:" + conversationID
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
		w.stopRelay()
		w.svc.completionWaitersMu.Lock()
		defer w.svc.completionWaitersMu.Unlock()
		if w.svc.completionWaiters[w.conversationID] == w.ch {
			delete(w.svc.completionWaiters, w.conversationID)
		}
	})
}

// notifyCompletionWaiter hands result to the conversation's waiter: on this
// replica, else through the relay to the replica that waits (KI-86).
func (s *ConversationService) notifyCompletionWaiter(ctx context.Context, conversationID string, result CompletionResult) {
	if s.notifyLocalCompletion(conversationID, result) {
		return
	}
	relay := messagequeue.RelayOf(s.queue)
	if relay == nil {
		return
	}
	data, err := json.Marshal(result)
	if err != nil {
		slog.Error("relay conversation completion", "conversation_id", conversationID, "error", err)
		return
	}
	if _, err := relay.Request(ctx, completionRelayKey(conversationID), data); err != nil {
		slog.Warn("relay conversation completion failed, its waiter times out", "conversation_id", conversationID, "error", err)
	}
}

// notifyLocalCompletion hands result to the conversation's waiter on this
// replica and reports whether there is one. A waiter takes one result; a
// further one is dropped instead of blocking under the lock.
func (s *ConversationService) notifyLocalCompletion(conversationID string, result CompletionResult) bool {
	s.completionWaitersMu.Lock()
	defer s.completionWaitersMu.Unlock()
	ch, ok := s.completionWaiters[conversationID]
	if ok {
		select {
		case ch <- result:
		default:
		}
	}
	return ok
}

// StopConversation cancels an active agentic run by publishing a cancel
// message to NATS. The conversation must be one of the caller's tenant
// (the store is tenant-scoped): the cancel reaches every worker by ID.
func (s *ConversationService) StopConversation(ctx context.Context, conversationID string) error {
	if s.queue == nil {
		return errors.New("stop requires NATS queue")
	}
	conv, err := s.db.GetConversation(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}

	if err := s.publishConversationCancel(ctx, conversationID); err != nil {
		return err
	}
	// The run ends now: its remaining tool calls are rejected and the
	// conversation takes its next message. Its own completion no longer
	// reaches a waiter (it is not the active run), so the stop ends the wait.
	// Only the stopped run's stored turn ends: a run that begins meanwhile
	// keeps its own. Without a run dispatched here (a restart), the stored
	// turn read above is the stopped one.
	stopped := conv.ActiveTurnID
	if s.runTracker != nil {
		if turn := s.runTracker.MarkConversationRunCancelled(conversationID); turn != "" {
			stopped = turn
		}
	}
	s.notifyCompletionWaiter(ctx, conversationID, CompletionResult{Status: "cancelled", Error: "stopped"})
	if stopped != "" {
		_, endErr := s.db.EndConversationTurn(ctx, conversationID, stopped)
		logBestEffort(ctx, endErr, "EndConversationTurn", slog.String("conversation_id", conversationID))
	}

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

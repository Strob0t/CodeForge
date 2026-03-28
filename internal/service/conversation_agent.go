package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

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
	s.completionWaitersMu.Lock()
	if ch, ok := s.completionWaiters[payload.ConversationID]; ok {
		ch <- CompletionResult{
			Status:  payload.Status,
			Error:   payload.Error,
			CostUSD: payload.CostUSD,
		}
	}
	s.completionWaitersMu.Unlock()

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

// WaitForCompletion blocks until the conversation run finishes or the context is cancelled.
func (s *ConversationService) WaitForCompletion(ctx context.Context, conversationID string) (CompletionResult, error) {
	ch := make(chan CompletionResult, 1)

	s.completionWaitersMu.Lock()
	if _, exists := s.completionWaiters[conversationID]; exists {
		s.completionWaitersMu.Unlock()
		return CompletionResult{}, fmt.Errorf("a waiter already exists for conversation %s", conversationID)
	}
	s.completionWaiters[conversationID] = ch
	s.completionWaitersMu.Unlock()

	defer func() {
		s.completionWaitersMu.Lock()
		delete(s.completionWaiters, conversationID)
		s.completionWaitersMu.Unlock()
	}()

	select {
	case result := <-ch:
		return result, nil
	case <-ctx.Done():
		return CompletionResult{}, ctx.Err()
	}
}

// StopConversation cancels an active agentic run by publishing a cancel message to NATS.
func (s *ConversationService) StopConversation(ctx context.Context, conversationID string) error {
	if s.queue == nil {
		return errors.New("stop requires NATS queue")
	}

	payload := struct {
		RunID string `json:"run_id"`
	}{
		RunID: conversationID,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal cancel payload: %w", err)
	}

	if err := s.queue.Publish(ctx, messagequeue.SubjectConversationRunCancel, data); err != nil {
		return fmt.Errorf("publish conversation run cancel: %w", err)
	}

	s.hub.BroadcastEvent(ctx, event.AGUIRunFinished, event.AGUIRunFinishedEvent{
		RunID:  conversationID,
		Status: "cancelled",
	})

	slog.Info("conversation run cancel requested", "conversation_id", conversationID)
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

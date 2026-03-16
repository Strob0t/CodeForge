package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

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

func (s *ConversationService) IsAgentic(ctx context.Context, conversationID string, req conversation.SendMessageRequest) bool {
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

// buildConversationContextEntries assembles context entries for a conversation run
// when ContextEnabled is true and the context optimizer is wired. The history
// parameter drives the adaptive budget: early turns get the full budget, long
// conversations get progressively less (or zero) injected context.
func (s *ConversationService) buildConversationContextEntries(
	ctx context.Context,
	projectID, userMessage, conversationID string,
	history []messagequeue.ConversationMessagePayload,
) []messagequeue.ContextEntryPayload {
	if s.contextOpt == nil || s.agentCfg == nil || !s.agentCfg.ContextEnabled {
		return nil
	}

	budget := AdaptiveContextBudget(s.agentCfg.ContextBudget, history)
	if budget <= 0 {
		slog.Debug("adaptive context budget is zero, skipping context injection",
			"conversation_id", conversationID,
			"history_messages", len(history),
		)
		return nil
	}

	entries, err := s.contextOpt.BuildConversationContext(ctx, projectID, userMessage, "",
		ConversationContextOpts{
			Budget:        budget,
			PromptReserve: s.agentCfg.ContextPromptReserve,
		})
	if err != nil {
		slog.Warn("conversation context build failed",
			"conversation_id", conversationID,
			"project_id", projectID,
			"error", err,
		)
		return nil
	}
	if len(entries) == 0 {
		return nil
	}

	slog.Info("conversation context entries built",
		"conversation_id", conversationID,
		"entries", len(entries),
		"budget", budget,
		"history_messages", len(history),
	)
	return toContextEntryPayloads(entries)
}

// policyForAutonomy maps an autonomy level (1-5) to a policy preset name.
func policyForAutonomy(autonomy int) string {
	switch autonomy {
	case 1:
		return "supervised-ask-all"
	case 4, 5:
		return "trusted-mount-autonomous"
	default:
		return "headless-safe-sandbox"
	}
}

// SendMessageAgentic stores the user message and dispatches an agentic run to the
// Python worker via NATS. Streaming results arrive asynchronously via WebSocket.
// The method returns immediately after dispatch.
func (s *ConversationService) SendMessageAgentic(ctx context.Context, conversationID string, req conversation.SendMessageRequest) error {
	if req.Content == "" {
		return errors.New("content is required")
	}
	if s.queue == nil {
		return errors.New("agentic mode requires NATS queue")
	}

	conv, err := s.db.GetConversation(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}

	// Store user message.
	userMsg := &conversation.Message{
		ConversationID: conversationID,
		Role:           "user",
		Content:        req.Content,
	}
	if _, err = s.db.CreateMessage(ctx, userMsg); err != nil {
		return fmt.Errorf("store user message: %w", err)
	}

	// Load full conversation history (including tool_calls and tool results).
	history, err := s.db.ListMessages(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("list messages: %w", err)
	}

	// Fetch project for workspace path, policy profile, etc.
	proj, err := s.db.GetProject(ctx, conv.ProjectID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	// Ensure a session exists for this conversation.
	var sessionID string
	var sessionMeta *messagequeue.SessionMetaPayload
	if s.sessionSvc != nil {
		sess, sessErr := s.sessionSvc.EnsureConversationSession(ctx, proj.ID, conversationID)
		if sessErr != nil {
			slog.Warn("failed to ensure conversation session", "conversation_id", conversationID, "error", sessErr)
		} else {
			sessionID = sess.ID
			sessionMeta = buildSessionMeta(sess)
		}
	}

	// Build system prompt.
	systemPrompt := s.buildSystemPrompt(ctx, conv.ProjectID)

	// Convert history to protocol messages.
	protoMessages := s.historyToPayload(history)

	// Resolve model.
	model := s.resolveModel()
	if model == "" {
		return fmt.Errorf("no LLM model configured — set conversation_model in litellm config or default_model in agent config")
	}

	// Resolve mode for scenario-based LLM routing.
	var modeAutonomy int
	var resolvedMode *messagequeue.ModePayload
	if s.modeSvc != nil {
		modeID := req.Mode
		if modeID == "" {
			modeID = "coder" // Conversations default to coder mode.
		}
		if m, mErr := s.modeSvc.Get(modeID); mErr == nil {
			modeAutonomy = m.Autonomy
			resolvedMode = &messagequeue.ModePayload{
				ID:               m.ID,
				LLMScenario:      m.LLMScenario,
				Tools:            m.Tools,
				DeniedTools:      m.DeniedTools,
				ModelAdaptations: m.ModelAdaptations,
			}
		}
	}

	// Resolve policy profile using the mode's autonomy level.
	policyProfile := ""
	if s.policySvc != nil {
		modePolicy := ""
		if modeAutonomy > 0 {
			modePolicy = policyForAutonomy(modeAutonomy)
		}
		policyProfile = s.policySvc.ResolveProfile(modePolicy, proj.PolicyProfile)
	}

	// Inject model-family prompt adaptation from mode config.
	systemPrompt = appendModelAdaptation(systemPrompt, model, resolvedMode)

	// Termination config.
	termination := messagequeue.TerminationPayload{
		MaxSteps:       50,
		TimeoutSeconds: 600,
	}
	if s.agentCfg != nil && s.agentCfg.MaxLoopIterations > 0 {
		termination.MaxSteps = s.agentCfg.MaxLoopIterations
	}

	// MCP servers.
	var mcpDefs []messagequeue.MCPServerDefPayload
	if s.mcpSvc != nil {
		servers := s.mcpSvc.ResolveForRun(proj.ID, "")
		for i := range servers {
			mcpDefs = append(mcpDefs, messagequeue.MCPServerDefPayload{
				ID:        servers[i].ID,
				Name:      servers[i].Name,
				Transport: string(servers[i].Transport),
				Command:   servers[i].Command,
				Args:      servers[i].Args,
				URL:       servers[i].URL,
				Env:       servers[i].Env,
			})
		}
	}

	// RunID matches conversationID for tool-call policy lookups.
	// A separate unique dedup key prevents NATS from dropping follow-up messages.
	runID := conversationID
	dedupKey := "conv-start-" + uuid.New().String()

	// Match microagents against the user message (Phase 22C).
	var microagentPrompts []string
	if s.microagentSvc != nil {
		matched, maErr := s.microagentSvc.Match(ctx, proj.ID, req.Content)
		if maErr != nil {
			slog.Warn("microagent match failed", "conversation_id", conversationID, "error", maErr)
		} else if len(matched) > 0 {
			for i := range matched {
				microagentPrompts = append(microagentPrompts, matched[i].Prompt)
			}
			slog.Info("microagents matched for conversation", "conversation_id", conversationID, "count", len(matched))
		}
	}

	// Build context entries for the conversation run.
	contextEntries := s.buildConversationContextEntries(ctx, proj.ID, req.Content, conversationID, protoMessages)

	// Resolve per-user provider API key (if configured).
	providerAPIKey := s.resolveProviderAPIKey(ctx, req.UserID, model)

	payload := messagequeue.ConversationRunStartPayload{
		RunID:             runID,
		ConversationID:    conversationID,
		SessionID:         sessionID,
		ProjectID:         proj.ID,
		Messages:          protoMessages,
		SystemPrompt:      systemPrompt,
		Model:             model,
		PolicyProfile:     policyProfile,
		WorkspacePath:     proj.WorkspacePath,
		Mode:              resolvedMode,
		Termination:       termination,
		MCPServers:        mcpDefs,
		MicroagentPrompts: microagentPrompts,
		RoutingEnabled:    s.routingCfg != nil && s.routingCfg.Enabled,
		Context:           contextEntries,
		Agentic:           true,
		ProviderAPIKey:    providerAPIKey,
		TenantID:          tenantctx.FromContext(ctx),
		SessionMeta:       sessionMeta,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal conversation run start: %w", err)
	}

	// Broadcast run started via WebSocket.
	s.hub.BroadcastEvent(ctx, ws.AGUIRunStarted, ws.AGUIRunStartedEvent{
		RunID:     runID,
		ThreadID:  conversationID,
		AgentName: "agent",
	})

	// Publish to NATS for the Python worker (with dedup to prevent duplicate runs).
	if err := s.queue.PublishWithDedup(ctx, messagequeue.SubjectConversationRunStart, data, dedupKey); err != nil {
		s.hub.BroadcastEvent(ctx, ws.AGUIRunFinished, ws.AGUIRunFinishedEvent{
			RunID:  runID,
			Status: "failed",
			Error:  err.Error(),
		})
		return fmt.Errorf("publish conversation run start: %w", err)
	}

	if s.metrics != nil {
		s.metrics.RunsStarted.Add(ctx, 1, metric.WithAttributes(
			attribute.String("type", "conversation_agentic"),
			attribute.String("project.id", proj.ID),
		))
	}

	slog.Info("conversation agentic run dispatched",
		"run_id", runID,
		"conversation_id", conversationID,
		"session_id", sessionID,
		"project_id", proj.ID,
		"model", model,
	)

	return nil
}

// AgenticOption configures optional behaviour for SendMessageAgenticWithMode.
type AgenticOption func(*agenticOpts)

type agenticOpts struct {
	extraContext []messagequeue.ContextEntryPayload
}

// WithContextEntries appends additional context entries to the NATS payload.
// These are merged with the automatically built conversation context entries.
func WithContextEntries(entries []messagequeue.ContextEntryPayload) AgenticOption {
	return func(o *agenticOpts) {
		o.extraContext = append(o.extraContext, entries...)
	}
}

// SendMessageAgenticWithMode is like SendMessageAgentic but accepts a mode ID override
// instead of defaulting to "coder". Used for specialized agent flows like goal discovery.
// Optional AgenticOption values can inject extra context entries into the NATS payload.
func (s *ConversationService) SendMessageAgenticWithMode(ctx context.Context, conversationID, content, modeID string, opts ...AgenticOption) error {
	if content == "" {
		return errors.New("content is required")
	}
	if s.queue == nil {
		return errors.New("agentic mode requires NATS queue")
	}

	conv, err := s.db.GetConversation(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}

	// Store user message.
	userMsg := &conversation.Message{
		ConversationID: conversationID,
		Role:           "user",
		Content:        content,
	}
	if _, err := s.db.CreateMessage(ctx, userMsg); err != nil {
		return fmt.Errorf("store user message: %w", err)
	}

	// Load history.
	history, err := s.db.ListMessages(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("list messages: %w", err)
	}

	proj, err := s.db.GetProject(ctx, conv.ProjectID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	// Ensure a session exists for this conversation.
	var sessionID string
	var sessionMeta *messagequeue.SessionMetaPayload
	if s.sessionSvc != nil {
		sess, sessErr := s.sessionSvc.EnsureConversationSession(ctx, proj.ID, conversationID)
		if sessErr != nil {
			slog.Warn("failed to ensure conversation session", "conversation_id", conversationID, "error", sessErr)
		} else {
			sessionID = sess.ID
			sessionMeta = buildSessionMeta(sess)
		}
	}

	systemPrompt := s.buildSystemPrompt(ctx, conv.ProjectID)
	protoMessages := s.historyToPayload(history)
	model := s.resolveModel()
	if model == "" {
		return fmt.Errorf("no LLM model configured")
	}

	// Resolve the requested mode.
	var modeAutonomy int
	var resolvedMode *messagequeue.ModePayload
	if s.modeSvc != nil {
		if m, mErr := s.modeSvc.Get(modeID); mErr == nil {
			modeAutonomy = m.Autonomy
			resolvedMode = &messagequeue.ModePayload{
				ID:               m.ID,
				LLMScenario:      m.LLMScenario,
				Tools:            m.Tools,
				DeniedTools:      m.DeniedTools,
				ModelAdaptations: m.ModelAdaptations,
			}
		}
	}

	// Resolve policy profile using the mode's autonomy level.
	policyProfile := ""
	if s.policySvc != nil {
		modePolicy := ""
		if modeAutonomy > 0 {
			modePolicy = policyForAutonomy(modeAutonomy)
		}
		policyProfile = s.policySvc.ResolveProfile(modePolicy, proj.PolicyProfile)
	}

	// Inject model-family prompt adaptation from mode config.
	systemPrompt = appendModelAdaptation(systemPrompt, model, resolvedMode)

	termination := messagequeue.TerminationPayload{
		MaxSteps:       50,
		TimeoutSeconds: 600,
	}
	if s.agentCfg != nil && s.agentCfg.MaxLoopIterations > 0 {
		termination.MaxSteps = s.agentCfg.MaxLoopIterations
	}

	// MCP servers.
	var mcpDefs []messagequeue.MCPServerDefPayload
	if s.mcpSvc != nil {
		servers := s.mcpSvc.ResolveForRun(proj.ID, "")
		for i := range servers {
			mcpDefs = append(mcpDefs, messagequeue.MCPServerDefPayload{
				ID:        servers[i].ID,
				Name:      servers[i].Name,
				Transport: string(servers[i].Transport),
				Command:   servers[i].Command,
				Args:      servers[i].Args,
				URL:       servers[i].URL,
				Env:       servers[i].Env,
			})
		}
	}

	// Match microagents against the user message.
	var microagentPrompts []string
	if s.microagentSvc != nil {
		matched, maErr := s.microagentSvc.Match(ctx, proj.ID, content)
		if maErr != nil {
			slog.Warn("microagent match failed", "conversation_id", conversationID, "error", maErr)
		} else if len(matched) > 0 {
			for i := range matched {
				microagentPrompts = append(microagentPrompts, matched[i].Prompt)
			}
			slog.Info("microagents matched for conversation", "conversation_id", conversationID, "count", len(matched))
		}
	}

	// Build context entries for the conversation run.
	contextEntries := s.buildConversationContextEntries(ctx, proj.ID, content, conversationID, protoMessages)

	// Apply functional options (e.g. extra context entries from the caller).
	var applied agenticOpts
	for _, o := range opts {
		o(&applied)
	}
	if len(applied.extraContext) > 0 {
		contextEntries = append(contextEntries, applied.extraContext...)
	}

	runID := conversationID
	payload := messagequeue.ConversationRunStartPayload{
		RunID:             runID,
		ConversationID:    conversationID,
		SessionID:         sessionID,
		ProjectID:         proj.ID,
		Messages:          protoMessages,
		SystemPrompt:      systemPrompt,
		Model:             model,
		PolicyProfile:     policyProfile,
		WorkspacePath:     proj.WorkspacePath,
		Mode:              resolvedMode,
		Termination:       termination,
		MCPServers:        mcpDefs,
		MicroagentPrompts: microagentPrompts,
		RoutingEnabled:    s.routingCfg != nil && s.routingCfg.Enabled,
		Context:           contextEntries,
		Agentic:           true,
		TenantID:          tenantctx.FromContext(ctx),
		SessionMeta:       sessionMeta,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal conversation run start: %w", err)
	}

	s.hub.BroadcastEvent(ctx, ws.AGUIRunStarted, ws.AGUIRunStartedEvent{
		RunID:     runID,
		ThreadID:  conversationID,
		AgentName: modeID,
	})

	if err := s.queue.Publish(ctx, messagequeue.SubjectConversationRunStart, data); err != nil {
		return fmt.Errorf("publish conversation run start: %w", err)
	}

	slog.Info("conversation agentic run dispatched (mode override)",
		"run_id", runID,
		"conversation_id", conversationID,
		"session_id", sessionID,
		"project_id", proj.ID,
		"mode", modeID,
		"model", model,
	)

	return nil
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
		attrs := metric.WithAttributes(
			attribute.String("type", "conversation_agentic"),
			attribute.String("status", wsStatus),
		)
		if wsStatus == "completed" {
			s.metrics.RunsCompleted.Add(ctx, 1, attrs)
		} else {
			s.metrics.RunsFailed.Add(ctx, 1, attrs)
		}
		if payload.CostUSD > 0 {
			s.metrics.RunCost.Record(ctx, payload.CostUSD, attrs)
		}
	}

	s.hub.BroadcastEvent(ctx, ws.AGUIRunFinished, ws.AGUIRunFinishedEvent{
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

	s.hub.BroadcastEvent(ctx, ws.AGUIRunFinished, ws.AGUIRunFinishedEvent{
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
func (s *ConversationService) historyToPayload(messages []conversation.Message) []messagequeue.ConversationMessagePayload {
	result := make([]messagequeue.ConversationMessagePayload, 0, len(messages))
	for i := range messages {
		if messages[i].Role == "system" {
			continue
		}
		pm := messagequeue.ConversationMessagePayload{
			Role:       messages[i].Role,
			Content:    messages[i].Content,
			ToolCallID: messages[i].ToolCallID,
			Name:       messages[i].ToolName,
		}
		// Parse tool_calls from stored JSON.
		if len(messages[i].ToolCalls) > 0 {
			var tcs []messagequeue.ConversationToolCall
			if err := json.Unmarshal(messages[i].ToolCalls, &tcs); err == nil {
				pm.ToolCalls = tcs
			}
		}
		// Propagate images if present.
		if len(messages[i].Images) > 0 {
			var imgs []messagequeue.MessageImagePayload
			if err := json.Unmarshal(messages[i].Images, &imgs); err == nil {
				pm.Images = imgs
			}
		}
		result = append(result, pm)
	}
	return result
}

// appendModelAdaptation appends a model-family-specific prompt adaptation from the
// mode's ModelAdaptations map. This allows modes to carry per-family instructions
// (e.g. "For OpenAI models, prefer function-calling over raw JSON").
func appendModelAdaptation(systemPrompt, model string, mode *messagequeue.ModePayload) string {
	if mode == nil || len(mode.ModelAdaptations) == 0 || model == "" {
		return systemPrompt
	}
	family := benchmark.ModelFamily(model)
	if adaptation, ok := mode.ModelAdaptations[family]; ok && adaptation != "" {
		return systemPrompt + "\n\n" + adaptation
	}
	return systemPrompt
}

// buildSystemPrompt assembles the system prompt for a conversation using the
// embedded template and project context. Failures in fetching optional context
// (agents, tasks, roadmap) are logged and skipped gracefully.
func (s *ConversationService) buildSystemPrompt(ctx context.Context, projectID string) string {
	data := conversationPromptData{}

	// Fetch project info (required for a meaningful prompt).
	proj, err := s.db.GetProject(ctx, projectID)
	if err != nil {
		slog.Warn("conversation: failed to fetch project for system prompt", "project_id", projectID, "error", err)
		data.ProjectName = projectID
	} else {
		data.ProjectName = proj.Name
		data.ProjectDescription = proj.Description
		data.WorkspacePath = proj.WorkspacePath
		data.Provider = proj.Provider
		data.RepoURL = proj.RepoURL
	}

	// Fetch available agents (optional).
	agents, err := s.db.ListAgents(ctx, projectID)
	if err != nil {
		slog.Debug("conversation: failed to list agents for system prompt", "project_id", projectID, "error", err)
	} else {
		for i := range agents {
			label := agents[i].Name
			if agents[i].Backend != "" {
				label += " (" + agents[i].Backend + ")"
			}
			data.Agents = append(data.Agents, label)
		}
	}

	// Fetch available modes (optional).
	if s.modeSvc != nil {
		modes := s.modeSvc.List()
		for i := range modes {
			data.Modes = append(data.Modes, modes[i].Name)
		}
	}

	// Fetch recent tasks (optional, limit to last 10).
	tasks, err := s.db.ListTasks(ctx, projectID)
	if err != nil {
		slog.Debug("conversation: failed to list tasks for system prompt", "project_id", projectID, "error", err)
	} else {
		limit := min(10, len(tasks))
		// Take the last N tasks (most recent).
		start := len(tasks) - limit
		for i := range tasks[start:] {
			data.RecentTasks = append(data.RecentTasks, conversationTaskSummary{
				ID:     tasks[start+i].ID,
				Name:   tasks[start+i].Title,
				Status: string(tasks[start+i].Status),
			})
		}
	}

	// Fetch roadmap summary with milestones and features (optional).
	rm, err := s.db.GetRoadmapByProject(ctx, projectID)
	if err == nil && rm != nil {
		var sb strings.Builder
		sb.WriteString(rm.Title)
		if rm.Description != "" {
			sb.WriteString(" - ")
			sb.WriteString(rm.Description)
		}
		for i := range rm.Milestones {
			ms := &rm.Milestones[i]
			sb.WriteString("\n  ")
			sb.WriteString(ms.Title)
			sb.WriteString(" [")
			sb.WriteString(string(ms.Status))
			sb.WriteString("]")
			for j := range ms.Features {
				f := &ms.Features[j]
				sb.WriteString("\n    - ")
				sb.WriteString(f.Title)
				sb.WriteString(" (")
				sb.WriteString(string(f.Status))
				sb.WriteString(")")
			}
		}
		data.RoadmapSummary = sb.String()
	}

	// Fetch project goals for system prompt (Goal Discovery).
	if s.goalSvc != nil {
		goals, gErr := s.goalSvc.ListEnabled(ctx, projectID)
		if gErr == nil && len(goals) > 0 {
			data.GoalContext = renderGoalContext(goals)
		}
	}

	// Detect tech stack summary if workspace path is available.
	if data.WorkspacePath != "" {
		stack, stackErr := detectStackSummary(data.WorkspacePath)
		if stackErr == nil && stack != "" {
			data.Stack = stack
		}
	}

	// Add built-in tool descriptions for agentic mode.
	data.BuiltinTools = []builtinToolSummary{
		{Name: "Read", Description: "Read file contents with optional line range"},
		{Name: "Write", Description: "Create or overwrite a file"},
		{Name: "Edit", Description: "Search-and-replace edit within a file"},
		{Name: "Bash", Description: "Execute a shell command"},
		{Name: "Search", Description: "Regex search across files"},
		{Name: "Glob", Description: "Find files by glob pattern"},
		{Name: "ListDir", Description: "List directory contents"},
	}

	var buf bytes.Buffer
	if err := conversationTmpl.Execute(&buf, data); err != nil {
		slog.Error("conversation: failed to render system prompt template", "error", err)
		return fmt.Sprintf("You are CodeForge, an AI coding orchestrator. Project: %s", data.ProjectName)
	}

	return buf.String()
}

// detectStackSummary runs a lightweight stack detection and returns a comma-separated
// summary of detected languages. Returns empty string on any failure.
func detectStackSummary(workspacePath string) (string, error) {
	result, err := project.ScanWorkspace(workspacePath)
	if err != nil {
		return "", err
	}
	if len(result.Languages) == 0 {
		return "", nil
	}
	names := make([]string, len(result.Languages))
	for i, lang := range result.Languages {
		names[i] = lang.Name
	}
	return strings.Join(names, ", "), nil
}

// resolveProviderAPIKey attempts to look up the user's per-provider LLM key.
// Returns "" when no key is found (caller falls back to global key).
func (s *ConversationService) resolveProviderAPIKey(ctx context.Context, userID, model string) string {
	if s.llmKeySvc == nil || userID == "" {
		return ""
	}
	provider := strings.SplitN(model, "/", 2)[0]
	if provider == "" || provider == model {
		return ""
	}
	key, err := s.llmKeySvc.ResolveKeyForProvider(ctx, userID, provider)
	if err != nil {
		slog.Warn("failed to resolve user LLM key", "user_id", userID, "provider", provider, "error", err)
		return ""
	}
	return key
}

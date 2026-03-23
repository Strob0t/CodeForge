package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/prompt"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
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

// evaluateReminders checks runtime conditions and returns matching reminder texts.
func (s *ConversationService) evaluateReminders(
	ctx context.Context,
	conversationID string,
	history []messagequeue.ConversationMessagePayload,
) []string {
	if s.promptAssembler == nil || s.promptAssembler.library == nil {
		return nil
	}

	// Fetch all reminder entries from the library.
	reminders := s.promptAssembler.library.GetByCategory(prompt.CategoryReminder)
	if len(reminders) == 0 {
		return nil
	}

	// Query accumulated cost for this conversation from trajectory events.
	budgetPct, budgetUsed, budgetLimit := s.computeBudget(ctx, conversationID)

	// Build reminder template data from current state.
	data := map[string]any{
		"TurnCount":       len(history),
		"BudgetPercent":   budgetPct,
		"BudgetUsed":      budgetUsed,
		"BudgetLimit":     budgetLimit,
		"StallIterations": countStallIterations(history),
	}

	var result []string
	for i := range reminders {
		text := renderEntry(&reminders[i], data)
		text = strings.TrimSpace(text)
		if text != "" {
			result = append(result, text)
		}
	}

	return result
}

// defaultBudgetLimit is the default cost budget (USD) when no explicit limit is configured.
const defaultBudgetLimit = 5.0

// computeBudget queries the event store for accumulated cost and returns
// (percent, usedStr, limitStr). Returns (0, "", "") when the event store is
// unavailable or the conversation has no cost data yet.
func (s *ConversationService) computeBudget(ctx context.Context, conversationID string) (pct float64, usedStr, limitStr string) {
	if s.events == nil || conversationID == "" {
		return 0, "", ""
	}
	stats, err := s.events.TrajectoryStats(ctx, conversationID)
	if err != nil || stats == nil {
		return 0, "", ""
	}
	costUsed := stats.TotalCostUSD
	if costUsed <= 0 {
		return 0, "", ""
	}
	costLimit := defaultBudgetLimit
	pct = (costUsed / costLimit) * 100
	if pct > 100 {
		pct = 100
	}
	return pct, fmt.Sprintf("$%.4f", costUsed), fmt.Sprintf("$%.2f", costLimit)
}

// progressToolsConv are tools that indicate meaningful work in conversations.
var progressToolsConv = map[string]bool{
	"Edit":  true,
	"Write": true,
	"Bash":  true,
}

// countStallIterations counts consecutive non-progress tool results at the tail
// of the message history. A "progress" tool is Edit, Write, or Bash. Non-tool
// messages (assistant, user) are skipped. The count resets on any progress tool.
func countStallIterations(history []messagequeue.ConversationMessagePayload) int {
	count := 0
	for i := range history {
		if history[i].Role != "tool" {
			continue
		}
		if progressToolsConv[history[i].Name] {
			count = 0
		} else {
			count++
		}
	}
	return count
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

// isFullAutoProject checks if the project's policy profile uses an auto-allow mode
// (ModeAcceptEdits or ModeDelegate), meaning HITL is bypassed and the agent runs autonomously.
func (s *ConversationService) isFullAutoProject(_ context.Context, proj *project.Project) bool {
	if s.policySvc == nil {
		return false
	}
	preset := proj.PolicyProfile
	if preset == "" {
		if p, ok := proj.Config["policy_preset"]; ok {
			preset = p
		}
	}
	if preset == "" {
		return false
	}
	profile, ok := s.policySvc.GetProfile(preset)
	if !ok {
		return false
	}
	return profile.Mode == policy.ModeAcceptEdits || profile.Mode == policy.ModeDelegate
}

// SendMessageAgentic stores the user message and dispatches an agentic run to the
// Python worker via NATS. Streaming results arrive asynchronously via WebSocket.
// The method returns immediately after dispatch.
func (s *ConversationService) SendMessageAgentic(ctx context.Context, conversationID string, req *conversation.SendMessageRequest) error {
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

	// Fetch project early so the full-auto gate can inspect it before storing
	// the user message (SendMessageAgenticWithMode stores its own copy).
	proj, err := s.db.GetProject(ctx, conv.ProjectID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	// Full-auto gate: if no goals or open roadmap features exist, redirect to
	// goal_researcher mode so the agent collaborates with the user on goals first.
	if s.isFullAutoProject(ctx, proj) && s.goalSvc != nil {
		goals, _ := s.goalSvc.ListEnabled(ctx, proj.ID)
		hasOpenGoals := len(goals) > 0

		hasOpenFeatures := false
		if rm, rmErr := s.db.GetRoadmapByProject(ctx, proj.ID); rmErr == nil && rm != nil {
			for i := range rm.Milestones {
				for j := range rm.Milestones[i].Features {
					if rm.Milestones[i].Features[j].Status != roadmap.FeatureDone && rm.Milestones[i].Features[j].Status != roadmap.FeatureCancelled {
						hasOpenFeatures = true
						break
					}
				}
				if hasOpenFeatures {
					break
				}
			}
		}

		if !hasOpenGoals && !hasOpenFeatures {
			slog.Info("full-auto gate: no goals or open features, redirecting to goal_researcher",
				"project_id", proj.ID,
				"conversation_id", conversationID,
			)
			return s.SendMessageAgenticWithMode(ctx, conversationID, req.Content, "goal_researcher", WithModel(req.Model))
		}
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

	// Resolve model: explicit request override takes priority over config cascade.
	model := req.Model
	if model == "" {
		model = s.resolveModel()
	}
	if model == "" {
		return fmt.Errorf("no LLM model configured — set conversation_model in litellm config or default_model in agent config")
	}

	// Resolve mode for scenario-based LLM routing.
	// Priority: explicit request > stored conversation mode > default "coder".
	var modeAutonomy int
	var resolvedMode *messagequeue.ModePayload
	if s.modeSvc != nil {
		modeID := req.Mode
		if modeID == "" {
			modeID = conv.Mode
		}
		if modeID == "" {
			modeID = "coder"
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

	// Evaluate system reminders.
	reminders := s.evaluateReminders(ctx, conversationID, protoMessages)

	// Resolve rollout count (only for autonomy >= 4, capped at 8).
	rolloutCount := 1
	if s.agentCfg != nil && s.agentCfg.ConversationRolloutCount > 1 && modeAutonomy >= 4 {
		rolloutCount = min(s.agentCfg.ConversationRolloutCount, 8)
	}

	payload := messagequeue.ConversationRunStartPayload{
		RunID:              runID,
		ConversationID:     conversationID,
		SessionID:          sessionID,
		ProjectID:          proj.ID,
		Messages:           protoMessages,
		SystemPrompt:       systemPrompt,
		Model:              model,
		PolicyProfile:      policyProfile,
		WorkspacePath:      proj.WorkspacePath,
		Mode:               resolvedMode,
		Termination:        termination,
		MCPServers:         mcpDefs,
		MicroagentPrompts:  microagentPrompts,
		RoutingEnabled:     s.routingCfg != nil && s.routingCfg.Enabled,
		Context:            contextEntries,
		Agentic:            true,
		PlanActEnabled:     modeAutonomy >= 4,
		ProviderAPIKey:     providerAPIKey,
		TenantID:           tenantctx.FromContext(ctx),
		SessionMeta:        sessionMeta,
		Reminders:          reminders,
		RolloutCount:       rolloutCount,
		SummarizeThreshold: s.summarizeThreshold(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal conversation run start: %w", err)
	}

	// Broadcast run started via WebSocket.
	s.hub.BroadcastEvent(ctx, event.AGUIRunStarted, event.AGUIRunStartedEvent{
		RunID:     runID,
		ThreadID:  conversationID,
		AgentName: "agent",
	})

	// Publish to NATS for the Python worker (with dedup to prevent duplicate runs).
	if err := s.queue.PublishWithDedup(ctx, messagequeue.SubjectConversationRunStart, data, dedupKey); err != nil {
		s.hub.BroadcastEvent(ctx, event.AGUIRunFinished, event.AGUIRunFinishedEvent{
			RunID:  runID,
			Status: "failed",
			Error:  err.Error(),
		})
		return fmt.Errorf("publish conversation run start: %w", err)
	}

	if s.metrics != nil {
		s.metrics.RecordRunStarted(ctx, "type", "conversation_agentic", "project.id", proj.ID)
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
	model        string
	extraContext []messagequeue.ContextEntryPayload
}

// WithModel overrides the default model resolution for the agentic run.
// This preserves the caller's explicit model choice through mode redirects.
func WithModel(model string) AgenticOption {
	return func(o *agenticOpts) {
		o.model = model
	}
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

	// Apply functional options early so the model override is available.
	var applied agenticOpts
	for _, o := range opts {
		o(&applied)
	}

	// Resolve model: explicit option override takes priority over config cascade.
	model := applied.model
	if model == "" {
		model = s.resolveModel()
	}
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

	// Merge extra context entries from functional options.
	if len(applied.extraContext) > 0 {
		contextEntries = append(contextEntries, applied.extraContext...)
	}

	// Evaluate system reminders.
	reminders := s.evaluateReminders(ctx, conversationID, protoMessages)

	runID := conversationID
	payload := messagequeue.ConversationRunStartPayload{
		RunID:              runID,
		ConversationID:     conversationID,
		SessionID:          sessionID,
		ProjectID:          proj.ID,
		Messages:           protoMessages,
		SystemPrompt:       systemPrompt,
		Model:              model,
		PolicyProfile:      policyProfile,
		WorkspacePath:      proj.WorkspacePath,
		Mode:               resolvedMode,
		Termination:        termination,
		MCPServers:         mcpDefs,
		MicroagentPrompts:  microagentPrompts,
		RoutingEnabled:     s.routingCfg != nil && s.routingCfg.Enabled,
		Context:            contextEntries,
		Agentic:            true,
		PlanActEnabled:     modeAutonomy >= 4,
		TenantID:           tenantctx.FromContext(ctx),
		SessionMeta:        sessionMeta,
		Reminders:          reminders,
		SummarizeThreshold: s.summarizeThreshold(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal conversation run start: %w", err)
	}

	s.hub.BroadcastEvent(ctx, event.AGUIRunStarted, event.AGUIRunStartedEvent{
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

	// Use the modular prompt assembler (YAML library) as the sole path.
	if s.promptAssembler != nil {
		asmCtx := prompt.AssemblyContext{
			ModeID:   "coder", // default for conversations
			Autonomy: 3,       // default
			Env:      s.appEnv,
			Agentic:  true,
		}
		if result := s.promptAssembler.Assemble(asmCtx, data); result != "" {
			return result
		}
		slog.Error("prompt assembler returned empty result, using minimal fallback",
			"project", data.ProjectName)
	}

	return fmt.Sprintf("You are CodeForge, an AI coding orchestrator. Project: %s", data.ProjectName)
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
	parts := make([]string, len(result.Languages))
	for i, lang := range result.Languages {
		if len(lang.Frameworks) > 0 {
			parts[i] = fmt.Sprintf("%s (%s)", lang.Name, strings.Join(lang.Frameworks, ", "))
		} else {
			parts[i] = lang.Name
		}
	}
	return strings.Join(parts, ", "), nil
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

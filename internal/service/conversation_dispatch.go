package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

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

// resolveFullAutoGate checks whether the full-auto gate should redirect
// to goal_researcher mode. Returns true if the redirect was performed.
func (s *ConversationService) resolveFullAutoGate(ctx context.Context, proj *project.Project, conversationID, userMessage, model string) (redirected bool, err error) {
	if !s.isFullAutoProject(ctx, proj) || s.goalSvc == nil {
		return false, nil
	}

	goals, _ := s.goalSvc.ListEnabled(ctx, proj.ID)
	if len(goals) > 0 {
		return false, nil
	}

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

	if hasOpenFeatures {
		return false, nil
	}

	slog.Info("full-auto gate: no goals or open features, redirecting to goal_researcher",
		"project_id", proj.ID,
		"conversation_id", conversationID,
	)
	return true, s.SendMessageAgenticWithMode(ctx, conversationID, userMessage, "goal_researcher", WithModel(model))
}

// resolveModelAndMode resolves the LLM model and agent mode for a conversation run.
// modeID is the preferred mode; if empty, conv.Mode or "coder" is used.
func (s *ConversationService) resolveModelAndMode(explicitModel, modeID, convMode string) (model string, resolvedMode *messagequeue.ModePayload, autonomy int, err error) {
	model = explicitModel
	if model == "" {
		model = s.resolveModel()
	}
	if model == "" {
		return "", nil, 0, fmt.Errorf("no LLM model configured — set conversation_model in litellm config or default_model in agent config")
	}

	if s.modeSvc != nil {
		if modeID == "" {
			modeID = convMode
		}
		if modeID == "" {
			modeID = "coder"
		}
		if m, mErr := s.modeSvc.Get(modeID); mErr == nil {
			autonomy = m.Autonomy
			resolvedMode = &messagequeue.ModePayload{
				ID:               m.ID,
				LLMScenario:      m.LLMScenario,
				Tools:            m.Tools,
				DeniedTools:      m.DeniedTools,
				ModelAdaptations: m.ModelAdaptations,
			}
		}
	}
	return model, resolvedMode, autonomy, nil
}

// buildMCPDefinitions builds the MCP server definition payloads for a project.
func (s *ConversationService) buildMCPDefinitions(projectID string) []messagequeue.MCPServerDefPayload {
	if s.mcpSvc == nil {
		return nil
	}
	servers := s.mcpSvc.ResolveForRun(projectID, "")
	defs := make([]messagequeue.MCPServerDefPayload, 0, len(servers))
	for i := range servers {
		defs = append(defs, messagequeue.MCPServerDefPayload{
			ID:        servers[i].ID,
			Name:      servers[i].Name,
			Transport: string(servers[i].Transport),
			Command:   servers[i].Command,
			Args:      servers[i].Args,
			URL:       servers[i].URL,
			Env:       servers[i].Env,
			Enabled:   servers[i].Enabled,
		})
	}
	return defs
}

// matchMicroagents matches microagent trigger patterns against a user message
// and returns the collected prompt strings.
func (s *ConversationService) matchMicroagents(ctx context.Context, projectID, content, conversationID string) []string {
	if s.microagentSvc == nil {
		return nil
	}
	matched, maErr := s.microagentSvc.Match(ctx, projectID, content)
	if maErr != nil {
		slog.Warn("microagent match failed", "conversation_id", conversationID, "error", maErr)
		return nil
	}
	if len(matched) == 0 {
		return nil
	}
	prompts := make([]string, 0, len(matched))
	for i := range matched {
		prompts = append(prompts, matched[i].Prompt)
	}
	slog.Info("microagents matched for conversation", "conversation_id", conversationID, "count", len(matched))
	return prompts
}

// AgenticOption configures optional behaviour for agentic run dispatch.
type AgenticOption func(*agenticOpts)

type agenticOpts struct {
	model          string
	extraContext   []messagequeue.ContextEntryPayload
	dedupKey       string
	providerAPIKey string
	rolloutCount   int
	recordMetrics  bool
	agentName      string // WS broadcast agent name ("agent" default, or modeID)
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

// dispatchAgenticRun is the shared core for SendMessageAgentic and SendMessageAgenticWithMode.
// It stores the user message, builds the NATS payload, and publishes the conversation run start.
func (s *ConversationService) dispatchAgenticRun(
	ctx context.Context,
	conv *conversation.Conversation,
	userMessage, modeID string,
	opts *agenticOpts,
) error {
	conversationID := conv.ID

	// Store user message.
	userMsg := &conversation.Message{
		ConversationID: conversationID,
		Role:           "user",
		Content:        userMessage,
	}
	if _, err := s.db.CreateMessage(ctx, userMsg); err != nil {
		return fmt.Errorf("store user message: %w", err)
	}

	// Load full conversation history.
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

	// Resolve model and mode.
	model, resolvedMode, modeAutonomy, modeErr := s.resolveModelAndMode(opts.model, modeID, conv.Mode)
	if modeErr != nil {
		return modeErr
	}

	// Resolve policy profile.
	policyProfile := ""
	if s.policySvc != nil {
		modePolicy := ""
		if modeAutonomy > 0 {
			modePolicy = policyForAutonomy(modeAutonomy)
		}
		policyProfile = s.policySvc.ResolveProfile(modePolicy, proj.PolicyProfile)
	}

	systemPrompt = appendModelAdaptation(systemPrompt, model, resolvedMode)

	// Termination config.
	termination := messagequeue.TerminationPayload{
		MaxSteps:       50,
		TimeoutSeconds: 600,
	}
	if s.agentCfg != nil && s.agentCfg.MaxLoopIterations > 0 {
		termination.MaxSteps = s.agentCfg.MaxLoopIterations
	}

	// Build context entries + merge extra from functional options.
	contextEntries := s.buildConversationContextEntries(ctx, proj.ID, userMessage, conversationID, protoMessages)
	if len(opts.extraContext) > 0 {
		contextEntries = append(contextEntries, opts.extraContext...)
	}

	reminders := s.evaluateReminders(ctx, conversationID, protoMessages)

	// Resolve rollout count (only for autonomy >= 4, capped at 8).
	rolloutCount := opts.rolloutCount
	if rolloutCount <= 0 {
		rolloutCount = 1
	}

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
		MCPServers:         s.buildMCPDefinitions(proj.ID),
		MicroagentPrompts:  s.matchMicroagents(ctx, proj.ID, userMessage, conversationID),
		RoutingEnabled:     s.routingCfg != nil && s.routingCfg.Enabled,
		Context:            contextEntries,
		Agentic:            true,
		PlanActEnabled:     modeAutonomy >= 4,
		ProviderAPIKey:     opts.providerAPIKey,
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

	agentName := opts.agentName
	if agentName == "" {
		agentName = "agent"
	}
	s.hub.BroadcastEvent(ctx, event.AGUIRunStarted, event.AGUIRunStartedEvent{
		RunID:     runID,
		ThreadID:  conversationID,
		AgentName: agentName,
	})

	// Publish with dedup key when provided, plain publish otherwise.
	if opts.dedupKey != "" {
		if err := s.queue.PublishWithDedup(ctx, messagequeue.SubjectConversationRunStart, data, opts.dedupKey); err != nil {
			s.hub.BroadcastEvent(ctx, event.AGUIRunFinished, event.AGUIRunFinishedEvent{
				RunID:  runID,
				Status: "failed",
				Error:  err.Error(),
			})
			return fmt.Errorf("publish conversation run start: %w", err)
		}
	} else {
		if err := s.queue.Publish(ctx, messagequeue.SubjectConversationRunStart, data); err != nil {
			return fmt.Errorf("publish conversation run start: %w", err)
		}
	}

	if opts.recordMetrics && s.metrics != nil {
		s.metrics.RecordRunStarted(ctx, "type", "conversation_agentic", "project.id", proj.ID)
	}

	slog.Info("conversation agentic run dispatched",
		"run_id", runID,
		"conversation_id", conversationID,
		"session_id", sessionID,
		"project_id", proj.ID,
		"mode", modeID,
		"model", model,
	)

	return nil
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

	proj, err := s.db.GetProject(ctx, conv.ProjectID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	// Full-auto gate: redirect to goal_researcher if no goals/features exist.
	if redirected, gateErr := s.resolveFullAutoGate(ctx, proj, conversationID, req.Content, req.Model); redirected || gateErr != nil {
		return gateErr
	}

	// Resolve rollout count (only for autonomy >= 4).
	rolloutCount := 1
	if s.agentCfg != nil && s.agentCfg.ConversationRolloutCount > 1 {
		_, _, modeAutonomy, resolveErr := s.resolveModelAndMode(req.Model, req.Mode, conv.Mode)
		if resolveErr != nil {
			slog.Warn("rollout mode resolution failed, using default autonomy", "error", resolveErr)
		}
		if modeAutonomy >= 4 {
			rolloutCount = min(s.agentCfg.ConversationRolloutCount, 8)
		}
	}

	return s.dispatchAgenticRun(ctx, conv, req.Content, req.Mode, &agenticOpts{
		model:          req.Model,
		dedupKey:       "conv-start-" + uuid.New().String(),
		providerAPIKey: s.resolveProviderAPIKey(ctx, req.UserID, req.Model),
		rolloutCount:   rolloutCount,
		recordMetrics:  true,
		agentName:      "agent",
	})
}

// SendMessageAgenticWithMode is like SendMessageAgentic but accepts a mode ID override
// instead of defaulting to "coder". Used for specialized agent flows like goal discovery.
// Optional AgenticOption values can inject extra context entries into the NATS payload.
func (s *ConversationService) SendMessageAgenticWithMode(ctx context.Context, conversationID, userMessage, modeID string, opts ...AgenticOption) error {
	if userMessage == "" {
		return errors.New("content is required")
	}
	if s.queue == nil {
		return errors.New("agentic mode requires NATS queue")
	}

	conv, err := s.db.GetConversation(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}

	// Apply functional options.
	var applied agenticOpts
	for _, o := range opts {
		o(&applied)
	}
	applied.agentName = modeID

	return s.dispatchAgenticRun(ctx, conv, userMessage, modeID, &applied)
}

// resolveProviderAPIKey attempts to look up the user's per-provider LLM key.
// Delegates to PromptAssemblyService when available.
func (s *ConversationService) resolveProviderAPIKey(ctx context.Context, userID, model string) string {
	if s.promptSvc != nil {
		return s.promptSvc.ResolveProviderAPIKey(ctx, userID, model)
	}
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

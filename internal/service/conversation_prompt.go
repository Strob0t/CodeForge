package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/prompt"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// PromptAssemblyService handles system prompt construction, context entry building,
// reminder evaluation, budget computation, and model-family adaptations.
type PromptAssemblyService struct {
	store           database.Store
	contextOpt      *ContextOptimizerService
	goalSvc         *GoalDiscoveryService
	modeSvc         *ModeService
	promptAssembler *PromptAssembler
	events          eventstore.Store
	agentCfg        *config.Agent
	appEnv          string
	llmKeySvc       *LLMKeyService
}

// NewPromptAssemblyService creates a prompt assembly service.
func NewPromptAssemblyService(
	store database.Store,
	contextOpt *ContextOptimizerService,
	goalSvc *GoalDiscoveryService,
	modeSvc *ModeService,
	assembler *PromptAssembler,
	events eventstore.Store,
	agentCfg *config.Agent,
) *PromptAssemblyService {
	return &PromptAssemblyService{
		store:           store,
		contextOpt:      contextOpt,
		goalSvc:         goalSvc,
		modeSvc:         modeSvc,
		promptAssembler: assembler,
		events:          events,
		agentCfg:        agentCfg,
	}
}

// SetAppEnv configures the application environment for prompt assembly.
func (s *PromptAssemblyService) SetAppEnv(env string) { s.appEnv = env }

// SetLLMKeyService configures per-user LLM key resolution.
func (s *PromptAssemblyService) SetLLMKeyService(svc *LLMKeyService) { s.llmKeySvc = svc }

// SetPromptAssembler configures the modular prompt assembler.
func (s *PromptAssemblyService) SetPromptAssembler(a *PromptAssembler) { s.promptAssembler = a }

// SetEventStore configures the event store for budget tracking.
func (s *PromptAssemblyService) SetEventStore(es eventstore.Store) { s.events = es }

// SetGoalService wires the goal discovery service.
func (s *PromptAssemblyService) SetGoalService(svc *GoalDiscoveryService) { s.goalSvc = svc }

// SetContextOptimizer configures the context optimizer.
func (s *PromptAssemblyService) SetContextOptimizer(opt *ContextOptimizerService) {
	s.contextOpt = opt
}

// BuildSystemPrompt assembles the system prompt for a conversation using the
// embedded template and project context. Failures in fetching optional context
// (agents, tasks, roadmap) are logged and skipped gracefully.
func (s *PromptAssemblyService) BuildSystemPrompt(ctx context.Context, projectID string) string {
	data := conversationPromptData{}

	proj, err := s.store.GetProject(ctx, projectID)
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

	agents, err := s.store.ListAgents(ctx, projectID)
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

	if s.modeSvc != nil {
		modes := s.modeSvc.List()
		for i := range modes {
			data.Modes = append(data.Modes, modes[i].Name)
		}
	}

	tasks, err := s.store.ListTasks(ctx, projectID)
	if err != nil {
		slog.Debug("conversation: failed to list tasks for system prompt", "project_id", projectID, "error", err)
	} else {
		limit := min(10, len(tasks))
		start := len(tasks) - limit
		for i := range tasks[start:] {
			data.RecentTasks = append(data.RecentTasks, conversationTaskSummary{
				ID:     tasks[start+i].ID,
				Name:   tasks[start+i].Title,
				Status: string(tasks[start+i].Status),
			})
		}
	}

	rm, err := s.store.GetRoadmapByProject(ctx, projectID)
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

	if s.goalSvc != nil {
		goals, gErr := s.goalSvc.ListEnabled(ctx, projectID)
		if gErr == nil && len(goals) > 0 {
			data.GoalContext = renderGoalContext(goals)
		}
	}

	if data.WorkspacePath != "" {
		stack, stackErr := detectStackSummary(data.WorkspacePath)
		if stackErr == nil && stack != "" {
			data.Stack = stack
		}
	}

	data.BuiltinTools = []builtinToolSummary{
		{Name: "Read", Description: "Read file contents with optional line range"},
		{Name: "Write", Description: "Create or overwrite a file"},
		{Name: "Edit", Description: "Search-and-replace edit within a file"},
		{Name: "Bash", Description: "Execute a shell command"},
		{Name: "Search", Description: "Regex search across files"},
		{Name: "Glob", Description: "Find files by glob pattern"},
		{Name: "ListDir", Description: "List directory contents"},
	}

	if s.promptAssembler != nil {
		asmCtx := prompt.AssemblyContext{
			ModeID:   "coder",
			Autonomy: 3,
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

// BuildConversationContextEntries assembles context entries for a conversation run.
func (s *PromptAssemblyService) BuildConversationContextEntries(
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

// EvaluateReminders checks runtime conditions and returns matching reminder texts.
func (s *PromptAssemblyService) EvaluateReminders(
	ctx context.Context,
	conversationID string,
	history []messagequeue.ConversationMessagePayload,
) []string {
	if s.promptAssembler == nil || s.promptAssembler.library == nil {
		return nil
	}

	reminders := s.promptAssembler.library.GetByCategory(prompt.CategoryReminder)
	if len(reminders) == 0 {
		return nil
	}

	budgetPct, budgetUsed, budgetLimit := s.ComputeBudget(ctx, conversationID)

	data := reminderTemplateData{
		TurnCount:       len(history),
		BudgetPercent:   budgetPct,
		BudgetUsed:      budgetUsed,
		BudgetLimit:     budgetLimit,
		StallIterations: countStallIterations(history),
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

// ComputeBudget queries the event store for accumulated cost and returns
// (percent, usedStr, limitStr).
func (s *PromptAssemblyService) ComputeBudget(ctx context.Context, conversationID string) (pct float64, usedStr, limitStr string) {
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

// HistoryToPayload converts domain messages to protocol payload messages.
func HistoryToPayload(messages []conversation.Message) []messagequeue.ConversationMessagePayload {
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
		if len(messages[i].ToolCalls) > 0 {
			var tcs []messagequeue.ConversationToolCall
			if err := json.Unmarshal(messages[i].ToolCalls, &tcs); err == nil {
				pm.ToolCalls = tcs
			}
		}
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

// ResolveProviderAPIKey attempts to look up the user's per-provider LLM key.
func (s *PromptAssemblyService) ResolveProviderAPIKey(ctx context.Context, userID, model string) string {
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

// FingerprintForMode returns the prompt fingerprint for a given mode.
func (s *PromptAssemblyService) FingerprintForMode(modeID string) string {
	if s.promptAssembler == nil {
		return ""
	}
	return s.promptAssembler.FingerprintForMode(modeID)
}

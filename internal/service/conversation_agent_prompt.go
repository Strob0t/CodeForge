package service

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/prompt"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// buildConversationContextEntries assembles context entries for a conversation run.
// Delegates to PromptAssemblyService when available.
func (s *ConversationService) buildConversationContextEntries(
	ctx context.Context,
	projectID, userMessage, conversationID string,
	history []messagequeue.ConversationMessagePayload,
) []messagequeue.ContextEntryPayload {
	if s.promptSvc != nil {
		return s.promptSvc.BuildConversationContextEntries(ctx, projectID, userMessage, conversationID, history)
	}
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

// evaluateReminders returns the reminders that fire for a dispatched turn.
// Delegates to PromptAssemblyService when available.
func (s *ConversationService) evaluateReminders(
	ctx context.Context,
	conversationID string,
	mode *messagequeue.ModePayload,
	autonomy int,
	history []messagequeue.ConversationMessagePayload,
) []string {
	if s.promptSvc != nil {
		return s.promptSvc.EvaluateReminders(ctx, conversationID, mode, autonomy, history)
	}
	if s.promptAssembler == nil || s.promptAssembler.library == nil {
		return nil
	}
	budgetPct, budgetUsed, budgetLimit := s.computeBudget(ctx, conversationID)
	data := reminderData(mode, history, budgetPct, budgetUsed, budgetLimit)
	return firingReminders(s.promptAssembler.library, reminderContext(mode, autonomy, s.appEnv), &data)
}

// reminderContext is the assembly context of a dispatched conversation turn
// in mode (nil without a mode service).
func reminderContext(mode *messagequeue.ModePayload, autonomy int, env string) prompt.AssemblyContext {
	ctx := prompt.AssemblyContext{Autonomy: autonomy, Env: env, Agentic: true}
	if mode != nil {
		ctx.ModeID = mode.ID
	}
	return ctx
}

// reminderData collects the runtime signals of a dispatched turn that
// reminders fire on and render.
func reminderData(
	mode *messagequeue.ModePayload,
	history []messagequeue.ConversationMessagePayload,
	budgetPct float64, budgetUsed, budgetLimit string,
) reminderTemplateData {
	data := reminderTemplateData{
		TurnCount:     len(history),
		BudgetPercent: budgetPct,
		BudgetUsed:    budgetUsed,
		BudgetLimit:   budgetLimit,
	}
	if modeCanProgress(mode) {
		data.StallIterations = countStallIterations(history)
	}
	return data
}

// firingReminders renders the reminders of lib that fire for the turn
// (PromptEntry.ReminderFires, KI-190). A reminder without a condition is
// skipped and logged: it would otherwise be sent on every turn.
func firingReminders(lib *PromptLibraryService, turn prompt.AssemblyContext, data *reminderTemplateData) []string {
	sig := prompt.ReminderSignals{StallIterations: data.StallIterations, BudgetPercent: data.BudgetPercent}
	reminders := lib.GetByCategory(prompt.CategoryReminder)
	var result []string
	for i := range reminders {
		if !reminders[i].HasReminderCondition() {
			slog.Warn("prompt reminder has no condition, skipped", "id", reminders[i].ID)
			continue
		}
		if !reminders[i].ReminderFires(turn, sig) {
			continue
		}
		if text := renderedEntry(&reminders[i], data); text != "" {
			result = append(result, text)
		}
	}
	return result
}

// modeCanProgress reports whether a mode may use a progress tool (Edit,
// Write, Bash). In a read-only mode every call is a read, which is no stall.
// No mode (no mode service) means the default coder mode.
func modeCanProgress(m *messagequeue.ModePayload) bool {
	if m == nil {
		return true
	}
	isTool := func(tool string) func(string) bool {
		return func(name string) bool { return policy.CanonicalTool(name) == tool }
	}
	for tool := range run.ProgressTools {
		if slices.ContainsFunc(m.DeniedTools, isTool(tool)) {
			continue
		}
		if len(m.Tools) == 0 || slices.ContainsFunc(m.Tools, isTool(tool)) {
			return true
		}
	}
	return false
}

// defaultBudgetLimit is the default cost budget (USD) when no explicit limit is configured.
const defaultBudgetLimit = 5.0

// computeBudget queries the event store for accumulated cost.
// Delegates to PromptAssemblyService when available.
func (s *ConversationService) computeBudget(ctx context.Context, conversationID string) (pct float64, usedStr, limitStr string) {
	if s.promptSvc != nil {
		return s.promptSvc.ComputeBudget(ctx, conversationID)
	}
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

// progressToolsConv references the canonical progress tools from the run package.
var progressToolsConv = run.ProgressTools

// countStallIterations counts consecutive non-progress tool results at the tail
// of the message history. A "progress" tool is Edit, Write, or Bash, under its
// canonical name (the worker stores edit_file, write_file, bash). Non-tool
// messages (assistant, user) are skipped. The count resets on any progress tool.
func countStallIterations(history []messagequeue.ConversationMessagePayload) int {
	count := 0
	for i := range history {
		if history[i].Role != "tool" {
			continue
		}
		if progressToolsConv[policy.CanonicalTool(history[i].Name)] {
			count = 0
		} else {
			count++
		}
	}
	return count
}

// ExtractModelFamily returns the provider prefix from a model string
// (e.g. "openai/gpt-4o" -> "openai"). Returns the full string when no slash is present.
func ExtractModelFamily(model string) string {
	if idx := strings.Index(model, "/"); idx > 0 {
		return model[:idx]
	}
	return model
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

// buildSystemPrompt assembles the system prompt for a conversation.
// Delegates unconditionally to PromptAssemblyService.
func (s *ConversationService) buildSystemPrompt(ctx context.Context, projectID string) string {
	if s.promptSvc == nil {
		return "You are a helpful coding assistant."
	}
	return s.promptSvc.BuildSystemPrompt(ctx, projectID)
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

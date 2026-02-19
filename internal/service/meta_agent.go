package service

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"text/template"
	"unicode"

	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

var decomposeTemplates = template.Must(template.ParseFS(templateFS, "templates/*.tmpl"))

// decomposeData provides data for the decomposition prompt templates.
type decomposeData struct {
	Feature string
	Context string
	Agents  []agent.Agent
	Tasks   []task.Task
}

// MetaAgentService uses an LLM to decompose features into subtasks and build execution plans.
type MetaAgentService struct {
	store   database.Store
	llm     *litellm.Client
	orchSvc *OrchestratorService
	orchCfg *config.Orchestrator
}

// NewMetaAgentService creates a MetaAgentService with all dependencies.
func NewMetaAgentService(
	store database.Store,
	llm *litellm.Client,
	orchSvc *OrchestratorService,
	orchCfg *config.Orchestrator,
) *MetaAgentService {
	return &MetaAgentService{
		store:   store,
		llm:     llm,
		orchSvc: orchSvc,
		orchCfg: orchCfg,
	}
}

// DecomposeFeature uses an LLM to break a feature description into subtasks,
// creates the tasks in the database, and builds an execution plan.
func (s *MetaAgentService) DecomposeFeature(ctx context.Context, req *plan.DecomposeRequest) (*plan.ExecutionPlan, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate decompose request: %w", err)
	}

	// Verify project exists
	if _, err := s.store.GetProject(ctx, req.ProjectID); err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	// Load available agents for the project
	agents, err := s.store.ListAgents(ctx, req.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	if len(agents) == 0 {
		return nil, fmt.Errorf("project has no agents configured")
	}

	// Load existing tasks for context
	tasks, err := s.store.ListTasks(ctx, req.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}

	// Build and send LLM request
	model := req.Model
	if model == "" {
		model = s.orchCfg.DecomposeModel
	}
	maxTokens := s.orchCfg.DecomposeMaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	systemPrompt, userPrompt := buildDecomposePrompt(req.Feature, req.Context, agents, tasks)

	llmResp, err := s.llm.ChatCompletion(ctx, litellm.ChatCompletionRequest{
		Model: model,
		Messages: []litellm.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.2,
		MaxTokens:   maxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("llm decomposition: %w", err)
	}

	// Parse structured JSON from LLM response
	var result plan.DecomposeResult
	content := extractJSON(llmResp.Content)
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parse decomposition result: %w (content: %s)", err, truncate(llmResp.Content, 200))
	}
	if err := result.ValidateResult(); err != nil {
		return nil, fmt.Errorf("invalid decomposition: %w", err)
	}

	// Override protocol if LLM's strategy doesn't match
	if result.Protocol == "" {
		result.Protocol = plan.StrategyToProtocol(result.Strategy)
	}

	// Create tasks in DB
	taskIDs := make([]string, len(result.Subtasks))
	for i, st := range result.Subtasks {
		created, err := s.store.CreateTask(ctx, task.CreateRequest{
			ProjectID: req.ProjectID,
			Title:     st.Title,
			Prompt:    st.Prompt,
		})
		if err != nil {
			return nil, fmt.Errorf("create subtask %d: %w", i, err)
		}
		taskIDs[i] = created.ID
	}

	// Build plan steps with agent assignment
	steps := make([]plan.CreateStepRequest, len(result.Subtasks))
	for i, st := range result.Subtasks {
		agentID := selectAgent(agents, st.AgentHint)
		deps := make([]string, len(st.DependsOn))
		for j, d := range st.DependsOn {
			deps[j] = strconv.Itoa(d)
		}
		steps[i] = plan.CreateStepRequest{
			TaskID:    taskIDs[i],
			AgentID:   agentID,
			DependsOn: deps,
		}
	}

	// Create the execution plan
	planReq := &plan.CreatePlanRequest{
		Name:        result.PlanName,
		Description: result.Description,
		ProjectID:   req.ProjectID,
		Protocol:    result.Protocol,
		Steps:       steps,
	}

	p, err := s.orchSvc.CreatePlan(ctx, planReq)
	if err != nil {
		return nil, fmt.Errorf("create plan: %w", err)
	}

	slog.Info("feature decomposed",
		"plan_id", p.ID,
		"subtasks", len(result.Subtasks),
		"strategy", result.Strategy,
		"protocol", result.Protocol,
		"tokens_in", llmResp.TokensIn,
		"tokens_out", llmResp.TokensOut,
	)

	// Auto-start if configured
	mode := plan.OrchestratorMode(s.orchCfg.Mode)
	if req.AutoStart || mode == plan.ModeFullAuto {
		started, err := s.orchSvc.StartPlan(ctx, p.ID)
		if err != nil {
			slog.Error("auto-start plan failed", "plan_id", p.ID, "error", err)
			return p, nil // return plan even if auto-start fails
		}
		return started, nil
	}

	return p, nil
}

// sanitizePromptInput strips control characters and common prompt injection
// patterns from user-supplied text before it is embedded in an LLM prompt.
// This prevents role-override attacks (e.g., "system: ignore all previous
// instructions") and fence escaping.
func sanitizePromptInput(s string) string {
	// Strip non-printable control characters (keep newlines, tabs, spaces).
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r == '\r' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)

	// Remove common prompt injection role markers at line beginnings.
	// These could trick the LLM into treating user data as system instructions.
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(strings.ToLower(line))
		for _, prefix := range []string{
			"system:", "assistant:", "user:", "[system]", "[assistant]",
			"<|system|>", "<|assistant|>", "<|im_start|>",
			"### system", "### assistant", "### instruction",
		} {
			if strings.HasPrefix(trimmed, prefix) {
				// Replace the role marker prefix with a safe escaped version.
				lines[i] = "[sanitized] " + line
				break
			}
		}
	}
	s = strings.Join(lines, "\n")

	// Enforce a reasonable length limit to prevent context flooding.
	const maxInputLen = 10000
	if len(s) > maxInputLen {
		s = s[:maxInputLen] + "\n[truncated]"
	}

	return s
}

// buildDecomposePrompt constructs the system and user prompts for feature decomposition
// using embedded text/template files (P1-2).
func buildDecomposePrompt(feature, extraContext string, agents []agent.Agent, tasks []task.Task) (system, userPrompt string) {
	// Sanitize user-provided inputs before passing to template.
	feature = sanitizePromptInput(feature)
	extraContext = sanitizePromptInput(extraContext)

	data := decomposeData{
		Feature: feature,
		Context: extraContext,
		Agents:  agents,
		Tasks:   tasks,
	}

	var sysBuf bytes.Buffer
	if err := decomposeTemplates.ExecuteTemplate(&sysBuf, "decompose_system.tmpl", data); err != nil {
		slog.Error("failed to execute system template", "error", err)
		return "You are a software engineering project planner.", ""
	}

	var usrBuf bytes.Buffer
	if err := decomposeTemplates.ExecuteTemplate(&usrBuf, "decompose_user.tmpl", data); err != nil {
		slog.Error("failed to execute user template", "error", err)
		return sysBuf.String(), feature
	}

	return sysBuf.String(), usrBuf.String()
}

// selectAgent picks the best agent for a subtask based on the hint.
func selectAgent(agents []agent.Agent, hint string) string {
	if hint == "" {
		return agents[0].ID
	}

	// Match by backend
	for i := range agents {
		if strings.EqualFold(agents[i].Backend, hint) {
			return agents[i].ID
		}
	}

	// Match by name (substring)
	hint = strings.ToLower(hint)
	for i := range agents {
		if strings.Contains(strings.ToLower(agents[i].Name), hint) {
			return agents[i].ID
		}
	}

	// Prefer idle agents as fallback
	for i := range agents {
		if agents[i].Status == agent.StatusIdle {
			return agents[i].ID
		}
	}

	return agents[0].ID
}

// extractJSON attempts to extract a JSON object from a string that may contain
// markdown fences or other surrounding text.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)

	// Strip markdown code fences
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
		return strings.TrimSpace(s)
	}
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
		return strings.TrimSpace(s)
	}

	// Find first { and last }
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}

	return s
}

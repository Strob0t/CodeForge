package http

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/resource"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/agentbackend"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
	"github.com/Strob0t/CodeForge/internal/port/llm"
	"github.com/Strob0t/CodeForge/internal/port/tokenexchange"
	"github.com/Strob0t/CodeForge/internal/service"
)

// llmFull combines all LLM port interfaces used by handlers.
type llmFull interface {
	llm.Provider
	llm.ModelDiscoverer
	llm.ModelAdmin
}

// Handlers holds the HTTP handler dependencies.
// Domain-specific handler groups (Project, Agent, Task, Run, Policy, Utility)
// own their respective methods and service dependencies.
// The remaining fields serve existing handler files (conversation, benchmark, etc.).
type Handlers struct {
	// Domain-specific handler groups
	Project *ProjectHandlers
	Agent   *AgentHandlers
	Task    *TaskHandlers
	Run     *RunHandlers
	Policy  *PolicyHandlers
	Utility *UtilityHandlers

	Projects         *service.ProjectService
	Tasks            *service.TaskService
	Agents           *service.AgentService
	LLM              llmFull
	Policies         *service.PolicyService
	PolicyDir        string // Custom policy YAML directory (empty = no persistence)
	Runtime          *service.RuntimeService
	Orchestrator     *service.OrchestratorService
	MetaAgent        *service.MetaAgentService
	PoolManager      *service.PoolManagerService
	TaskPlanner      *service.TaskPlannerService
	ContextOptimizer *service.ContextOptimizerService
	SharedContext    *service.SharedContextService
	Modes            *service.ModeService
	RepoMap          *service.RepoMapService
	Retrieval        *service.RetrievalService
	Graph            *service.GraphService
	Events           eventstore.Store
	Cost             *service.CostService
	Roadmap          *service.RoadmapService
	Tenants          *service.TenantService
	BranchProtection *service.BranchProtectionService
	Replay           *service.ReplayService
	Sessions         *service.SessionService
	VCSWebhook       *service.VCSWebhookService
	Sync             *service.SyncService
	PMWebhook        *service.PMWebhookService
	Notification     *service.NotificationService
	Auth             *service.AuthService
	Scope            *service.ScopeService
	Pipelines        *service.PipelineService
	Review           *service.ReviewService
	KnowledgeBases   *service.KnowledgeBaseService
	Settings         *service.SettingsService
	VCSAccounts      *service.VCSAccountService
	LLMKeys          *service.LLMKeyService
	Conversations    *service.ConversationService
	LSP              *service.LSPService
	MCP              *service.MCPService
	PromptSections   *service.PromptSectionService
	Benchmarks       *service.BenchmarkService
	ReviewRouter     *service.ReviewRouterService
	ModelRegistry    *service.ModelRegistry
	TokenExchanger   tokenexchange.Exchanger
	Memory           *service.MemoryService
	ExperiencePool   *service.ExperiencePoolService
	Microagents      *service.MicroagentService
	Skills           *service.SkillService
	Files            *service.FileService
	AutoAgent        *service.AutoAgentService
	Quarantine       *service.QuarantineService
	ActiveWork       *service.ActiveWorkService
	Routing          *service.RoutingService
	A2A              *service.A2AService
	GoalDiscovery    *service.GoalDiscoveryService
	Dashboard        *service.DashboardService
	GitHubOAuth      *service.GitHubOAuthService
	BackendHealth    *service.BackendHealthService
	Checkpoint       *service.CheckpointService
	Commands         *service.CommandService
	Channels         *service.ChannelService
	Subscription     *service.SubscriptionService
	Limits           *config.Limits
	AgentConfig      *config.Agent
	AppEnv           string // From cfg.AppEnv (APP_ENV env var)
	OllamaBaseURL    string // From cfg.Ollama.BaseURL (OLLAMA_BASE_URL env var)
	Boundaries       *service.BoundaryService
	ReviewTrigger    *service.ReviewTriggerService
	PromptEvolution  *service.PromptEvolutionService
}

// ListTasks handles GET /api/v1/projects/{id}/tasks
func (h *Handlers) ListTasks(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	tasks, err := h.Tasks.List(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	if tasks == nil {
		tasks = []task.Task{}
	}
	writeJSON(w, http.StatusOK, tasks)
}

// CreateTask handles POST /api/v1/projects/{id}/tasks
func (h *Handlers) CreateTask(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	req, ok := readJSON[task.CreateRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	req.ProjectID = projectID

	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	t, err := h.Tasks.Create(r.Context(), req)
	if err != nil {
		writeDomainError(w, err, "task creation failed")
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// GetTask handles GET /api/v1/tasks/{id}
func (h *Handlers) GetTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := h.Tasks.Get(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "task not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// ListAgents handles GET /api/v1/projects/{id}/agents
// Supports ?limit=N&offset=N query params (default limit=100, offset=0).
func (h *Handlers) ListAgents(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	agents, err := h.Agents.List(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	if agents == nil {
		agents = []agent.Agent{}
	}
	limit, offset := parsePagination(r, 100)
	agents = applyPagination(agents, limit, offset)
	writeJSON(w, http.StatusOK, agents)
}

// CreateAgent handles POST /api/v1/projects/{id}/agents
func (h *Handlers) CreateAgent(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	req, ok := readJSON[struct {
		Name           string            `json:"name"`
		Backend        string            `json:"backend"`
		Config         map[string]string `json:"config"`
		ResourceLimits *resource.Limits  `json:"resource_limits,omitempty"`
	}](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Backend == "" {
		writeError(w, http.StatusBadRequest, "backend is required")
		return
	}

	a, err := h.Agents.Create(r.Context(), projectID, req.Name, req.Backend, req.Config, req.ResourceLimits)
	if err != nil {
		writeDomainError(w, err, "create agent failed")
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

// GetAgent handles GET /api/v1/agents/{id}
func (h *Handlers) GetAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	a, err := h.Agents.Get(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "agent not found")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// DeleteAgent handles DELETE /api/v1/agents/{id}
func (h *Handlers) DeleteAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Agents.Delete(r.Context(), id); err != nil {
		writeDomainError(w, err, "agent not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DispatchTask handles POST /api/v1/agents/{id}/dispatch
func (h *Handlers) DispatchTask(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")

	req, ok := readJSON[struct {
		TaskID string `json:"task_id"`
	}](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.TaskID == "" {
		writeError(w, http.StatusBadRequest, "task_id is required")
		return
	}

	if err := h.Agents.Dispatch(r.Context(), agentID, req.TaskID); err != nil {
		writeDomainError(w, err, "agent or task not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "dispatched"})
}

// StopAgentTask handles POST /api/v1/agents/{id}/stop
func (h *Handlers) StopAgentTask(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")

	req, ok := readJSON[struct {
		TaskID string `json:"task_id"`
	}](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.TaskID == "" {
		writeError(w, http.StatusBadRequest, "task_id is required")
		return
	}

	if err := h.Agents.StopTask(r.Context(), agentID, req.TaskID); err != nil {
		writeDomainError(w, err, "agent or task not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// ListAgentInbox handles GET /api/v1/agents/{id}/inbox
func (h *Handlers) ListAgentInbox(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")
	unreadOnly := r.URL.Query().Get("unread") == "true"

	msgs, err := h.Agents.GetInbox(r.Context(), agentID, unreadOnly)
	if err != nil {
		writeDomainError(w, err, "list inbox failed")
		return
	}
	writeJSON(w, http.StatusOK, msgs)
}

// SendAgentMessage handles POST /api/v1/agents/{id}/inbox
func (h *Handlers) SendAgentMessage(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")

	req, ok := readJSON[struct {
		FromAgent string `json:"from_agent"`
		Content   string `json:"content"`
		Priority  int    `json:"priority"`
	}](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}

	msg := &agent.InboxMessage{
		AgentID:   agentID,
		FromAgent: req.FromAgent,
		Content:   req.Content,
		Priority:  req.Priority,
	}
	if err := h.Agents.SendMessage(r.Context(), msg); err != nil {
		writeDomainError(w, err, "send message failed")
		return
	}
	writeJSON(w, http.StatusCreated, msg)
}

// MarkInboxRead handles POST /api/v1/agents/{id}/inbox/{msgId}/read
func (h *Handlers) MarkInboxRead(w http.ResponseWriter, r *http.Request) {
	msgID := chi.URLParam(r, "msgId")
	if err := h.Agents.MarkRead(r.Context(), msgID); err != nil {
		writeDomainError(w, err, "message not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "read"})
}

// GetAgentState handles GET /api/v1/agents/{id}/state
func (h *Handlers) GetAgentState(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	a, err := h.Agents.Get(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "agent not found")
		return
	}
	writeJSON(w, http.StatusOK, a.State)
}

// UpdateAgentState handles PUT /api/v1/agents/{id}/state
func (h *Handlers) UpdateAgentState(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	state, ok := readJSON[map[string]string](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if err := h.Agents.UpdateState(r.Context(), id, state); err != nil {
		writeDomainError(w, err, "agent not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// ListTaskEvents handles GET /api/v1/tasks/{id}/events
func (h *Handlers) ListTaskEvents(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	events, err := h.Agents.LoadTaskEvents(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "events not found")
		return
	}
	if events == nil {
		events = []event.AgentEvent{}
	}
	writeJSON(w, http.StatusOK, events)
}

// ListGitProviders handles GET /api/v1/providers/git
func (h *Handlers) ListGitProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string][]string{
		"providers": gitprovider.Available(),
	})
}

// ListAgentBackends handles GET /api/v1/providers/agent
func (h *Handlers) ListAgentBackends(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string][]string{
		"backends": agentbackend.Available(),
	})
}

// ListLLMModels handles GET /api/v1/llm/models

// --- Policy Endpoints ---

// ListPolicyProfiles handles GET /api/v1/policies
func (h *Handlers) ListPolicyProfiles(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string][]string{
		"profiles": h.Policies.ListProfiles(),
	})
}

// GetPolicyProfile handles GET /api/v1/policies/{name}
func (h *Handlers) GetPolicyProfile(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	p, ok := h.Policies.GetProfile(name)
	if !ok {
		writeError(w, http.StatusNotFound, "policy profile not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// EvaluatePolicy handles POST /api/v1/policies/{name}/evaluate
func (h *Handlers) EvaluatePolicy(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	call, ok := readJSON[policy.ToolCall](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if call.Tool == "" {
		writeError(w, http.StatusBadRequest, "tool is required")
		return
	}

	result, err := h.Policies.EvaluateWithReason(r.Context(), name, call)
	if err != nil {
		writeDomainError(w, err, "policy not found")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// CreatePolicyProfile handles POST /api/v1/policies
func (h *Handlers) CreatePolicyProfile(w http.ResponseWriter, r *http.Request) {
	profile, ok := readJSON[policy.PolicyProfile](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if err := sanitizeName(profile.Name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.Policies.SaveProfile(&profile); err != nil {
		writeDomainError(w, err, "save policy profile failed")
		return
	}

	if h.PolicyDir != "" {
		path := filepath.Join(h.PolicyDir, profile.Name+".yaml")
		if err := os.MkdirAll(h.PolicyDir, 0o750); err != nil {
			slog.Error("failed to create policy directory", "error", err)
		} else if err := policy.SaveToFile(path, &profile); err != nil {
			slog.Error("failed to persist policy profile", "name", profile.Name, "error", err)
		}
	}

	writeJSON(w, http.StatusCreated, profile)
}

// DeletePolicyProfile handles DELETE /api/v1/policies/{name}
func (h *Handlers) DeletePolicyProfile(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := sanitizeName(name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.Policies.DeleteProfile(name); err != nil {
		if policy.IsPreset(name) {
			writeError(w, http.StatusForbidden, err.Error())
		} else {
			writeError(w, http.StatusNotFound, err.Error())
		}
		return
	}

	if h.PolicyDir != "" {
		path := filepath.Join(h.PolicyDir, name+".yaml")
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.Error("failed to remove policy file", "name", name, "error", err)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// AllowAlwaysPolicy handles POST /api/v1/policies/allow-always.
// It adds a persistent "allow" rule for a specific tool to a project's policy profile.
// If the project uses a built-in preset, a custom clone is created first.
func (h *Handlers) AllowAlwaysPolicy(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[struct {
		ProjectID string `json:"project_id"`
		Tool      string `json:"tool"`
		Command   string `json:"command,omitempty"`
	}](w, r, 1<<20)
	if !ok {
		return
	}
	if req.ProjectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}
	if req.Tool == "" {
		writeError(w, http.StatusBadRequest, "tool is required")
		return
	}

	ctx := r.Context()

	// Get the project to resolve its policy profile.
	proj, err := h.Projects.Get(ctx, req.ProjectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}

	// Resolve effective profile (project-level or service default).
	effectiveProfile := h.Policies.ResolveProfile("", proj.PolicyProfile)

	// If the resolved profile is a built-in preset, clone it to a custom profile.
	if policy.IsPreset(effectiveProfile) {
		source, _ := h.Policies.GetProfile(effectiveProfile)
		cloneName := effectiveProfile + "-custom-" + req.ProjectID
		clone := source
		clone.Name = cloneName
		clone.Description = fmt.Sprintf("Custom clone of %s for project %s", effectiveProfile, req.ProjectID)

		// Check if clone already exists (from a previous "Allow Always" call).
		if _, exists := h.Policies.GetProfile(cloneName); !exists {
			if err := h.Policies.SaveProfile(&clone); err != nil {
				writeInternalError(w, err)
				return
			}
		}

		// Update the project to use the custom clone.
		if err := h.Projects.SetPolicyProfile(ctx, req.ProjectID, cloneName); err != nil {
			writeInternalError(w, err)
			return
		}
		effectiveProfile = cloneName
	}

	// Construct the permission rule.
	spec := policy.ToolSpecifier{Tool: req.Tool}
	if req.Command != "" {
		// Use first word as command prefix pattern (e.g., "git" from "git status").
		parts := strings.SplitN(req.Command, " ", 2)
		spec.SubPattern = parts[0] + "*"
	}
	rule := policy.PermissionRule{
		Specifier: spec,
		Decision:  policy.DecisionAllow,
	}

	// Prepend the rule (idempotent — no-op if same specifier already exists).
	if err := h.Policies.PrependRule(effectiveProfile, &rule); err != nil {
		writeInternalError(w, err)
		return
	}

	// Persist to disk if PolicyDir is configured.
	if h.PolicyDir != "" {
		updated, ok := h.Policies.GetProfile(effectiveProfile)
		if ok {
			path := filepath.Join(h.PolicyDir, effectiveProfile+".yaml")
			if err := os.MkdirAll(h.PolicyDir, 0o750); err != nil {
				slog.Error("failed to create policy directory", "error", err)
			} else if err := policy.SaveToFile(path, &updated); err != nil {
				slog.Error("failed to persist policy profile", "name", effectiveProfile, "error", err)
			}
		}
	}

	// Return the updated profile.
	updated, _ := h.Policies.GetProfile(effectiveProfile)
	writeJSON(w, http.StatusOK, updated)
}

// --- Run Endpoints ---

// StartRun handles POST /api/v1/runs
func (h *Handlers) StartRun(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[run.StartRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.TaskID == "" {
		writeError(w, http.StatusBadRequest, "task_id is required")
		return
	}
	if req.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	result, err := h.Runtime.StartRun(r.Context(), &req)
	if err != nil {
		writeDomainError(w, err, "start run failed")
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

// GetRun handles GET /api/v1/runs/{id}
func (h *Handlers) GetRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	result, err := h.Runtime.GetRun(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "run not found")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// CancelRun handles POST /api/v1/runs/{id}/cancel
func (h *Handlers) CancelRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Runtime.CancelRun(r.Context(), id); err != nil {
		writeDomainError(w, err, "run not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// ListTaskRuns handles GET /api/v1/tasks/{id}/runs
func (h *Handlers) ListTaskRuns(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "id")
	runs, err := h.Runtime.ListRunsByTask(r.Context(), taskID)
	if err != nil {
		writeDomainError(w, err, "task not found")
		return
	}
	if runs == nil {
		runs = []run.Run{}
	}
	writeJSON(w, http.StatusOK, runs)
}

// ListRunEvents handles GET /api/v1/runs/{id}/events
func (h *Handlers) ListRunEvents(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	if h.Events == nil {
		writeError(w, http.StatusInternalServerError, "event store not configured")
		return
	}
	events, err := h.Events.LoadByRun(r.Context(), runID)
	if err != nil {
		writeDomainError(w, err, "run not found")
		return
	}
	if events == nil {
		events = []event.AgentEvent{}
	}
	writeJSON(w, http.StatusOK, events)
}

// ListActiveWork handles GET /api/v1/projects/{id}/active-work
func (h *Handlers) ListActiveWork(w http.ResponseWriter, r *http.Request) {
	if h.ActiveWork == nil {
		writeJSON(w, http.StatusOK, []task.ActiveWorkItem{})
		return
	}
	projectID := chi.URLParam(r, "id")
	items, err := h.ActiveWork.ListActiveWork(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	if items == nil {
		items = []task.ActiveWorkItem{}
	}
	writeJSON(w, http.StatusOK, items)
}

// ClaimTask handles POST /api/v1/tasks/{id}/claim

// ListActiveAgents handles GET /api/v1/projects/{id}/agents/active (Phase 23D War Room).
func (h *Handlers) ListActiveAgents(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	agents, err := h.Agents.List(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}

	active := make([]agent.Agent, 0)
	for i := range agents {
		if agents[i].Status == "running" {
			active = append(active, agents[i])
		}
	}
	writeJSON(w, http.StatusOK, active)
}

func (h *Handlers) ClaimTask(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "id")

	b, ok := readJSON[struct {
		AgentID string `json:"agent_id"`
	}](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}

	if !requireField(w, b.AgentID, "agent_id") {
		return
	}

	result, err := h.ActiveWork.ClaimTask(r.Context(), taskID, b.AgentID)
	if err != nil {
		writeDomainError(w, err, "task not found")
		return
	}
	if !result.Claimed {
		writeJSON(w, http.StatusConflict, result)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

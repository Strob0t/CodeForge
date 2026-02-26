package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/adapter/copilot"
	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	bp "github.com/Strob0t/CodeForge/internal/domain/branchprotection"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/cost"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/experience"
	lspDomain "github.com/Strob0t/CodeForge/internal/domain/lsp"
	"github.com/Strob0t/CodeForge/internal/domain/memory"
	"github.com/Strob0t/CodeForge/internal/domain/microagent"
	"github.com/Strob0t/CodeForge/internal/domain/mode"
	"github.com/Strob0t/CodeForge/internal/domain/pipeline"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/resource"
	"github.com/Strob0t/CodeForge/internal/domain/review"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/settings"
	"github.com/Strob0t/CodeForge/internal/domain/skill"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/domain/tenant"
	"github.com/Strob0t/CodeForge/internal/domain/vcsaccount"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/agentbackend"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
	"github.com/Strob0t/CodeForge/internal/port/specprovider"
	"github.com/Strob0t/CodeForge/internal/service"
)

const maxQueryLength = 2000
const maxRequestBodySize = 1 << 20 // 1 MB

// readJSON decodes a JSON request body with a size limit.
func readJSON[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var v T
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		if err.Error() == "http: request body too large" {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid request body")
		}
		return v, false
	}
	return v, true
}

// sanitizeName validates a name is safe for use in file paths.
// It rejects names containing path separators, dots-prefix, or other traversal patterns.
func sanitizeName(name string) error {
	if name == "" {
		return errors.New("name is required")
	}
	if len(name) > 128 {
		return errors.New("name too long (max 128 chars)")
	}
	if strings.ContainsAny(name, `/\`) {
		return errors.New("name must not contain path separators")
	}
	if strings.Contains(name, "..") {
		return errors.New("name must not contain '..'")
	}
	if name[0] == '.' {
		return errors.New("name must not start with '.'")
	}
	// Verify cleaned path stays within the expected directory
	cleaned := filepath.Clean(name)
	if cleaned != name {
		return errors.New("name contains invalid path characters")
	}
	return nil
}

// Handlers holds the HTTP handler dependencies.
type Handlers struct {
	Projects         *service.ProjectService
	Tasks            *service.TaskService
	Agents           *service.AgentService
	LiteLLM          *litellm.Client
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
	Conversations    *service.ConversationService
	LSP              *service.LSPService
	MCP              *service.MCPService
	PromptSections   *service.PromptSectionService
	Benchmarks       *service.BenchmarkService
	ReviewRouter     *service.ReviewRouterService
	ModelRegistry    *service.ModelRegistry
	Copilot          *copilot.Client
	Memory           *service.MemoryService
	ExperiencePool   *service.ExperiencePoolService
	Microagents      *service.MicroagentService
	Skills           *service.SkillService
}

// ListProjects handles GET /api/v1/projects
func (h *Handlers) ListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.Projects.List(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if projects == nil {
		projects = []project.Project{}
	}
	writeJSON(w, http.StatusOK, projects)
}

// GetProject handles GET /api/v1/projects/{id}
func (h *Handlers) GetProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := h.Projects.Get(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// CreateProject handles POST /api/v1/projects
func (h *Handlers) CreateProject(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[project.CreateRequest](w, r)
	if !ok {
		return
	}

	if err := project.ValidateCreateRequest(&req, gitprovider.Available()); err != nil {
		writeDomainError(w, err, "invalid project request")
		return
	}

	p, err := h.Projects.Create(r.Context(), &req)
	if err != nil {
		writeDomainError(w, err, "project creation failed")
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// DeleteProject handles DELETE /api/v1/projects/{id}
func (h *Handlers) DeleteProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Projects.Delete(r.Context(), id); err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UpdateProject handles PUT /api/v1/projects/{id}
func (h *Handlers) UpdateProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, ok := readJSON[project.UpdateRequest](w, r)
	if !ok {
		return
	}
	p, err := h.Projects.Update(r.Context(), id, req)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// ParseRepoURL handles POST /api/v1/parse-repo-url
func (h *Handlers) ParseRepoURL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	parsed, err := project.ParseRepoURL(req.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, parsed)
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

	req, ok := readJSON[task.CreateRequest](w, r)
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

// CloneProject handles POST /api/v1/projects/{id}/clone
func (h *Handlers) CloneProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tenantID := middleware.TenantIDFromContext(r.Context())

	// Optionally accept a branch in the request body.
	var body struct {
		Branch string `json:"branch"`
	}
	// Ignore decode errors — body is optional for backward compatibility.
	_ = json.NewDecoder(r.Body).Decode(&body)

	p, err := h.Projects.Clone(r.Context(), id, tenantID, body.Branch)
	if err != nil {
		writeDomainError(w, err, "clone failed")
		return
	}

	// Auto-trigger repo map generation after successful clone.
	if h.RepoMap != nil {
		go func() {
			if err := h.RepoMap.RequestGeneration(context.Background(), id, nil); err != nil {
				slog.Error("auto repomap generation failed", "project_id", id, "error", err)
			}
		}()
	}

	writeJSON(w, http.StatusOK, p)
}

// AdoptProject handles POST /api/v1/projects/{id}/adopt
func (h *Handlers) AdoptProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, ok := readJSON[project.AdoptRequest](w, r)
	if !ok {
		return
	}

	// Validate the path is an absolute path and exists
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	cleanPath := filepath.Clean(req.Path)
	if !filepath.IsAbs(cleanPath) {
		writeError(w, http.StatusBadRequest, "path must be absolute")
		return
	}
	// Prevent traversal: path must resolve to itself after cleaning
	if cleanPath != req.Path && cleanPath+"/" != req.Path {
		writeError(w, http.StatusBadRequest, "path contains invalid characters")
		return
	}

	p, err := h.Projects.Adopt(r.Context(), id, cleanPath)
	if err != nil {
		writeDomainError(w, err, "adopt failed")
		return
	}

	writeJSON(w, http.StatusOK, p)
}

// SetupProject handles POST /api/v1/projects/{id}/setup
// It chains clone, stack detection, and spec import in a single request.
func (h *Handlers) SetupProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tenantID := middleware.TenantIDFromContext(r.Context())

	// Optionally accept a branch in the request body.
	var body struct {
		Branch string `json:"branch"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	result, err := h.Projects.SetupProject(r.Context(), id, tenantID, body.Branch)
	if err != nil {
		writeDomainError(w, err, "setup failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// GetWorkspaceInfo handles GET /api/v1/projects/{id}/workspace
func (h *Handlers) GetWorkspaceInfo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	info, err := h.Projects.WorkspaceHealth(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "workspace info failed")
		return
	}

	writeJSON(w, http.StatusOK, info)
}

// DetectProjectStack handles GET /api/v1/projects/{id}/detect-stack
func (h *Handlers) DetectProjectStack(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	result, err := h.Projects.DetectStack(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "stack detection failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// DetectStackByPath handles POST /api/v1/detect-stack
func (h *Handlers) DetectStackByPath(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[struct {
		Path string `json:"path"`
	}](w, r)
	if !ok {
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	cleanPath := filepath.Clean(req.Path)
	if !filepath.IsAbs(cleanPath) {
		writeError(w, http.StatusBadRequest, "path must be absolute")
		return
	}
	result, err := h.Projects.DetectStackByPath(r.Context(), cleanPath)
	if err != nil {
		writeDomainError(w, err, "stack detection failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ProjectGitStatus handles GET /api/v1/projects/{id}/git/status
func (h *Handlers) ProjectGitStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	status, err := h.Projects.Status(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// PullProject handles POST /api/v1/projects/{id}/git/pull
func (h *Handlers) PullProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Projects.Pull(r.Context(), id); err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ListProjectBranches handles GET /api/v1/projects/{id}/git/branches
func (h *Handlers) ListProjectBranches(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	branches, err := h.Projects.ListBranches(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, branches)
}

// CheckoutBranch handles POST /api/v1/projects/{id}/git/checkout
func (h *Handlers) CheckoutBranch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	req, ok := readJSON[struct {
		Branch string `json:"branch"`
	}](w, r)
	if !ok {
		return
	}
	if req.Branch == "" {
		writeError(w, http.StatusBadRequest, "branch is required")
		return
	}

	if err := h.Projects.Checkout(r.Context(), id, req.Branch); err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "branch": req.Branch})
}

// ListRemoteBranches handles GET /api/v1/projects/remote-branches?url=<repo-url>
// It runs `git ls-remote --heads <url>` and returns the branch names.
func (h *Handlers) ListRemoteBranches(w http.ResponseWriter, r *http.Request) {
	repoURL := r.URL.Query().Get("url")
	if repoURL == "" {
		writeError(w, http.StatusBadRequest, "url query parameter is required")
		return
	}

	// Basic validation: reject obviously invalid URLs (must contain a host-like segment).
	if !strings.Contains(repoURL, "/") {
		writeError(w, http.StatusBadRequest, "invalid repository URL")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--heads", repoURL) //nolint:gosec // repoURL is validated above as a safe git URL.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		slog.Warn("git ls-remote failed", "url", repoURL, "error", err, "stderr", stderr.String())
		writeError(w, http.StatusBadGateway, "failed to list remote branches")
		return
	}

	var branches []string
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: <sha>\trefs/heads/<branch-name>
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		ref := parts[1]
		branch := strings.TrimPrefix(ref, "refs/heads/")
		if branch != ref {
			branches = append(branches, branch)
		}
	}

	if branches == nil {
		branches = []string{}
	}
	writeJSON(w, http.StatusOK, map[string][]string{"branches": branches})
}

// ListAgents handles GET /api/v1/projects/{id}/agents
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
	}](w, r)
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
		writeError(w, http.StatusBadRequest, err.Error())
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
	}](w, r)
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
	}](w, r)
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
func (h *Handlers) ListLLMModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.LiteLLM.ListModels(r.Context())
	if err != nil {
		slog.Error("litellm unavailable", "error", err)
		writeError(w, http.StatusBadGateway, "LLM service unavailable")
		return
	}
	if models == nil {
		models = []litellm.Model{}
	}
	writeJSON(w, http.StatusOK, models)
}

// AddLLMModel handles POST /api/v1/llm/models
func (h *Handlers) AddLLMModel(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[litellm.AddModelRequest](w, r)
	if !ok {
		return
	}
	if req.ModelName == "" {
		writeError(w, http.StatusBadRequest, "model_name is required")
		return
	}

	if err := h.LiteLLM.AddModel(r.Context(), req); err != nil {
		slog.Error("litellm request failed", "error", err)
		writeError(w, http.StatusBadGateway, "LLM service error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok", "model": req.ModelName})
}

// DeleteLLMModel handles POST /api/v1/llm/models/delete
func (h *Handlers) DeleteLLMModel(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[struct {
		ID string `json:"id"`
	}](w, r)
	if !ok {
		return
	}
	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	if err := h.LiteLLM.DeleteModel(r.Context(), req.ID); err != nil {
		slog.Error("litellm request failed", "error", err)
		writeError(w, http.StatusBadGateway, "LLM service error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// LLMHealth handles GET /api/v1/llm/health
func (h *Handlers) LLMHealth(w http.ResponseWriter, r *http.Request) {
	healthy, err := h.LiteLLM.Health(r.Context())
	status := "healthy"
	if !healthy || err != nil {
		status = "unhealthy"
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}

// DiscoverLLMModels handles GET /api/v1/llm/discover
// It queries LiteLLM and optionally Ollama to discover all available models.
func (h *Handlers) DiscoverLLMModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Discover models from LiteLLM.
	models, err := h.LiteLLM.DiscoverModels(ctx)
	if err != nil {
		slog.Error("litellm discovery failed", "error", err)
		writeError(w, http.StatusBadGateway, "LLM discovery failed: "+err.Error())
		return
	}

	// Discover Ollama models if OLLAMA_BASE_URL is set.
	ollamaURL := os.Getenv("OLLAMA_BASE_URL")
	if ollamaURL != "" {
		ollamaModels, err := h.LiteLLM.DiscoverOllamaModels(ctx, ollamaURL)
		if err != nil {
			slog.Warn("ollama discovery failed", "error", err)
			// Non-fatal: continue with LiteLLM models only.
		} else {
			models = append(models, ollamaModels...)
		}
	}

	if models == nil {
		models = []litellm.DiscoveredModel{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"models":     models,
		"count":      len(models),
		"ollama_url": ollamaURL,
	})
}

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

	call, ok := readJSON[policy.ToolCall](w, r)
	if !ok {
		return
	}
	if call.Tool == "" {
		writeError(w, http.StatusBadRequest, "tool is required")
		return
	}

	result, err := h.Policies.EvaluateWithReason(r.Context(), name, call)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// CreatePolicyProfile handles POST /api/v1/policies
func (h *Handlers) CreatePolicyProfile(w http.ResponseWriter, r *http.Request) {
	profile, ok := readJSON[policy.PolicyProfile](w, r)
	if !ok {
		return
	}
	if err := sanitizeName(profile.Name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.Policies.SaveProfile(&profile); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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

// --- Run Endpoints ---

// StartRun handles POST /api/v1/runs
func (h *Handlers) StartRun(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[run.StartRequest](w, r)
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

// --- Execution Plan Endpoints ---

// CreatePlan handles POST /api/v1/projects/{id}/plans
func (h *Handlers) CreatePlan(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	req, ok := readJSON[plan.CreatePlanRequest](w, r)
	if !ok {
		return
	}
	req.ProjectID = projectID

	p, err := h.Orchestrator.CreatePlan(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// ListPlans handles GET /api/v1/projects/{id}/plans
func (h *Handlers) ListPlans(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	plans, err := h.Orchestrator.ListPlans(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	if plans == nil {
		plans = []plan.ExecutionPlan{}
	}
	writeJSON(w, http.StatusOK, plans)
}

// GetPlan handles GET /api/v1/plans/{id}
func (h *Handlers) GetPlan(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := h.Orchestrator.GetPlan(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "plan not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// StartPlan handles POST /api/v1/plans/{id}/start
func (h *Handlers) StartPlan(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := h.Orchestrator.StartPlan(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// CancelPlan handles POST /api/v1/plans/{id}/cancel
func (h *Handlers) CancelPlan(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Orchestrator.CancelPlan(r.Context(), id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// EvaluateStep handles POST /api/v1/plans/{id}/steps/{stepId}/evaluate
// It manually triggers the review router for a specific step.
func (h *Handlers) EvaluateStep(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	stepID := chi.URLParam(r, "stepId")

	if h.ReviewRouter == nil {
		writeError(w, http.StatusServiceUnavailable, "review router not configured")
		return
	}

	p, err := h.Orchestrator.GetPlan(r.Context(), planID)
	if err != nil {
		writeDomainError(w, err, "plan not found")
		return
	}

	var step *plan.Step
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			step = &p.Steps[i]
			break
		}
	}
	if step == nil {
		writeError(w, http.StatusNotFound, "step not found in plan")
		return
	}

	// Fetch task description for context
	taskDesc := ""
	t, taskErr := h.Tasks.Get(r.Context(), step.TaskID)
	if taskErr == nil {
		taskDesc = t.Prompt
		if taskDesc == "" {
			taskDesc = t.Title
		}
	}

	decision, err := h.ReviewRouter.Evaluate(r.Context(), step, taskDesc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("review evaluation failed: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, decision)
}

// GetPlanGraph handles GET /api/v1/plans/{id}/graph
// It returns the execution plan as a DAG in a frontend-friendly format.
func (h *Handlers) GetPlanGraph(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")

	p, err := h.Orchestrator.GetPlan(r.Context(), planID)
	if err != nil {
		writeDomainError(w, err, "plan not found")
		return
	}

	type GraphNode struct {
		ID        string   `json:"id"`
		TaskID    string   `json:"task_id"`
		AgentID   string   `json:"agent_id"`
		ModeID    string   `json:"mode_id,omitempty"`
		Status    string   `json:"status"`
		RunID     string   `json:"run_id,omitempty"`
		Round     int      `json:"round"`
		Error     string   `json:"error,omitempty"`
		DependsOn []string `json:"depends_on,omitempty"`
	}

	type GraphEdge struct {
		From     string `json:"from"`
		To       string `json:"to"`
		Protocol string `json:"protocol"`
	}

	type PlanGraph struct {
		PlanID   string      `json:"plan_id"`
		Name     string      `json:"name"`
		Protocol string      `json:"protocol"`
		Status   string      `json:"status"`
		Nodes    []GraphNode `json:"nodes"`
		Edges    []GraphEdge `json:"edges"`
	}

	graph := PlanGraph{
		PlanID:   p.ID,
		Name:     p.Name,
		Protocol: string(p.Protocol),
		Status:   string(p.Status),
		Nodes:    make([]GraphNode, 0, len(p.Steps)),
		Edges:    make([]GraphEdge, 0),
	}

	for i := range p.Steps {
		step := &p.Steps[i]
		graph.Nodes = append(graph.Nodes, GraphNode{
			ID:        step.ID,
			TaskID:    step.TaskID,
			AgentID:   step.AgentID,
			ModeID:    step.ModeID,
			Status:    string(step.Status),
			RunID:     step.RunID,
			Round:     step.Round,
			Error:     step.Error,
			DependsOn: step.DependsOn,
		})

		for _, dep := range step.DependsOn {
			graph.Edges = append(graph.Edges, GraphEdge{
				From:     dep,
				To:       step.ID,
				Protocol: string(p.Protocol),
			})
		}
	}

	writeJSON(w, http.StatusOK, graph)
}

// --- Feature Decomposition (Meta-Agent) ---

// DecomposeFeature handles POST /api/v1/projects/{id}/decompose
func (h *Handlers) DecomposeFeature(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	req, ok := readJSON[plan.DecomposeRequest](w, r)
	if !ok {
		return
	}
	req.ProjectID = projectID

	p, err := h.MetaAgent.DecomposeFeature(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// --- Agent Teams ---

// ListTeams handles GET /api/v1/projects/{id}/teams
func (h *Handlers) ListTeams(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	teams, err := h.PoolManager.ListTeams(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	if teams == nil {
		teams = []agent.Team{}
	}
	writeJSON(w, http.StatusOK, teams)
}

// CreateTeam handles POST /api/v1/projects/{id}/teams
func (h *Handlers) CreateTeam(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	req, ok := readJSON[agent.CreateTeamRequest](w, r)
	if !ok {
		return
	}
	req.ProjectID = projectID

	team, err := h.PoolManager.CreateTeam(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, team)
}

// GetTeam handles GET /api/v1/teams/{id}
func (h *Handlers) GetTeam(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	team, err := h.PoolManager.GetTeam(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "team not found")
		return
	}
	writeJSON(w, http.StatusOK, team)
}

// DeleteTeam handles DELETE /api/v1/teams/{id}
func (h *Handlers) DeleteTeam(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.PoolManager.DeleteTeam(r.Context(), id); err != nil {
		writeDomainError(w, err, "team not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Context-Optimized Feature Planning ---

// PlanFeature handles POST /api/v1/projects/{id}/plan-feature
func (h *Handlers) PlanFeature(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	req, ok := readJSON[plan.PlanFeatureRequest](w, r)
	if !ok {
		return
	}
	req.ProjectID = projectID

	p, err := h.TaskPlanner.PlanFeature(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// --- Context Pack Endpoints ---

// GetContextPack handles GET /api/v1/tasks/{id}/context
func (h *Handlers) GetContextPack(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "id")
	pack, err := h.ContextOptimizer.GetPackByTask(r.Context(), taskID)
	if err != nil {
		writeDomainError(w, err, "context pack not found")
		return
	}
	writeJSON(w, http.StatusOK, pack)
}

// BuildContextPack handles POST /api/v1/tasks/{id}/context
func (h *Handlers) BuildContextPack(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "id")

	req, ok := readJSON[struct {
		ProjectID string `json:"project_id"`
		TeamID    string `json:"team_id"`
	}](w, r)
	if !ok {
		return
	}
	if req.ProjectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	pack, err := h.ContextOptimizer.BuildContextPack(r.Context(), taskID, req.ProjectID, req.TeamID)
	if err != nil {
		writeDomainError(w, err, "task or project not found")
		return
	}
	writeJSON(w, http.StatusCreated, pack)
}

// --- Shared Context Endpoints ---

// GetSharedContext handles GET /api/v1/teams/{id}/shared-context
func (h *Handlers) GetSharedContext(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")
	sc, err := h.SharedContext.Get(r.Context(), teamID)
	if err != nil {
		writeDomainError(w, err, "shared context not found")
		return
	}
	writeJSON(w, http.StatusOK, sc)
}

// AddSharedContextItem handles POST /api/v1/teams/{id}/shared-context
func (h *Handlers) AddSharedContextItem(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")

	req, ok := readJSON[cfcontext.AddSharedItemRequest](w, r)
	if !ok {
		return
	}
	req.TeamID = teamID

	item, err := h.SharedContext.AddItem(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

// --- Mode Endpoints ---

// ListModes handles GET /api/v1/modes
func (h *Handlers) ListModes(w http.ResponseWriter, _ *http.Request) {
	modes := h.Modes.List()
	if modes == nil {
		modes = []mode.Mode{}
	}
	writeJSON(w, http.StatusOK, modes)
}

// GetMode handles GET /api/v1/modes/{id}
func (h *Handlers) GetMode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	m, err := h.Modes.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "mode not found")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// ListScenarios handles GET /api/v1/modes/scenarios
func (h *Handlers) ListScenarios(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, mode.ValidScenarios)
}

// CreateMode handles POST /api/v1/modes
func (h *Handlers) CreateMode(w http.ResponseWriter, r *http.Request) {
	m, ok := readJSON[mode.Mode](w, r)
	if !ok {
		return
	}
	if err := h.Modes.Register(&m); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// UpdateMode handles PUT /api/v1/modes/{id}
func (h *Handlers) UpdateMode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	m, ok := readJSON[mode.Mode](w, r)
	if !ok {
		return
	}
	if err := h.Modes.Update(id, &m); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, _ := h.Modes.Get(id)
	writeJSON(w, http.StatusOK, updated)
}

// --- RepoMap Endpoints ---

// GetRepoMap handles GET /api/v1/projects/{id}/repomap
func (h *Handlers) GetRepoMap(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	m, err := h.RepoMap.Get(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "repo map not found")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// GenerateRepoMap handles POST /api/v1/projects/{id}/repomap
func (h *Handlers) GenerateRepoMap(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	var req struct {
		ActiveFiles []string `json:"active_files"`
	}
	// Body is optional; empty body is fine.
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := h.RepoMap.RequestGeneration(r.Context(), projectID, req.ActiveFiles); err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "generating"})
}

// --- Retrieval Endpoints ---

// IndexProject handles POST /api/v1/projects/{id}/index
func (h *Handlers) IndexProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	var req struct {
		EmbeddingModel string `json:"embedding_model"`
	}
	// Body is optional; empty body is fine.
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := h.Retrieval.RequestIndex(r.Context(), projectID, "", req.EmbeddingModel); err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "building"})
}

// GetIndexStatus handles GET /api/v1/projects/{id}/index
func (h *Handlers) GetIndexStatus(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	info := h.Retrieval.GetIndexStatus(projectID)
	if info == nil {
		writeError(w, http.StatusNotFound, "no index found for project")
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// SearchProject handles POST /api/v1/projects/{id}/search
func (h *Handlers) SearchProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	req, ok := readJSON[struct {
		Query          string  `json:"query"`
		TopK           int     `json:"top_k"`
		BM25Weight     float64 `json:"bm25_weight"`
		SemanticWeight float64 `json:"semantic_weight"`
	}](w, r)
	if !ok {
		return
	}
	if req.Query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}
	if len(req.Query) > maxQueryLength {
		writeError(w, http.StatusBadRequest, "query exceeds maximum length of 2000 characters")
		return
	}

	// Clamp top_k to safe bounds.
	topK := req.TopK
	if topK <= 0 {
		topK = 20
	} else if topK > 500 {
		topK = 500
	}

	result, err := h.Retrieval.SearchSync(r.Context(), projectID, req.Query, topK, req.BM25Weight, req.SemanticWeight)
	if err != nil {
		slog.Error("search timed out", "error", err)
		writeError(w, http.StatusGatewayTimeout, "search timed out")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// --- Retrieval Sub-Agent Endpoints (Phase 6C) ---

// AgentSearchProject handles POST /api/v1/projects/{id}/search/agent
func (h *Handlers) AgentSearchProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	req, ok := readJSON[struct {
		Query      string `json:"query"`
		TopK       int    `json:"top_k"`
		MaxQueries int    `json:"max_queries"`
		Model      string `json:"model"`
		Rerank     *bool  `json:"rerank"` // pointer to distinguish absent (use config default) from explicit false
	}](w, r)
	if !ok {
		return
	}
	if req.Query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}
	if len(req.Query) > maxQueryLength {
		writeError(w, http.StatusBadRequest, "query exceeds maximum length of 2000 characters")
		return
	}

	// Apply defaults from config, clamp to safe bounds.
	defaultModel, defaultMaxQueries, defaultRerank := h.Retrieval.SubAgentDefaults()
	topK := req.TopK
	if topK <= 0 {
		topK = 20
	} else if topK > 500 {
		topK = 500
	}
	maxQueries := req.MaxQueries
	if maxQueries <= 0 {
		maxQueries = defaultMaxQueries
	} else if maxQueries > 20 {
		maxQueries = 20
	}
	model := req.Model
	if model == "" {
		model = defaultModel
	}
	rerank := defaultRerank
	if req.Rerank != nil {
		rerank = *req.Rerank
	}

	// Look up project-specific expansion prompt from config.
	var expansionPrompt string
	if proj, projErr := h.Projects.Get(r.Context(), projectID); projErr == nil && proj.Config != nil {
		expansionPrompt = proj.Config["expansion_prompt"]
	}

	result, err := h.Retrieval.SubAgentSearchSync(r.Context(), projectID, req.Query, topK, maxQueries, model, rerank, expansionPrompt)
	if err != nil {
		slog.Error("search timed out", "error", err)
		writeError(w, http.StatusGatewayTimeout, "search timed out")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// --- GraphRAG Endpoints (Phase 6D) ---

// BuildGraph handles POST /api/v1/projects/{id}/graph/build
func (h *Handlers) BuildGraph(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	proj, err := h.Projects.Get(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}

	if err := h.Graph.RequestBuild(r.Context(), projectID, proj.WorkspacePath); err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "building"})
}

// GetGraphStatus handles GET /api/v1/projects/{id}/graph/status
func (h *Handlers) GetGraphStatus(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	info := h.Graph.GetStatus(projectID)
	if info == nil {
		writeError(w, http.StatusNotFound, "no graph found for project")
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// SearchGraph handles POST /api/v1/projects/{id}/graph/search
func (h *Handlers) SearchGraph(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	req, ok := readJSON[struct {
		SeedSymbols []string `json:"seed_symbols"`
		MaxHops     int      `json:"max_hops"`
		TopK        int      `json:"top_k"`
	}](w, r)
	if !ok {
		return
	}
	if len(req.SeedSymbols) == 0 {
		writeError(w, http.StatusBadRequest, "seed_symbols is required")
		return
	}

	maxHops := req.MaxHops
	if maxHops <= 0 {
		maxHops = 2
	} else if maxHops > 10 {
		maxHops = 10
	}
	topK := req.TopK
	if topK <= 0 {
		topK = 10
	} else if topK > 500 {
		topK = 500
	}

	result, err := h.Graph.SearchSync(r.Context(), projectID, req.SeedSymbols, maxHops, topK)
	if err != nil {
		slog.Error("search timed out", "error", err)
		writeError(w, http.StatusGatewayTimeout, "search timed out")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// --- Cost Endpoints ---

// GlobalCostSummary handles GET /api/v1/costs
func (h *Handlers) GlobalCostSummary(w http.ResponseWriter, r *http.Request) {
	summaries, err := h.Cost.GlobalSummary(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if summaries == nil {
		summaries = []cost.ProjectSummary{}
	}
	writeJSON(w, http.StatusOK, summaries)
}

// ProjectCostSummary handles GET /api/v1/projects/{id}/costs
func (h *Handlers) ProjectCostSummary(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	summary, err := h.Cost.ProjectSummary(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// ProjectCostByModel handles GET /api/v1/projects/{id}/costs/by-model
func (h *Handlers) ProjectCostByModel(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	models, err := h.Cost.ByModel(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	if models == nil {
		models = []cost.ModelSummary{}
	}
	writeJSON(w, http.StatusOK, models)
}

// ProjectCostTimeSeries handles GET /api/v1/projects/{id}/costs/daily
func (h *Handlers) ProjectCostTimeSeries(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = parsed
		}
	}
	series, err := h.Cost.TimeSeries(r.Context(), projectID, days)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	if series == nil {
		series = []cost.DailyCost{}
	}
	writeJSON(w, http.StatusOK, series)
}

// ProjectRecentRuns handles GET /api/v1/projects/{id}/costs/runs
func (h *Handlers) ProjectRecentRuns(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	runs, err := h.Cost.RecentRuns(r.Context(), projectID, limit)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	if runs == nil {
		runs = []run.Run{}
	}
	writeJSON(w, http.StatusOK, runs)
}

// ProjectCostByTool handles GET /api/v1/projects/{id}/costs/by-tool
func (h *Handlers) ProjectCostByTool(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	tools, err := h.Cost.ByTool(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	if tools == nil {
		tools = []cost.ToolSummary{}
	}
	writeJSON(w, http.StatusOK, tools)
}

// RunCostByTool handles GET /api/v1/runs/{id}/costs/by-tool
func (h *Handlers) RunCostByTool(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	tools, err := h.Cost.ByToolForRun(r.Context(), runID)
	if err != nil {
		writeDomainError(w, err, "run not found")
		return
	}
	if tools == nil {
		tools = []cost.ToolSummary{}
	}
	writeJSON(w, http.StatusOK, tools)
}

// --- Roadmap Endpoints (Phase 8) ---

// GetProjectRoadmap handles GET /api/v1/projects/{id}/roadmap
func (h *Handlers) GetProjectRoadmap(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	rm, err := h.Roadmap.GetByProject(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "roadmap not found")
		return
	}
	writeJSON(w, http.StatusOK, rm)
}

// CreateProjectRoadmap handles POST /api/v1/projects/{id}/roadmap
func (h *Handlers) CreateProjectRoadmap(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	req, ok := readJSON[roadmap.CreateRoadmapRequest](w, r)
	if !ok {
		return
	}
	req.ProjectID = projectID

	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	rm, err := h.Roadmap.Create(r.Context(), req)
	if err != nil {
		writeDomainError(w, err, "roadmap creation failed")
		return
	}
	writeJSON(w, http.StatusCreated, rm)
}

// UpdateProjectRoadmap handles PUT /api/v1/projects/{id}/roadmap
func (h *Handlers) UpdateProjectRoadmap(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	rm, err := h.Roadmap.GetByProject(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "roadmap not found")
		return
	}

	req, ok := readJSON[struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Status      string `json:"status"`
		Version     int    `json:"version"`
	}](w, r)
	if !ok {
		return
	}

	if req.Title != "" {
		rm.Title = req.Title
	}
	if req.Description != "" {
		rm.Description = req.Description
	}
	if req.Status != "" {
		rm.Status = roadmap.RoadmapStatus(req.Status)
	}
	if req.Version > 0 {
		rm.Version = req.Version
	}

	if err := h.Roadmap.Update(r.Context(), rm); err != nil {
		writeDomainError(w, err, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, rm)
}

// DeleteProjectRoadmap handles DELETE /api/v1/projects/{id}/roadmap
func (h *Handlers) DeleteProjectRoadmap(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	rm, err := h.Roadmap.GetByProject(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "roadmap not found")
		return
	}

	if err := h.Roadmap.Delete(r.Context(), rm.ID); err != nil {
		writeDomainError(w, err, "roadmap not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetRoadmapAI handles GET /api/v1/projects/{id}/roadmap/ai
func (h *Handlers) GetRoadmapAI(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "markdown"
	}

	view, err := h.Roadmap.AIView(r.Context(), projectID, format)
	if err != nil {
		writeDomainError(w, err, "roadmap not found")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// DetectRoadmap handles POST /api/v1/projects/{id}/roadmap/detect
func (h *Handlers) DetectRoadmap(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	result, err := h.Roadmap.AutoDetect(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// CreateMilestone handles POST /api/v1/projects/{id}/roadmap/milestones
func (h *Handlers) CreateMilestone(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	rm, err := h.Roadmap.GetByProject(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "roadmap not found")
		return
	}

	req, ok := readJSON[roadmap.CreateMilestoneRequest](w, r)
	if !ok {
		return
	}
	req.RoadmapID = rm.ID

	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	m, err := h.Roadmap.CreateMilestone(r.Context(), req)
	if err != nil {
		writeDomainError(w, err, "milestone creation failed")
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// UpdateMilestone handles PUT /api/v1/milestones/{id}
func (h *Handlers) UpdateMilestone(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	req, ok := readJSON[struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Status      string `json:"status"`
		SortOrder   *int   `json:"sort_order"`
		Version     int    `json:"version"`
	}](w, r)
	if !ok {
		return
	}

	m, err := h.Roadmap.GetMilestone(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "milestone not found")
		return
	}

	if req.Title != "" {
		m.Title = req.Title
	}
	if req.Description != "" {
		m.Description = req.Description
	}
	if req.Status != "" {
		m.Status = roadmap.RoadmapStatus(req.Status)
	}
	if req.SortOrder != nil {
		m.SortOrder = *req.SortOrder
	}
	if req.Version > 0 {
		m.Version = req.Version
	}

	if err := h.Roadmap.UpdateMilestone(r.Context(), m); err != nil {
		writeDomainError(w, err, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// DeleteMilestone handles DELETE /api/v1/milestones/{id}
func (h *Handlers) DeleteMilestone(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Roadmap.DeleteMilestone(r.Context(), id); err != nil {
		writeDomainError(w, err, "milestone not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CreateFeature handles POST /api/v1/milestones/{id}/features
func (h *Handlers) CreateFeature(w http.ResponseWriter, r *http.Request) {
	milestoneID := chi.URLParam(r, "id")

	req, ok := readJSON[roadmap.CreateFeatureRequest](w, r)
	if !ok {
		return
	}
	req.MilestoneID = milestoneID

	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	f, err := h.Roadmap.CreateFeature(r.Context(), &req)
	if err != nil {
		writeDomainError(w, err, "milestone not found")
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

// UpdateFeature handles PUT /api/v1/features/{id}
func (h *Handlers) UpdateFeature(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	req, ok := readJSON[struct {
		Title       string            `json:"title"`
		Description string            `json:"description"`
		Status      string            `json:"status"`
		Labels      []string          `json:"labels"`
		SpecRef     string            `json:"spec_ref"`
		ExternalIDs map[string]string `json:"external_ids"`
		SortOrder   *int              `json:"sort_order"`
		Version     int               `json:"version"`
	}](w, r)
	if !ok {
		return
	}

	f, err := h.Roadmap.GetFeature(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "feature not found")
		return
	}

	if req.Title != "" {
		f.Title = req.Title
	}
	if req.Description != "" {
		f.Description = req.Description
	}
	if req.Status != "" {
		f.Status = roadmap.FeatureStatus(req.Status)
	}
	if req.Labels != nil {
		f.Labels = req.Labels
	}
	if req.SpecRef != "" {
		f.SpecRef = req.SpecRef
	}
	if req.ExternalIDs != nil {
		f.ExternalIDs = req.ExternalIDs
	}
	if req.SortOrder != nil {
		f.SortOrder = *req.SortOrder
	}
	if req.Version > 0 {
		f.Version = req.Version
	}

	if err := h.Roadmap.UpdateFeature(r.Context(), f); err != nil {
		writeDomainError(w, err, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// DeleteFeature handles DELETE /api/v1/features/{id}
func (h *Handlers) DeleteFeature(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Roadmap.DeleteFeature(r.Context(), id); err != nil {
		writeDomainError(w, err, "feature not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Spec/PM Import Endpoints (Phase 9A) ---

// ImportSpecs handles POST /api/v1/projects/{id}/roadmap/import
func (h *Handlers) ImportSpecs(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	result, err := h.Roadmap.ImportSpecs(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "import failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ImportPMItems handles POST /api/v1/projects/{id}/roadmap/import/pm
func (h *Handlers) ImportPMItems(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	req, ok := readJSON[struct {
		Provider   string `json:"provider"`
		ProjectRef string `json:"project_ref"`
	}](w, r)
	if !ok {
		return
	}
	if req.Provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}
	if req.ProjectRef == "" {
		writeError(w, http.StatusBadRequest, "project_ref is required")
		return
	}

	result, err := h.Roadmap.ImportPMItems(r.Context(), projectID, req.Provider, req.ProjectRef)
	if err != nil {
		writeDomainError(w, err, "PM import failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// SyncToSpecFile handles POST /api/v1/projects/{id}/roadmap/sync-to-file
func (h *Handlers) SyncToSpecFile(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	if err := h.Roadmap.SyncToSpecFile(r.Context(), projectID); err != nil {
		writeDomainError(w, err, "sync to spec file failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "synced"})
}

// ListSpecProviders handles GET /api/v1/providers/spec
func (h *Handlers) ListSpecProviders(w http.ResponseWriter, _ *http.Request) {
	names := specprovider.Available()
	type providerInfo struct {
		Name         string                    `json:"name"`
		Capabilities specprovider.Capabilities `json:"capabilities"`
	}

	providers := make([]providerInfo, 0, len(names))
	for _, name := range names {
		p, err := specprovider.New(name, nil)
		if err != nil {
			continue
		}
		providers = append(providers, providerInfo{
			Name:         p.Name(),
			Capabilities: p.Capabilities(),
		})
	}
	writeJSON(w, http.StatusOK, providers)
}

// ListPMProviders handles GET /api/v1/providers/pm
func (h *Handlers) ListPMProviders(w http.ResponseWriter, _ *http.Request) {
	names := pmprovider.Available()
	type providerInfo struct {
		Name         string                  `json:"name"`
		Capabilities pmprovider.Capabilities `json:"capabilities"`
	}

	providers := make([]providerInfo, 0, len(names))
	for _, name := range names {
		p, err := pmprovider.New(name, nil)
		if err != nil {
			continue
		}
		providers = append(providers, providerInfo{
			Name:         p.Name(),
			Capabilities: p.Capabilities(),
		})
	}
	writeJSON(w, http.StatusOK, providers)
}

// --- Trajectory Endpoints (Phase 8) ---

// GetTrajectory handles GET /api/v1/runs/{id}/trajectory
func (h *Handlers) GetTrajectory(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	if h.Events == nil {
		writeError(w, http.StatusInternalServerError, "event store not configured")
		return
	}

	filter := eventstore.TrajectoryFilter{}

	if types := r.URL.Query().Get("types"); types != "" {
		for _, t := range strings.Split(types, ",") {
			filter.Types = append(filter.Types, event.Type(strings.TrimSpace(t)))
		}
	}

	cursor := r.URL.Query().Get("cursor")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 500 {
		limit = 500
	}

	page, err := h.Events.LoadTrajectory(r.Context(), runID, filter, cursor, limit)
	if err != nil {
		writeDomainError(w, err, "run not found")
		return
	}

	// Include stats in the response.
	stats, err := h.Events.TrajectoryStats(r.Context(), runID)
	if err != nil {
		writeDomainError(w, err, "run not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"events":   page.Events,
		"cursor":   page.Cursor,
		"has_more": page.HasMore,
		"total":    page.Total,
		"stats":    stats,
	})
}

// ExportTrajectory handles GET /api/v1/runs/{id}/trajectory/export
func (h *Handlers) ExportTrajectory(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"trajectory-%s.json\"", runID))
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(events)
}

// GetMilestone (direct access) handles GET /api/v1/milestones/{id}
func (h *Handlers) GetMilestone(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	m, err := h.Roadmap.GetMilestone(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "milestone not found")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// GetFeature (direct access) handles GET /api/v1/features/{id}
func (h *Handlers) GetFeature(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	f, err := h.Roadmap.GetFeature(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "feature not found")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// --- Tenant Endpoints ---

// ListTenants handles GET /api/v1/tenants
func (h *Handlers) ListTenants(w http.ResponseWriter, r *http.Request) {
	tenants, err := h.Tenants.List(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if tenants == nil {
		tenants = []tenant.Tenant{}
	}
	writeJSON(w, http.StatusOK, tenants)
}

// CreateTenant handles POST /api/v1/tenants
func (h *Handlers) CreateTenant(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[tenant.CreateRequest](w, r)
	if !ok {
		return
	}

	t, err := h.Tenants.Create(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// GetTenant handles GET /api/v1/tenants/{id}
func (h *Handlers) GetTenant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := h.Tenants.Get(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "tenant not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// UpdateTenant handles PUT /api/v1/tenants/{id}
func (h *Handlers) UpdateTenant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	req, ok := readJSON[tenant.UpdateRequest](w, r)
	if !ok {
		return
	}

	t, err := h.Tenants.Update(r.Context(), id, req)
	if err != nil {
		writeDomainError(w, err, "tenant not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// --- Branch Protection Rules ---

// ListBranchProtectionRules handles GET /api/v1/projects/{id}/branch-rules
func (h *Handlers) ListBranchProtectionRules(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	rules, err := h.BranchProtection.ListRules(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	if rules == nil {
		rules = []bp.ProtectionRule{}
	}
	writeJSON(w, http.StatusOK, rules)
}

// CreateBranchProtectionRule handles POST /api/v1/projects/{id}/branch-rules
func (h *Handlers) CreateBranchProtectionRule(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	req, ok := readJSON[bp.CreateRuleRequest](w, r)
	if !ok {
		return
	}
	req.ProjectID = projectID

	rule, err := h.BranchProtection.CreateRule(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, rule)
}

// GetBranchProtectionRule handles GET /api/v1/branch-rules/{id}
func (h *Handlers) GetBranchProtectionRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rule, err := h.BranchProtection.GetRule(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "branch protection rule not found")
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

// UpdateBranchProtectionRule handles PUT /api/v1/branch-rules/{id}
func (h *Handlers) UpdateBranchProtectionRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	req, ok := readJSON[bp.UpdateRuleRequest](w, r)
	if !ok {
		return
	}

	rule, err := h.BranchProtection.UpdateRule(r.Context(), id, req)
	if err != nil {
		writeDomainError(w, err, "branch protection rule not found")
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

// DeleteBranchProtectionRule handles DELETE /api/v1/branch-rules/{id}
func (h *Handlers) DeleteBranchProtectionRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.BranchProtection.DeleteRule(r.Context(), id); err != nil {
		writeDomainError(w, err, "branch protection rule not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Replay / Audit Trail ---

// ListRunCheckpoints handles GET /api/v1/runs/{id}/checkpoints
func (h *Handlers) ListRunCheckpoints(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	checkpoints, err := h.Replay.ListCheckpoints(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "run not found")
		return
	}
	if checkpoints == nil {
		checkpoints = []event.AgentEvent{}
	}
	writeJSON(w, http.StatusOK, checkpoints)
}

// ReplayRun handles POST /api/v1/runs/{id}/replay
func (h *Handlers) ReplayRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, ok := readJSON[event.ReplayRequest](w, r)
	if !ok {
		return
	}
	req.RunID = id

	result, err := h.Replay.Replay(r.Context(), req)
	if err != nil {
		writeDomainError(w, err, "run not found")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// GlobalAuditTrail handles GET /api/v1/audit
func (h *Handlers) GlobalAuditTrail(w http.ResponseWriter, r *http.Request) {
	filter := event.AuditFilter{
		Action: r.URL.Query().Get("action"),
	}
	cursor := r.URL.Query().Get("cursor")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	page, err := h.Replay.AuditTrail(r.Context(), &filter, cursor, limit)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// ProjectAuditTrail handles GET /api/v1/projects/{id}/audit
func (h *Handlers) ProjectAuditTrail(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	filter := event.AuditFilter{
		ProjectID: projectID,
		Action:    r.URL.Query().Get("action"),
	}
	cursor := r.URL.Query().Get("cursor")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	page, err := h.Replay.AuditTrail(r.Context(), &filter, cursor, limit)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// --- Sessions ---

// ResumeRun handles POST /api/v1/runs/{id}/resume
func (h *Handlers) ResumeRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req run.ResumeRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req = run.ResumeRequest{} // empty body is OK
	}
	req.RunID = id

	sess, err := h.Sessions.Resume(r.Context(), req)
	if err != nil {
		writeDomainError(w, err, "run not found")
		return
	}
	writeJSON(w, http.StatusCreated, sess)
}

// ForkRun handles POST /api/v1/runs/{id}/fork
func (h *Handlers) ForkRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req run.ForkRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req = run.ForkRequest{} // empty body is OK
	}
	req.RunID = id

	sess, err := h.Sessions.Fork(r.Context(), req)
	if err != nil {
		writeDomainError(w, err, "run not found")
		return
	}
	writeJSON(w, http.StatusCreated, sess)
}

// RewindRun handles POST /api/v1/runs/{id}/rewind
func (h *Handlers) RewindRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req run.RewindRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req = run.RewindRequest{} // empty body is OK
	}
	req.RunID = id

	sess, err := h.Sessions.Rewind(r.Context(), req)
	if err != nil {
		writeDomainError(w, err, "run not found")
		return
	}
	writeJSON(w, http.StatusCreated, sess)
}

// ListProjectSessions handles GET /api/v1/projects/{id}/sessions
func (h *Handlers) ListProjectSessions(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	sessions, err := h.Sessions.ListSessions(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	if sessions == nil {
		sessions = []run.Session{}
	}
	writeJSON(w, http.StatusOK, sessions)
}

// GetSession handles GET /api/v1/sessions/{id}
func (h *Handlers) GetSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sess, err := h.Sessions.GetSession(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "session not found")
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

// --- VCS Webhooks ---

// HandleGitHubWebhook handles POST /api/v1/webhooks/vcs/github
func (h *Handlers) HandleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	eventType := r.Header.Get("X-GitHub-Event")
	switch eventType {
	case "push":
		ev, err := h.VCSWebhook.HandleGitHubPush(r.Context(), body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, ev)
	case "pull_request":
		ev, err := h.VCSWebhook.HandleGitHubPullRequest(r.Context(), body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, ev)
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "event": eventType})
	}
}

// HandleGitLabWebhook handles POST /api/v1/webhooks/vcs/gitlab
func (h *Handlers) HandleGitLabWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	eventType := r.Header.Get("X-Gitlab-Event")
	switch eventType {
	case "Push Hook":
		ev, err := h.VCSWebhook.HandleGitLabPush(r.Context(), body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, ev)
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "event": eventType})
	}
}

// --- Bidirectional Sync ---

// SyncRoadmap handles POST /api/v1/projects/{id}/roadmap/sync
func (h *Handlers) SyncRoadmap(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	req, ok := readJSON[roadmap.SyncConfig](w, r)
	if !ok {
		return
	}
	req.ProjectID = projectID

	if req.Provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}
	if req.ProjectRef == "" {
		writeError(w, http.StatusBadRequest, "project_ref is required")
		return
	}
	if req.Direction == "" {
		req.Direction = roadmap.SyncDirectionPull
	}

	result, err := h.Sync.Sync(r.Context(), req)
	if err != nil {
		writeDomainError(w, err, "sync failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// --- PM Webhooks ---

// HandleGitHubIssueWebhook handles POST /api/v1/webhooks/pm/github
func (h *Handlers) HandleGitHubIssueWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	eventType := r.Header.Get("X-GitHub-Event")
	if eventType != "issues" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "event": eventType})
		return
	}

	ev, err := h.PMWebhook.HandleGitHubIssueWebhook(r.Context(), body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// HandleGitLabIssueWebhook handles POST /api/v1/webhooks/pm/gitlab
func (h *Handlers) HandleGitLabIssueWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	eventType := r.Header.Get("X-Gitlab-Event")
	if eventType != "Issue Hook" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "event": eventType})
		return
	}

	ev, err := h.PMWebhook.HandleGitLabIssueWebhook(r.Context(), body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// HandlePlaneWebhook handles POST /api/v1/webhooks/pm/plane
func (h *Handlers) HandlePlaneWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	ev, err := h.PMWebhook.HandlePlaneWebhook(r.Context(), body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// --- Helpers ---

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to write JSON response", "error", err)
	}
}

// --- Pipeline Templates ---

// ListPipelines handles GET /api/v1/pipelines
func (h *Handlers) ListPipelines(w http.ResponseWriter, _ *http.Request) {
	templates := h.Pipelines.List()
	if templates == nil {
		templates = []pipeline.Template{}
	}
	writeJSON(w, http.StatusOK, templates)
}

// GetPipeline handles GET /api/v1/pipelines/{id}
func (h *Handlers) GetPipeline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := h.Pipelines.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "pipeline template not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// RegisterPipeline handles POST /api/v1/pipelines
func (h *Handlers) RegisterPipeline(w http.ResponseWriter, r *http.Request) {
	t, ok := readJSON[pipeline.Template](w, r)
	if !ok {
		return
	}
	if err := h.Pipelines.Register(&t); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// InstantiatePipeline handles POST /api/v1/pipelines/{id}/instantiate
func (h *Handlers) InstantiatePipeline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, ok := readJSON[pipeline.InstantiateRequest](w, r)
	if !ok {
		return
	}

	result, err := h.Pipelines.Instantiate(r.Context(), id, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

// --- Review Policies & Reviews (Phase 12I) ---

// ListReviewPolicies handles GET /api/v1/projects/{id}/review-policies
func (h *Handlers) ListReviewPolicies(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	policies, err := h.Review.ListPolicies(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, policies)
}

// CreateReviewPolicy handles POST /api/v1/projects/{id}/review-policies
func (h *Handlers) CreateReviewPolicy(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	tenantID := middleware.TenantIDFromContext(r.Context())
	req, ok := readJSON[review.CreatePolicyRequest](w, r)
	if !ok {
		return
	}

	p, err := h.Review.CreatePolicy(r.Context(), projectID, tenantID, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// GetReviewPolicy handles GET /api/v1/review-policies/{id}
func (h *Handlers) GetReviewPolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := h.Review.GetPolicy(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "review policy not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// UpdateReviewPolicy handles PUT /api/v1/review-policies/{id}
func (h *Handlers) UpdateReviewPolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, ok := readJSON[review.UpdatePolicyRequest](w, r)
	if !ok {
		return
	}

	p, err := h.Review.UpdatePolicy(r.Context(), id, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// DeleteReviewPolicy handles DELETE /api/v1/review-policies/{id}
func (h *Handlers) DeleteReviewPolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Review.DeletePolicy(r.Context(), id); err != nil {
		writeDomainError(w, err, "review policy not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TriggerReview handles POST /api/v1/review-policies/{id}/trigger
func (h *Handlers) TriggerReview(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rev, err := h.Review.ManualTrigger(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "review policy not found")
		return
	}
	writeJSON(w, http.StatusCreated, rev)
}

// ListReviews handles GET /api/v1/projects/{id}/reviews
func (h *Handlers) ListReviews(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	reviews, err := h.Review.ListReviews(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, reviews)
}

// GetReviewHandler handles GET /api/v1/reviews/{id}
func (h *Handlers) GetReviewHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rev, err := h.Review.GetReview(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "review not found")
		return
	}
	writeJSON(w, http.StatusOK, rev)
}

func writeDomainError(w http.ResponseWriter, err error, fallbackMsg string) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, fallbackMsg)
	case errors.Is(err, domain.ErrConflict):
		writeError(w, http.StatusConflict, "resource was modified by another request")
	case errors.Is(err, domain.ErrValidation):
		msg := strings.TrimPrefix(err.Error(), domain.ErrValidation.Error()+": ")
		writeError(w, http.StatusBadRequest, msg)
	case strings.Contains(err.Error(), "invalid input syntax"):
		// PostgreSQL type-cast error (e.g. invalid UUID format) → 400
		writeError(w, http.StatusBadRequest, "invalid identifier format")
	case strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "SQLSTATE 23505"):
		// PostgreSQL unique violation → 409 Conflict
		writeError(w, http.StatusConflict, "resource already exists")
	default:
		slog.Error("unhandled domain error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// --- Settings ---

// GetSettings handles GET /api/v1/settings
func (h *Handlers) GetSettings(w http.ResponseWriter, r *http.Request) {
	list, err := h.Settings.List(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}

	// Return as a map of key -> value for frontend convenience.
	result := make(map[string]json.RawMessage, len(list))
	for _, s := range list {
		result[s.Key] = s.Value
	}
	writeJSON(w, http.StatusOK, result)
}

// UpdateSettings handles PUT /api/v1/settings
func (h *Handlers) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[settings.UpdateRequest](w, r)
	if !ok {
		return
	}
	if len(req.Settings) == 0 {
		writeError(w, http.StatusBadRequest, "settings map must not be empty")
		return
	}
	if err := h.Settings.Update(r.Context(), req); err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- VCS Accounts ---

// ListVCSAccounts handles GET /api/v1/vcs-accounts
func (h *Handlers) ListVCSAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.VCSAccounts.List(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if accounts == nil {
		accounts = []vcsaccount.VCSAccount{}
	}
	writeJSON(w, http.StatusOK, accounts)
}

// CreateVCSAccount handles POST /api/v1/vcs-accounts
func (h *Handlers) CreateVCSAccount(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[vcsaccount.CreateRequest](w, r)
	if !ok {
		return
	}
	account, err := h.VCSAccounts.Create(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Clear encrypted token from the response.
	account.EncryptedToken = nil
	writeJSON(w, http.StatusCreated, account)
}

// DeleteVCSAccount handles DELETE /api/v1/vcs-accounts/{id}
func (h *Handlers) DeleteVCSAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.VCSAccounts.Delete(r.Context(), id); err != nil {
		writeDomainError(w, err, "vcs account not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TestVCSAccount handles POST /api/v1/vcs-accounts/{id}/test
func (h *Handlers) TestVCSAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.VCSAccounts.Test(r.Context(), id); err != nil {
		writeDomainError(w, err, "vcs account not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- Conversation Handlers ---

// CreateConversation handles POST /api/v1/projects/{id}/conversations
func (h *Handlers) CreateConversation(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	req, ok := readJSON[conversation.CreateRequest](w, r)
	if !ok {
		return
	}
	req.ProjectID = projectID
	conv, err := h.Conversations.Create(r.Context(), req)
	if err != nil {
		writeDomainError(w, err, "create conversation")
		return
	}
	writeJSON(w, http.StatusCreated, conv)
}

// ListConversations handles GET /api/v1/projects/{id}/conversations
func (h *Handlers) ListConversations(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	conversations, err := h.Conversations.ListByProject(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	if conversations == nil {
		conversations = []conversation.Conversation{}
	}
	writeJSON(w, http.StatusOK, conversations)
}

// GetConversation handles GET /api/v1/conversations/{id}
func (h *Handlers) GetConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	conv, err := h.Conversations.Get(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "get conversation")
		return
	}
	writeJSON(w, http.StatusOK, conv)
}

// DeleteConversation handles DELETE /api/v1/conversations/{id}
func (h *Handlers) DeleteConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Conversations.Delete(r.Context(), id); err != nil {
		writeDomainError(w, err, "delete conversation")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListConversationMessages handles GET /api/v1/conversations/{id}/messages
func (h *Handlers) ListConversationMessages(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	messages, err := h.Conversations.ListMessages(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "conversation not found")
		return
	}
	if messages == nil {
		messages = []conversation.Message{}
	}
	writeJSON(w, http.StatusOK, messages)
}

// SendConversationMessage handles POST /api/v1/conversations/{id}/messages.
// When agentic mode is active (via request body or project default), the message
// is dispatched to the Python worker for autonomous tool-using execution.
// Otherwise it falls back to a simple single-turn LLM call.
func (h *Handlers) SendConversationMessage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, ok := readJSON[conversation.SendMessageRequest](w, r)
	if !ok {
		return
	}

	// Route to agentic path when applicable.
	if h.Conversations.IsAgentic(r.Context(), id, req) {
		if err := h.Conversations.SendMessageAgentic(r.Context(), id, req); err != nil {
			writeDomainError(w, err, "send agentic message")
			return
		}
		// Agentic mode returns immediately; results stream via WebSocket.
		writeJSON(w, http.StatusAccepted, map[string]string{
			"status":  "dispatched",
			"run_id":  id,
			"message": "Agentic run dispatched. Results will stream via WebSocket.",
		})
		return
	}

	msg, err := h.Conversations.SendMessage(r.Context(), id, req)
	if err != nil {
		writeDomainError(w, err, "send message")
		return
	}
	writeJSON(w, http.StatusCreated, msg)
}

// StopConversation handles POST /api/v1/conversations/{id}/stop.
// Cancels an active agentic conversation run.
func (h *Handlers) StopConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Conversations.StopConversation(r.Context(), id); err != nil {
		writeDomainError(w, err, "stop conversation")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled", "conversation_id": id})
}

// --- HITL Approval ---

// ApproveToolCall handles POST /api/v1/runs/{id}/approve/{callId}.
// The user sends a decision ("allow" or "deny") to approve or reject a pending
// tool call that the policy evaluated as "ask".
func (h *Handlers) ApproveToolCall(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	callID := chi.URLParam(r, "callId")

	type approvalRequest struct {
		Decision string `json:"decision"` // "allow" or "deny"
	}

	req, ok := readJSON[approvalRequest](w, r)
	if !ok {
		return
	}
	if req.Decision != "allow" && req.Decision != "deny" {
		writeError(w, http.StatusBadRequest, "decision must be 'allow' or 'deny'")
		return
	}

	resolved := h.Runtime.ResolveApproval(runID, callID, req.Decision)
	if !resolved {
		writeError(w, http.StatusNotFound, "no pending approval for this run/call")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":   "resolved",
		"run_id":   runID,
		"call_id":  callID,
		"decision": req.Decision,
	})
}

// --- Dev Tools ---

// BenchmarkPrompt handles POST /api/v1/dev/benchmark
// Sends a prompt to LiteLLM and returns the response with timing/token metrics.
// Guarded by the DEV_MODE environment variable.
func (h *Handlers) BenchmarkPrompt(w http.ResponseWriter, r *http.Request) {
	if strings.ToLower(os.Getenv("DEV_MODE")) != "true" {
		writeError(w, http.StatusForbidden, "dev mode not enabled")
		return
	}

	type benchmarkRequest struct {
		Model        string  `json:"model"`
		Prompt       string  `json:"prompt"`
		SystemPrompt string  `json:"system_prompt"`
		Temperature  float64 `json:"temperature"`
		MaxTokens    int     `json:"max_tokens"`
	}

	req, ok := readJSON[benchmarkRequest](w, r)
	if !ok {
		return
	}
	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "model is required")
		return
	}
	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}

	messages := []litellm.ChatMessage{}
	if req.SystemPrompt != "" {
		messages = append(messages, litellm.ChatMessage{Role: "system", Content: req.SystemPrompt})
	}
	messages = append(messages, litellm.ChatMessage{Role: "user", Content: req.Prompt})

	start := time.Now()
	resp, err := h.LiteLLM.ChatCompletion(r.Context(), litellm.ChatCompletionRequest{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	})
	latencyMs := time.Since(start).Milliseconds()

	if err != nil {
		slog.Error("benchmark prompt failed", "error", err)
		writeError(w, http.StatusBadGateway, "LLM call failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"content":    resp.Content,
		"model":      resp.Model,
		"tokens_in":  resp.TokensIn,
		"tokens_out": resp.TokensOut,
		"latency_ms": latencyMs,
	})
}

// --- LSP (Language Server Protocol) ---

// StartLSP handles POST /api/v1/projects/{id}/lsp/start
func (h *Handlers) StartLSP(w http.ResponseWriter, r *http.Request) {
	if h.LSP == nil {
		writeError(w, http.StatusServiceUnavailable, "LSP integration is not enabled")
		return
	}
	projectID := chi.URLParam(r, "id")
	proj, err := h.Projects.Get(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	if proj.WorkspacePath == "" {
		writeError(w, http.StatusBadRequest, "project has no workspace; clone or adopt first")
		return
	}

	var body struct {
		Languages []string `json:"languages"`
	}
	// Body is optional — auto-detect if empty.
	_ = json.NewDecoder(r.Body).Decode(&body)

	if err := h.LSP.StartServers(r.Context(), projectID, proj.WorkspacePath, body.Languages); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

// StopLSP handles POST /api/v1/projects/{id}/lsp/stop
func (h *Handlers) StopLSP(w http.ResponseWriter, r *http.Request) {
	if h.LSP == nil {
		writeError(w, http.StatusServiceUnavailable, "LSP integration is not enabled")
		return
	}
	projectID := chi.URLParam(r, "id")
	if err := h.LSP.StopServers(r.Context(), projectID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// LSPStatus handles GET /api/v1/projects/{id}/lsp/status
func (h *Handlers) LSPStatus(w http.ResponseWriter, r *http.Request) {
	if h.LSP == nil {
		writeJSON(w, http.StatusOK, []lspDomain.ServerInfo{})
		return
	}
	projectID := chi.URLParam(r, "id")
	infos := h.LSP.Status(projectID)
	if infos == nil {
		infos = []lspDomain.ServerInfo{}
	}
	writeJSON(w, http.StatusOK, infos)
}

// LSPDiagnostics handles GET /api/v1/projects/{id}/lsp/diagnostics
func (h *Handlers) LSPDiagnostics(w http.ResponseWriter, r *http.Request) {
	if h.LSP == nil {
		writeJSON(w, http.StatusOK, []lspDomain.Diagnostic{})
		return
	}
	projectID := chi.URLParam(r, "id")
	uri := r.URL.Query().Get("uri")
	diags := h.LSP.Diagnostics(projectID, uri)
	if diags == nil {
		diags = []lspDomain.Diagnostic{}
	}
	writeJSON(w, http.StatusOK, diags)
}

// lspPositionRequest is the shared request body for definition/references/hover.
type lspPositionRequest struct {
	URI       string `json:"uri"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
}

// LSPDefinition handles POST /api/v1/projects/{id}/lsp/definition
func (h *Handlers) LSPDefinition(w http.ResponseWriter, r *http.Request) {
	if h.LSP == nil {
		writeError(w, http.StatusServiceUnavailable, "LSP integration is not enabled")
		return
	}
	projectID := chi.URLParam(r, "id")
	req, ok := readJSON[lspPositionRequest](w, r)
	if !ok {
		return
	}
	if req.URI == "" {
		writeError(w, http.StatusBadRequest, "uri is required")
		return
	}
	locs, err := h.LSP.Definition(r.Context(), projectID, req.URI, lspDomain.Position{
		Line: req.Line, Character: req.Character,
	})
	if err != nil {
		writeDomainError(w, err, "definition lookup failed")
		return
	}
	if locs == nil {
		locs = []lspDomain.Location{}
	}
	writeJSON(w, http.StatusOK, locs)
}

// LSPReferences handles POST /api/v1/projects/{id}/lsp/references
func (h *Handlers) LSPReferences(w http.ResponseWriter, r *http.Request) {
	if h.LSP == nil {
		writeError(w, http.StatusServiceUnavailable, "LSP integration is not enabled")
		return
	}
	projectID := chi.URLParam(r, "id")
	req, ok := readJSON[lspPositionRequest](w, r)
	if !ok {
		return
	}
	if req.URI == "" {
		writeError(w, http.StatusBadRequest, "uri is required")
		return
	}
	locs, err := h.LSP.References(r.Context(), projectID, req.URI, lspDomain.Position{
		Line: req.Line, Character: req.Character,
	})
	if err != nil {
		writeDomainError(w, err, "references lookup failed")
		return
	}
	if locs == nil {
		locs = []lspDomain.Location{}
	}
	writeJSON(w, http.StatusOK, locs)
}

// LSPDocumentSymbols handles POST /api/v1/projects/{id}/lsp/symbols
func (h *Handlers) LSPDocumentSymbols(w http.ResponseWriter, r *http.Request) {
	if h.LSP == nil {
		writeError(w, http.StatusServiceUnavailable, "LSP integration is not enabled")
		return
	}
	projectID := chi.URLParam(r, "id")
	var req struct {
		URI string `json:"uri"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodySize)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.URI == "" {
		writeError(w, http.StatusBadRequest, "uri is required")
		return
	}
	symbols, err := h.LSP.DocumentSymbols(r.Context(), projectID, req.URI)
	if err != nil {
		writeDomainError(w, err, "symbol lookup failed")
		return
	}
	if symbols == nil {
		symbols = []lspDomain.DocumentSymbol{}
	}
	writeJSON(w, http.StatusOK, symbols)
}

// LSPHover handles POST /api/v1/projects/{id}/lsp/hover
func (h *Handlers) LSPHover(w http.ResponseWriter, r *http.Request) {
	if h.LSP == nil {
		writeError(w, http.StatusServiceUnavailable, "LSP integration is not enabled")
		return
	}
	projectID := chi.URLParam(r, "id")
	req, ok := readJSON[lspPositionRequest](w, r)
	if !ok {
		return
	}
	if req.URI == "" {
		writeError(w, http.StatusBadRequest, "uri is required")
		return
	}
	result, err := h.LSP.Hover(r.Context(), projectID, req.URI, lspDomain.Position{
		Line: req.Line, Character: req.Character,
	})
	if err != nil {
		writeDomainError(w, err, "hover lookup failed")
		return
	}
	if result == nil {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// writeInternalError logs the actual error server-side and returns a generic message to the client.
func writeInternalError(w http.ResponseWriter, err error) {
	slog.Error("request failed", "error", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}

// --- Model Registry Handlers (Phase 22) ---

// AvailableLLMModels handles GET /api/v1/llm/available — returns cached model health.
func (h *Handlers) AvailableLLMModels(w http.ResponseWriter, r *http.Request) {
	if h.ModelRegistry == nil {
		writeError(w, http.StatusServiceUnavailable, "model registry not initialized")
		return
	}
	type resp struct {
		Models    []litellm.DiscoveredModel `json:"models"`
		BestModel string                    `json:"best_model"`
	}
	writeJSON(w, http.StatusOK, resp{
		Models:    h.ModelRegistry.AvailableModels(),
		BestModel: h.ModelRegistry.BestModel(),
	})
}

// RefreshLLMModels handles POST /api/v1/llm/refresh — triggers immediate model refresh.
func (h *Handlers) RefreshLLMModels(w http.ResponseWriter, r *http.Request) {
	if h.ModelRegistry == nil {
		writeError(w, http.StatusServiceUnavailable, "model registry not initialized")
		return
	}
	if err := h.ModelRegistry.Refresh(r.Context()); err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "refreshed"})
}

// --- Copilot Token Exchange Handler (Phase 22A) ---

// HandleCopilotExchange handles POST /api/v1/copilot/exchange.
func (h *Handlers) HandleCopilotExchange(w http.ResponseWriter, r *http.Request) {
	if h.Copilot == nil {
		writeError(w, http.StatusNotFound, "copilot integration not enabled")
		return
	}
	token, expiry, err := h.Copilot.ExchangeToken(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"token":      token,
		"expires_at": expiry.Format(time.RFC3339),
	})
}

// --- Memory Handlers (Phase 22B) ---

// ListMemories handles GET /api/v1/projects/{id}/memories.
func (h *Handlers) ListMemories(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	mems, err := h.Memory.ListByProject(r.Context(), projectID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if mems == nil {
		mems = []memory.Memory{}
	}
	writeJSON(w, http.StatusOK, mems)
}

// StoreMemory handles POST /api/v1/projects/{id}/memories.
func (h *Handlers) StoreMemory(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	req, ok := readJSON[memory.CreateRequest](w, r)
	if !ok {
		return
	}
	req.ProjectID = projectID
	if err := h.Memory.Store(r.Context(), &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "dispatched"})
}

// RecallMemories handles POST /api/v1/projects/{id}/memories/recall.
func (h *Handlers) RecallMemories(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	req, ok := readJSON[memory.RecallRequest](w, r)
	if !ok {
		return
	}
	req.ProjectID = projectID
	if err := h.Memory.Recall(r.Context(), req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "dispatched"})
}

// --- Experience Pool Handlers (Phase 22B) ---

// ListExperienceEntries handles GET /api/v1/projects/{id}/experience.
func (h *Handlers) ListExperienceEntries(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	entries, err := h.ExperiencePool.ListByProject(r.Context(), projectID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if entries == nil {
		entries = []experience.Entry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

// DeleteExperienceEntry handles DELETE /api/v1/experience/{id}.
func (h *Handlers) DeleteExperienceEntry(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.ExperiencePool.Delete(r.Context(), id); err != nil {
		writeInternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Microagent Handlers (Phase 22C) ---

// ListMicroagents handles GET /api/v1/projects/{id}/microagents.
func (h *Handlers) ListMicroagents(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	mas, err := h.Microagents.List(r.Context(), projectID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if mas == nil {
		mas = []microagent.Microagent{}
	}
	writeJSON(w, http.StatusOK, mas)
}

// CreateMicroagent handles POST /api/v1/projects/{id}/microagents.
func (h *Handlers) CreateMicroagent(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	req, ok := readJSON[microagent.CreateRequest](w, r)
	if !ok {
		return
	}
	req.ProjectID = projectID
	m, err := h.Microagents.Create(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// GetMicroagent handles GET /api/v1/microagents/{id}.
func (h *Handlers) GetMicroagent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	m, err := h.Microagents.Get(r.Context(), id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// UpdateMicroagent handles PUT /api/v1/microagents/{id}.
func (h *Handlers) UpdateMicroagent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, ok := readJSON[microagent.UpdateRequest](w, r)
	if !ok {
		return
	}
	m, err := h.Microagents.Update(r.Context(), id, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// DeleteMicroagent handles DELETE /api/v1/microagents/{id}.
func (h *Handlers) DeleteMicroagent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Microagents.Delete(r.Context(), id); err != nil {
		writeInternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Skill Handlers (Phase 22D) ---

// ListSkills handles GET /api/v1/projects/{id}/skills.
func (h *Handlers) ListSkills(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	sk, err := h.Skills.List(r.Context(), projectID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if sk == nil {
		sk = []skill.Skill{}
	}
	writeJSON(w, http.StatusOK, sk)
}

// CreateSkill handles POST /api/v1/projects/{id}/skills.
func (h *Handlers) CreateSkill(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	req, ok := readJSON[skill.CreateRequest](w, r)
	if !ok {
		return
	}
	req.ProjectID = projectID
	s, err := h.Skills.Create(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

// GetSkill handles GET /api/v1/skills/{id}.
func (h *Handlers) GetSkill(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s, err := h.Skills.Get(r.Context(), id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// UpdateSkill handles PUT /api/v1/skills/{id}.
func (h *Handlers) UpdateSkill(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, ok := readJSON[skill.UpdateRequest](w, r)
	if !ok {
		return
	}
	s, err := h.Skills.Update(r.Context(), id, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// DeleteSkill handles DELETE /api/v1/skills/{id}.
func (h *Handlers) DeleteSkill(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Skills.Delete(r.Context(), id); err != nil {
		writeInternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleFeedbackCallback handles POST /api/v1/feedback/{run_id}/{call_id}.
// This is the callback endpoint for email/Slack approval links.
func (h *Handlers) HandleFeedbackCallback(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "run_id")
	callID := chi.URLParam(r, "call_id")
	decision := r.URL.Query().Get("decision")

	if decision != "allow" && decision != "deny" {
		writeError(w, http.StatusBadRequest, "decision must be 'allow' or 'deny'")
		return
	}

	resolved := h.Runtime.ResolveApproval(runID, callID, decision)
	if !resolved {
		writeError(w, http.StatusNotFound, "no pending approval for this run/call")
		return
	}

	// Log audit entry via RuntimeService.
	_ = h.Runtime.LogFeedbackAudit(r.Context(), runID, callID, "", "web_callback", decision, "")

	writeJSON(w, http.StatusOK, map[string]string{
		"status":   "resolved",
		"decision": decision,
	})
}

// ListFeedbackAudit handles GET /api/v1/runs/{id}/feedback.
func (h *Handlers) ListFeedbackAudit(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	entries, err := h.Runtime.ListFeedbackAudit(r.Context(), runID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

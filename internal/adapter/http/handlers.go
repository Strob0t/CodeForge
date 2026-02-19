package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	bp "github.com/Strob0t/CodeForge/internal/domain/branchprotection"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/cost"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/mode"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/resource"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/domain/tenant"
	"github.com/Strob0t/CodeForge/internal/port/agentbackend"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
	"github.com/Strob0t/CodeForge/internal/port/specprovider"
	"github.com/Strob0t/CodeForge/internal/service"
)

const maxQueryLength = 2000

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
}

// ListProjects handles GET /api/v1/projects
func (h *Handlers) ListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.Projects.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
	var req project.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	p, err := h.Projects.Create(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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

// ListTasks handles GET /api/v1/projects/{id}/tasks
func (h *Handlers) ListTasks(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	tasks, err := h.Tasks.List(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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

	var req task.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.ProjectID = projectID

	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	t, err := h.Tasks.Create(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
	p, err := h.Projects.Clone(r.Context(), id)
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

// ProjectGitStatus handles GET /api/v1/projects/{id}/git/status
func (h *Handlers) ProjectGitStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	status, err := h.Projects.Status(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// PullProject handles POST /api/v1/projects/{id}/git/pull
func (h *Handlers) PullProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Projects.Pull(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ListProjectBranches handles GET /api/v1/projects/{id}/git/branches
func (h *Handlers) ListProjectBranches(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	branches, err := h.Projects.ListBranches(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, branches)
}

// CheckoutBranch handles POST /api/v1/projects/{id}/git/checkout
func (h *Handlers) CheckoutBranch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req struct {
		Branch string `json:"branch"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Branch == "" {
		writeError(w, http.StatusBadRequest, "branch is required")
		return
	}

	if err := h.Projects.Checkout(r.Context(), id, req.Branch); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "branch": req.Branch})
}

// ListAgents handles GET /api/v1/projects/{id}/agents
func (h *Handlers) ListAgents(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	agents, err := h.Agents.List(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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

	var req struct {
		Name           string            `json:"name"`
		Backend        string            `json:"backend"`
		Config         map[string]string `json:"config"`
		ResourceLimits *resource.Limits  `json:"resource_limits,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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

	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.TaskID == "" {
		writeError(w, http.StatusBadRequest, "task_id is required")
		return
	}

	if err := h.Agents.Dispatch(r.Context(), agentID, req.TaskID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "dispatched"})
}

// StopAgentTask handles POST /api/v1/agents/{id}/stop
func (h *Handlers) StopAgentTask(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")

	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.TaskID == "" {
		writeError(w, http.StatusBadRequest, "task_id is required")
		return
	}

	if err := h.Agents.StopTask(r.Context(), agentID, req.TaskID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusBadGateway, "litellm unavailable: "+err.Error())
		return
	}
	if models == nil {
		models = []litellm.Model{}
	}
	writeJSON(w, http.StatusOK, models)
}

// AddLLMModel handles POST /api/v1/llm/models
func (h *Handlers) AddLLMModel(w http.ResponseWriter, r *http.Request) {
	var req litellm.AddModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ModelName == "" {
		writeError(w, http.StatusBadRequest, "model_name is required")
		return
	}

	if err := h.LiteLLM.AddModel(r.Context(), req); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok", "model": req.ModelName})
}

// DeleteLLMModel handles POST /api/v1/llm/models/delete
func (h *Handlers) DeleteLLMModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	if err := h.LiteLLM.DeleteModel(r.Context(), req.ID); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
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

	var call policy.ToolCall
	if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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
	var profile policy.PolicyProfile
	if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if profile.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
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
	var req run.StartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// ListTaskRuns handles GET /api/v1/tasks/{id}/runs
func (h *Handlers) ListTaskRuns(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "id")
	runs, err := h.Runtime.ListRunsByTask(r.Context(), taskID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
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

	var req plan.CreatePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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
		writeError(w, http.StatusInternalServerError, err.Error())
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

// --- Feature Decomposition (Meta-Agent) ---

// DecomposeFeature handles POST /api/v1/projects/{id}/decompose
func (h *Handlers) DecomposeFeature(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	var req plan.DecomposeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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
		writeError(w, http.StatusInternalServerError, err.Error())
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

	var req agent.CreateTeamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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

	var req plan.PlanFeatureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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

	var req struct {
		ProjectID string `json:"project_id"`
		TeamID    string `json:"team_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ProjectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	pack, err := h.ContextOptimizer.BuildContextPack(r.Context(), taskID, req.ProjectID, req.TeamID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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

	var req cfcontext.AddSharedItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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

// CreateMode handles POST /api/v1/modes
func (h *Handlers) CreateMode(w http.ResponseWriter, r *http.Request) {
	var m mode.Mode
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.Modes.Register(&m); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, m)
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
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := h.Retrieval.RequestIndex(r.Context(), projectID, req.EmbeddingModel); err != nil {
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

	var req struct {
		Query          string  `json:"query"`
		TopK           int     `json:"top_k"`
		BM25Weight     float64 `json:"bm25_weight"`
		SemanticWeight float64 `json:"semantic_weight"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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
		writeError(w, http.StatusGatewayTimeout, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// --- Retrieval Sub-Agent Endpoints (Phase 6C) ---

// AgentSearchProject handles POST /api/v1/projects/{id}/search/agent
func (h *Handlers) AgentSearchProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	var req struct {
		Query      string `json:"query"`
		TopK       int    `json:"top_k"`
		MaxQueries int    `json:"max_queries"`
		Model      string `json:"model"`
		Rerank     *bool  `json:"rerank"` // pointer to distinguish absent (use config default) from explicit false
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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

	result, err := h.Retrieval.SubAgentSearchSync(r.Context(), projectID, req.Query, topK, maxQueries, model, rerank)
	if err != nil {
		writeError(w, http.StatusGatewayTimeout, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
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

	var req struct {
		SeedSymbols []string `json:"seed_symbols"`
		MaxHops     int      `json:"max_hops"`
		TopK        int      `json:"top_k"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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
		writeError(w, http.StatusGatewayTimeout, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// --- Cost Endpoints ---

// GlobalCostSummary handles GET /api/v1/costs
func (h *Handlers) GlobalCostSummary(w http.ResponseWriter, r *http.Request) {
	summaries, err := h.Cost.GlobalSummary(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// ProjectCostByModel handles GET /api/v1/projects/{id}/costs/by-model
func (h *Handlers) ProjectCostByModel(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	models, err := h.Cost.ByModel(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if runs == nil {
		runs = []run.Run{}
	}
	writeJSON(w, http.StatusOK, runs)
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

	var req roadmap.CreateRoadmapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.ProjectID = projectID

	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	rm, err := h.Roadmap.Create(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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

	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Status      string `json:"status"`
		Version     int    `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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

	var req roadmap.CreateMilestoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.RoadmapID = rm.ID

	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	m, err := h.Roadmap.CreateMilestone(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// UpdateMilestone handles PUT /api/v1/milestones/{id}
func (h *Handlers) UpdateMilestone(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Status      string `json:"status"`
		SortOrder   *int   `json:"sort_order"`
		Version     int    `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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

	var req roadmap.CreateFeatureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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

	var req struct {
		Title       string            `json:"title"`
		Description string            `json:"description"`
		Status      string            `json:"status"`
		Labels      []string          `json:"labels"`
		SpecRef     string            `json:"spec_ref"`
		ExternalIDs map[string]string `json:"external_ids"`
		SortOrder   *int              `json:"sort_order"`
		Version     int               `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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

	var req struct {
		Provider   string `json:"provider"`
		ProjectRef string `json:"project_ref"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Include stats in the response.
	stats, err := h.Events.TrajectoryStats(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tenants == nil {
		tenants = []tenant.Tenant{}
	}
	writeJSON(w, http.StatusOK, tenants)
}

// CreateTenant handles POST /api/v1/tenants
func (h *Handlers) CreateTenant(w http.ResponseWriter, r *http.Request) {
	var req tenant.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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

	var req tenant.UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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
		writeError(w, http.StatusInternalServerError, err.Error())
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

	var req bp.CreateRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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

	var req bp.UpdateRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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
	var req event.ReplayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
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
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// --- Sessions ---

// ResumeRun handles POST /api/v1/runs/{id}/resume
func (h *Handlers) ResumeRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req run.ResumeRequest
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
		writeError(w, http.StatusInternalServerError, err.Error())
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

	var req roadmap.SyncConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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
		writeError(w, http.StatusInternalServerError, err.Error())
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

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func writeDomainError(w http.ResponseWriter, err error, fallbackMsg string) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, fallbackMsg)
	case errors.Is(err, domain.ErrConflict):
		writeError(w, http.StatusConflict, "resource was modified by another request")
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

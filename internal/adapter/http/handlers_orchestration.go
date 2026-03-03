package http

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/artifact"
	"github.com/Strob0t/CodeForge/internal/domain/mode"
	"github.com/Strob0t/CodeForge/internal/domain/pipeline"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
)

// --- Execution Plan Endpoints ---

// CreatePlan handles POST /api/v1/projects/{id}/plans
func (h *Handlers) CreatePlan(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	req, ok := readJSON[plan.CreatePlanRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	req.ProjectID = projectID

	p, err := h.Orchestrator.CreatePlan(r.Context(), &req)
	if err != nil {
		writeDomainError(w, err, "create plan failed")
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
		writeDomainError(w, err, "start plan failed")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// CancelPlan handles POST /api/v1/plans/{id}/cancel
func (h *Handlers) CancelPlan(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Orchestrator.CancelPlan(r.Context(), id); err != nil {
		writeDomainError(w, err, "cancel plan failed")
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

	req, ok := readJSON[plan.DecomposeRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	req.ProjectID = projectID

	p, err := h.MetaAgent.DecomposeFeature(r.Context(), &req)
	if err != nil {
		writeDomainError(w, err, "decompose feature failed")
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// --- Context-Optimized Feature Planning ---

// PlanFeature handles POST /api/v1/projects/{id}/plan-feature
func (h *Handlers) PlanFeature(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	req, ok := readJSON[plan.PlanFeatureRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	req.ProjectID = projectID

	p, err := h.TaskPlanner.PlanFeature(r.Context(), &req)
	if err != nil {
		writeDomainError(w, err, "plan feature failed")
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
	}](w, r, h.Limits.MaxRequestBodySize)
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

// ListModeTools handles GET /api/v1/modes/tools
func (h *Handlers) ListModeTools(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, mode.BuiltinToolNames)
}

// ListArtifactTypes handles GET /api/v1/modes/artifact-types
func (h *Handlers) ListArtifactTypes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, artifact.KnownTypeNames())
}

// CreateMode handles POST /api/v1/modes
func (h *Handlers) CreateMode(w http.ResponseWriter, r *http.Request) {
	m, ok := readJSON[mode.Mode](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if err := h.Modes.Register(&m); err != nil {
		writeDomainError(w, err, "register mode failed")
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// UpdateMode handles PUT /api/v1/modes/{id}
func (h *Handlers) UpdateMode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	m, ok := readJSON[mode.Mode](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if err := h.Modes.Update(id, &m); err != nil {
		writeDomainError(w, err, "update mode failed")
		return
	}
	updated, _ := h.Modes.Get(id)
	writeJSON(w, http.StatusOK, updated)
}

// DeleteMode handles DELETE /api/v1/modes/{id}
func (h *Handlers) DeleteMode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Modes.Delete(id); err != nil {
		writeDomainError(w, err, "delete mode failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	t, ok := readJSON[pipeline.Template](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if err := h.Pipelines.Register(&t); err != nil {
		writeDomainError(w, err, "register pipeline failed")
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// InstantiatePipeline handles POST /api/v1/pipelines/{id}/instantiate
func (h *Handlers) InstantiatePipeline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, ok := readJSON[pipeline.InstantiateRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}

	result, err := h.Pipelines.Instantiate(r.Context(), id, req)
	if err != nil {
		writeDomainError(w, err, "instantiate pipeline failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

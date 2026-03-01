package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
	"github.com/Strob0t/CodeForge/internal/port/specprovider"
)

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

	req, ok := readJSON[roadmap.CreateRoadmapRequest](w, r, h.Limits.MaxRequestBodySize)
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
	}](w, r, h.Limits.MaxRequestBodySize)
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

	req, ok := readJSON[roadmap.CreateMilestoneRequest](w, r, h.Limits.MaxRequestBodySize)
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
	}](w, r, h.Limits.MaxRequestBodySize)
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

	req, ok := readJSON[roadmap.CreateFeatureRequest](w, r, h.Limits.MaxRequestBodySize)
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
	}](w, r, h.Limits.MaxRequestBodySize)
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
	}](w, r, h.Limits.MaxRequestBodySize)
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

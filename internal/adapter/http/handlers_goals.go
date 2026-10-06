package http

import (
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/goal"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/workspacefs"
)

// ListProjectGoals handles GET /api/v1/projects/{id}/goals.
func (h *Handlers) ListProjectGoals(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	goals, err := h.GoalDiscovery.List(r.Context(), projectID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSONList(w, http.StatusOK, goals)
}

// CreateProjectGoal handles POST /api/v1/projects/{id}/goals.
func (h *Handlers) CreateProjectGoal(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	req, ok := readJSON[goal.CreateRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	g, err := h.GoalDiscovery.Create(r.Context(), projectID, &req)
	if err != nil {
		writeDomainError(w, err, "create goal failed")
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

// DetectProjectGoals handles POST /api/v1/projects/{id}/goals/detect.
func (h *Handlers) DetectProjectGoals(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	proj, err := h.Projects.Get(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	if proj.WorkspacePath == "" {
		writeError(w, http.StatusBadRequest, "project has no workspace path")
		return
	}

	result, err := h.GoalDiscovery.DetectAndImport(r.Context(), projectID, proj.WorkspacePath)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// GetProjectGoal handles GET /api/v1/goals/{id}.
func (h *Handlers) GetProjectGoal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	g, err := h.GoalDiscovery.Get(r.Context(), id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

// UpdateProjectGoal handles PUT /api/v1/goals/{id}.
func (h *Handlers) UpdateProjectGoal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, ok := readJSON[goal.UpdateRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	g, err := h.GoalDiscovery.Update(r.Context(), id, req)
	if err != nil {
		writeDomainError(w, err, "update goal failed")
		return
	}
	writeJSON(w, http.StatusOK, g)
}

// DeleteProjectGoal handles DELETE /api/v1/goals/{id}.
func (h *Handlers) DeleteProjectGoal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.GoalDiscovery.Delete(r.Context(), id); err != nil {
		writeInternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AIDiscoverProjectGoals handles POST /api/v1/projects/{id}/goals/ai-discover.
// It creates a conversation with the goal_researcher mode and dispatches an agentic run
// to analyze the repository and ask the user targeted questions for goal creation.
func (h *Handlers) AIDiscoverProjectGoals(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	// Accept optional model override from request body.
	body, ok := readOptionalJSON[struct {
		Model string `json:"model"`
	}](w, r, h.Limits.MaxRequestBodySize, "AIDiscoverProjectGoals")
	if !ok {
		return
	}

	proj, err := h.Projects.Get(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	if proj.WorkspacePath == "" {
		writeError(w, http.StatusBadRequest, "project has no workspace — clone it first")
		return
	}

	if h.Conversations == nil {
		writeError(w, http.StatusServiceUnavailable, "conversation service unavailable")
		return
	}

	// Create a dedicated conversation for goal discovery.
	conv, err := h.Conversations.Create(r.Context(), conversation.CreateRequest{
		ProjectID: projectID,
		Title:     "Goal Discovery",
	})
	if err != nil {
		writeInternalError(w, err)
		return
	}

	// Inject existing goal/project doc files as context for the agent. They
	// are read through workspacefs (KI-95): never through a symlink that
	// leaves the workspace, a FIFO or a file over maxGoalDocBytes.
	contextEntries := goalDocEntries(proj.WorkspacePath)

	// Dispatch an agentic run with the goal_researcher mode.
	initialPrompt := "Analyze this repository and help me define project goals. " +
		"Start by exploring the codebase structure, then ask me targeted questions."

	var agenticOpts []service.AgenticOption
	if body.Model != "" {
		agenticOpts = append(agenticOpts, service.WithModel(body.Model))
	}
	if len(contextEntries) > 0 {
		agenticOpts = append(agenticOpts, service.WithContextEntries(contextEntries))
	}

	if err := h.Conversations.SendMessageAgenticWithMode(
		r.Context(), conv.ID, initialPrompt, "goal_researcher", agenticOpts...,
	); err != nil {
		slog.Warn("failed to dispatch goal discovery run",
			"conversation_id", conv.ID,
			"project_id", projectID,
			"error", err,
		)
		writeDomainError(w, err, "start goal discovery")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"conversation_id": conv.ID,
		"status":          "started",
	})
}

// maxGoalDocBytes caps a goal document injected into the discovery run; a longer
// one is cut there and marked (as goal discovery caps its files).
const maxGoalDocBytes = 50 * 1024

// goalDocEntries reads the existing goal documents of a workspace; missing
// or refused ones are left out.
func goalDocEntries(workspacePath string) []messagequeue.ContextEntryPayload {
	ws, err := workspacefs.Open(workspacePath)
	if err != nil {
		slog.Warn("goal discovery: cannot open workspace", "path", workspacePath, "error", err)
		return nil
	}
	defer func() { _ = ws.Close() }()
	var entries []messagequeue.ContextEntryPayload
	for _, name := range []string{"docs/PROJECT.md", "docs/REQUIREMENTS.md", "docs/STATE.md"} {
		content, info, truncated, readErr := ws.ReadFilePrefix(name, maxGoalDocBytes)
		if readErr != nil {
			continue // File doesn't exist yet -- that's fine.
		}
		text := string(content)
		if truncated {
			slog.Info("goal discovery: document truncated", "path", name, "size", info.Size(), "max", maxGoalDocBytes)
			text = strings.ToValidUTF8(trimPartialRune(text), "") + goalDocTruncatedMarker
		}
		entries = append(entries, messagequeue.ContextEntryPayload{
			Kind:    "file",
			Path:    name,
			Content: text,
		})
	}
	return entries
}

// goalDocTruncatedMarker ends a goal document cut at maxGoalDocBytes.
const goalDocTruncatedMarker = "\n\n[truncated]"

// trimPartialRune drops an incomplete UTF-8 sequence the cut left at the end.
func trimPartialRune(s string) string {
	for i := 0; i < utf8.UTFMax && s != ""; i++ {
		if r, size := utf8.DecodeLastRuneInString(s); r != utf8.RuneError || size != 1 {
			return s
		}
		s = s[:len(s)-1]
	}
	return s
}

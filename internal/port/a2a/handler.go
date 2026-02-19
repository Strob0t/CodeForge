package a2a

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
)

// Handler serves the A2A protocol endpoints.
type Handler struct {
	baseURL string
	mu      sync.RWMutex
	tasks   map[string]*TaskResponse // In-memory task store (stub)
}

// NewHandler creates an A2A handler.
func NewHandler(baseURL string) *Handler {
	return &Handler{
		baseURL: baseURL,
		tasks:   make(map[string]*TaskResponse),
	}
}

// MountRoutes registers A2A routes on the given chi router.
// These are mounted at the root level, not under /api/v1.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Get("/.well-known/agent.json", h.handleAgentCard)
	r.Post("/a2a/tasks", h.handleCreateTask)
	r.Get("/a2a/tasks/{id}", h.handleGetTask)
}

func (h *Handler) handleAgentCard(w http.ResponseWriter, _ *http.Request) {
	card := BuildAgentCard(h.baseURL)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(card)
}

func (h *Handler) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req TaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		http.Error(w, `{"error":"id is required"}`, http.StatusBadRequest)
		return
	}

	resp := &TaskResponse{
		ID:     req.ID,
		Status: "queued",
	}

	h.mu.Lock()
	h.tasks[req.ID] = resp
	h.mu.Unlock()

	slog.Info("a2a task created", "id", req.ID, "skill", req.Skill)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	h.mu.RLock()
	resp, ok := h.tasks[id]
	h.mu.RUnlock()

	if !ok {
		http.Error(w, `{"error":"task not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

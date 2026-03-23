package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/Strob0t/CodeForge/internal/port/llm"
)

func (h *Handlers) ListLLMModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.LLM.ListModels(r.Context())
	if err != nil {
		slog.Error("litellm unavailable", "error", err)
		writeError(w, http.StatusBadGateway, "LLM service unavailable")
		return
	}
	writeJSONList(w, http.StatusOK, models)
}

// AddLLMModel handles POST /api/v1/llm/models
func (h *Handlers) AddLLMModel(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[llm.AddModelRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.ModelName == "" {
		writeError(w, http.StatusBadRequest, "model_name is required")
		return
	}

	if err := h.LLM.AddModel(r.Context(), req); err != nil {
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
	}](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	if err := h.LLM.DeleteModel(r.Context(), req.ID); err != nil {
		slog.Error("litellm request failed", "error", err)
		writeError(w, http.StatusBadGateway, "LLM service error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// LLMHealth handles GET /api/v1/llm/health
func (h *Handlers) LLMHealth(w http.ResponseWriter, r *http.Request) {
	healthy, err := h.LLM.Health(r.Context())
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
	models, err := h.LLM.DiscoverModels(ctx)
	if err != nil {
		slog.Error("litellm discovery failed", "error", err)
		writeError(w, http.StatusBadGateway, "LLM discovery failed")
		return
	}

	// Discover Ollama models if OLLAMA_BASE_URL is configured.
	if h.OllamaBaseURL != "" {
		ollamaModels, err := h.LLM.DiscoverOllamaModels(ctx, h.OllamaBaseURL)
		if err != nil {
			slog.Warn("ollama discovery failed", "error", err)
			// Non-fatal: continue with LiteLLM models only.
		} else {
			models = append(models, ollamaModels...)
		}
	}

	if models == nil {
		models = []llm.DiscoveredModel{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"models":     models,
		"count":      len(models),
		"ollama_url": h.OllamaBaseURL,
	})
}

// --- Model Registry Handlers (Phase 22) ---

// AvailableLLMModels handles GET /api/v1/llm/available — returns cached model health.
func (h *Handlers) AvailableLLMModels(w http.ResponseWriter, r *http.Request) {
	if h.ModelRegistry == nil {
		writeError(w, http.StatusServiceUnavailable, "model registry not initialized")
		return
	}
	type resp struct {
		Models    []llm.DiscoveredModel `json:"models"`
		BestModel string                `json:"best_model"`
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
	if h.TokenExchanger == nil {
		writeError(w, http.StatusNotFound, "copilot integration not enabled")
		return
	}
	token, expiry, err := h.TokenExchanger.ExchangeToken(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "copilot token exchange failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"token":      token,
		"expires_at": expiry.Format(time.RFC3339),
	})
}

package http

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/llmkey"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/llm"
	"github.com/Strob0t/CodeForge/internal/secrets"
)

func (h *Handlers) ListLLMModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.LLM.ListModels(r.Context())
	if err != nil {
		slog.Error("litellm unavailable", "error", err)
		writeError(w, http.StatusBadGateway, "LLM service unavailable")
		return
	}
	// Every user may list the models: their credentials stay on the server.
	for i := range models {
		models[i].Params = redactCredentials(models[i].Params)
	}
	writeJSONList(w, http.StatusOK, models)
}

// redactCredentials returns the LiteLLM parameters without credentials: a
// parameter whose name holds one (secrets.IsCredentialName: api_key,
// aws_secret_access_key, vertex_credentials, an Authorization header, ...) is
// dropped, and the values are walked (redactParamValue). The parameters are
// the proxy's free-form model config, hence the map of JSON values.
func redactCredentials(params map[string]any) map[string]any {
	if params == nil {
		return nil
	}
	out := make(map[string]any, len(params))
	for name, value := range params {
		if secrets.IsCredentialName(name) {
			continue
		}
		out[name] = redactParamValue(value)
	}
	return out
}

// redactParamValue redacts a JSON parameter value by shape: URL userinfo and
// credential query parameters in strings, credential keys in objects, and the
// elements of arrays. Numbers, booleans and null are kept.
func redactParamValue(value any) any {
	switch v := value.(type) {
	case string:
		return secrets.RedactURL(v)
	case map[string]any:
		return redactCredentials(v)
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = redactParamValue(v[i])
		}
		return out
	default:
		return v
	}
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

// DeleteLLMModel handles DELETE /api/v1/llm/models/{id}
func (h *Handlers) DeleteLLMModel(w http.ResponseWriter, r *http.Request) {
	id, err := decodedURLParam(r, "id")
	if err != nil || strings.TrimSpace(id) == "" {
		writeError(w, http.StatusBadRequest, "a valid model id is required")
		return
	}

	if err := h.LLM.DeleteModel(r.Context(), id); err != nil {
		slog.Error("litellm request failed", "error", err)
		writeError(w, http.StatusBadGateway, "LLM service error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// decodedURLParam returns a path parameter with percent-escapes decoded. chi
// matches routes on the escaped path only when the URL contains escapes that
// normalize differently (such as %2F); only then is the parameter still escaped.
func decodedURLParam(r *http.Request, name string) (string, error) {
	param := chi.URLParam(r, name)
	if r.URL.RawPath == "" {
		return param, nil
	}
	return url.PathUnescape(param)
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

	type discoverModelsResponse struct {
		Models    []llm.DiscoveredModel `json:"models"`
		Count     int                   `json:"count"`
		OllamaURL string                `json:"ollama_url"`
	}
	writeJSON(w, http.StatusOK, discoverModelsResponse{
		Models:    models,
		Count:     len(models),
		OllamaURL: h.OllamaBaseURL,
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

// HandleCopilotExchange handles POST /api/v1/copilot/exchange (platform
// admins only): it checks that the platform's GitHub Copilot credential can
// be exchanged. The bearer token is the platform's credential and never
// leaves the server (KI-80); the response carries only the status and the
// expiry.
func (h *Handlers) HandleCopilotExchange(w http.ResponseWriter, r *http.Request) {
	if h.TokenExchanger == nil {
		writeError(w, http.StatusNotFound, "copilot integration not enabled")
		return
	}
	_, expiry, err := h.TokenExchanger.ExchangeToken(r.Context())
	if err != nil {
		slog.Warn("copilot token exchange failed", "error", err)
		writeError(w, http.StatusBadGateway, "copilot token exchange failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":     "ok",
		"expires_at": expiry.Format(time.RFC3339),
	})
}

// --- LLM Keys ---

// ListLLMKeys handles GET /api/v1/llm-keys
func (h *Handlers) ListLLMKeys(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	keys, err := h.LLMKeys.List(r.Context(), u.ID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSONList(w, http.StatusOK, keys)
}

// CreateLLMKey handles POST /api/v1/llm-keys
func (h *Handlers) CreateLLMKey(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	req, ok := readJSON[llmkey.CreateRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}

	key, err := h.LLMKeys.Create(r.Context(), u.ID, req)
	if err != nil {
		writeDomainError(w, err, "create llm key failed")
		return
	}
	writeJSON(w, http.StatusCreated, key)
}

// DeleteLLMKey handles DELETE /api/v1/llm-keys/{id}
func (h *Handlers) DeleteLLMKey(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.LLMKeys.Delete(r.Context(), id, u.ID); err != nil {
		writeDomainError(w, err, "llm key not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

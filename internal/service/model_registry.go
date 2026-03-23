package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/llm"
)

// ModelRegistry maintains an in-memory cache of discovered LLM models
// with their health status, periodically refreshed via LiteLLM.
//
// ModelRegistry is intentionally shared across all tenants as an infrastructure-layer
// component. LiteLLM and Ollama are shared sidecar services; model discovery data
// (availability, capabilities, costs) is public provider information, not tenant-specific.
// Tenant isolation is enforced at the application layer: routing stats and outcomes in
// the database are scoped via tenantFromCtx() in store_routing.go.
type ModelRegistry struct {
	mu          sync.RWMutex
	models      []llm.DiscoveredModel
	bestModel   string
	lastRefresh time.Time
	interval    time.Duration
	discoverer  llm.ModelDiscoverer
	hub         broadcast.Broadcaster
	ollamaURL   string // from OLLAMA_BASE_URL env
	routingSvc  *RoutingService
}

// NewModelRegistry creates a new registry with the given poll interval.
// Pass interval <= 0 to disable periodic polling (manual refresh only).
// The ollamaURL parameter comes from cfg.Ollama.BaseURL (OLLAMA_BASE_URL env var).
func NewModelRegistry(discoverer llm.ModelDiscoverer, hub broadcast.Broadcaster, interval time.Duration, ollamaURL string) *ModelRegistry {
	return &ModelRegistry{
		discoverer: discoverer,
		hub:        hub,
		interval:   interval,
		ollamaURL:  ollamaURL,
	}
}

// Start launches the background refresh goroutine. The first refresh is
// performed synchronously so the caller has models available immediately.
// Subsequent refreshes happen on the configured interval until ctx is cancelled.
func (r *ModelRegistry) Start(ctx context.Context) {
	// Synchronous first refresh.
	if err := r.Refresh(ctx); err != nil {
		slog.Warn("model registry: initial refresh failed", "error", err)
	}

	if r.interval <= 0 {
		return
	}

	go func() {
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := r.Refresh(ctx); err != nil {
					slog.Warn("model registry: periodic refresh failed", "error", err)
				}
			}
		}
	}()
}

// Refresh discovers models from LiteLLM (and Ollama if configured),
// updates the cache, and broadcasts a WS event if the best model changed.
func (r *ModelRegistry) Refresh(ctx context.Context) error {
	models, err := r.discoverer.DiscoverModels(ctx)
	if err != nil {
		return err
	}

	// Discover Ollama models if configured.
	if r.ollamaURL != "" {
		ollamaModels, ollamaErr := r.discoverer.DiscoverOllamaModels(ctx, r.ollamaURL)
		if ollamaErr != nil {
			slog.Warn("model registry: ollama discovery failed", "error", ollamaErr)
		} else {
			models = append(models, ollamaModels...)
		}
	}

	if models == nil {
		models = []llm.DiscoveredModel{}
	}

	// Filter reachable models for best-model selection.
	reachable := make([]llm.DiscoveredModel, 0, len(models))
	for i := range models {
		if models[i].Status == "reachable" {
			reachable = append(reachable, models[i])
		}
	}
	newBest := llm.SelectStrongestModel(reachable)

	r.mu.Lock()
	oldBest := r.bestModel
	r.models = models
	r.bestModel = newBest
	r.lastRefresh = time.Now()
	r.mu.Unlock()

	if oldBest != newBest {
		slog.Info("model registry: best model changed", "old", oldBest, "new", newBest)
	}

	r.broadcastHealth(ctx, models, newBest)

	// Sync model capabilities into routing stats.
	if r.routingSvc != nil {
		go func() {
			bgCtx := context.WithoutCancel(ctx)
			if err := r.routingSvc.SyncModelCapabilities(bgCtx, models); err != nil {
				slog.Warn("model registry: sync routing capabilities failed", "error", err)
			}
		}()
	}

	return nil
}

// SetRoutingService sets the routing service for model capabilities sync.
func (r *ModelRegistry) SetRoutingService(routingSvc *RoutingService) {
	r.routingSvc = routingSvc
}

// AvailableModels returns a copy of the cached discovered models.
func (r *ModelRegistry) AvailableModels() []llm.DiscoveredModel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]llm.DiscoveredModel, len(r.models))
	copy(out, r.models)
	return out
}

// BestModel returns the current strongest reachable model name.
func (r *ModelRegistry) BestModel() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.bestModel
}

// LastRefresh returns when the registry was last refreshed.
func (r *ModelRegistry) LastRefresh() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastRefresh
}

// IsHealthy checks if a specific model is currently reachable.
func (r *ModelRegistry) IsHealthy(modelName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for i := range r.models {
		if r.models[i].ModelName == modelName {
			return r.models[i].Status == "reachable"
		}
	}
	return false
}

// ValidateConfiguredModel logs a warning if the given model name is not among
// the currently discovered models. This is a non-blocking startup check.
func (r *ModelRegistry) ValidateConfiguredModel(modelName string) {
	if modelName == "" {
		return
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for i := range r.models {
		if r.models[i].ModelName == modelName {
			return
		}
	}
	slog.Warn("configured model not found in discovered models",
		"model", modelName,
		"discovered_count", len(r.models),
	)
}

// broadcastHealth sends a models.health WS event with current state.
func (r *ModelRegistry) broadcastHealth(ctx context.Context, models []llm.DiscoveredModel, bestModel string) {
	if r.hub == nil {
		return
	}

	entries := make([]ws.ModelHealthEntry, len(models))
	healthy, unhealthy := 0, 0
	for i := range models {
		entries[i] = ws.ModelHealthEntry{
			ModelName:   models[i].ModelName,
			Status:      models[i].Status,
			Provider:    models[i].Provider,
			ErrorDetail: models[i].ErrorDetail,
			Source:      models[i].Source,
		}
		if models[i].Status == "reachable" {
			healthy++
		} else {
			unhealthy++
		}
	}

	r.hub.BroadcastEvent(ctx, ws.EventModelHealth, ws.ModelHealthEvent{
		Models:         entries,
		BestModel:      bestModel,
		HealthyCount:   healthy,
		UnhealthyCount: unhealthy,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
	})
}

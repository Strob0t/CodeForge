package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// ListSubscriptionProviders handles GET /api/v1/auth/providers
func (h *Handlers) ListSubscriptionProviders(w http.ResponseWriter, _ *http.Request) {
	if h.Subscription == nil {
		writeError(w, http.StatusNotImplemented, "subscription providers not configured")
		return
	}

	providers := h.Subscription.ListProviders()
	writeJSON(w, http.StatusOK, map[string]any{"providers": providers})
}

// StartProviderConnect handles POST /api/v1/auth/providers/{provider}/connect
func (h *Handlers) StartProviderConnect(w http.ResponseWriter, r *http.Request) {
	if h.Subscription == nil {
		writeError(w, http.StatusNotImplemented, "subscription providers not configured")
		return
	}

	provider := chi.URLParam(r, "provider")
	if provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}

	dc, err := h.Subscription.StartConnect(r.Context(), provider)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, dc)
}

// GetProviderStatus handles GET /api/v1/auth/providers/{provider}/status
func (h *Handlers) GetProviderStatus(w http.ResponseWriter, r *http.Request) {
	if h.Subscription == nil {
		writeError(w, http.StatusNotImplemented, "subscription providers not configured")
		return
	}

	provider := chi.URLParam(r, "provider")
	if provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}

	status := h.Subscription.GetStatus(provider)
	writeJSON(w, http.StatusOK, status)
}

// DisconnectProvider handles DELETE /api/v1/auth/providers/{provider}/disconnect
func (h *Handlers) DisconnectProvider(w http.ResponseWriter, r *http.Request) {
	if h.Subscription == nil {
		writeError(w, http.StatusNotImplemented, "subscription providers not configured")
		return
	}

	provider := chi.URLParam(r, "provider")
	if provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}

	if err := h.Subscription.Disconnect(provider); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

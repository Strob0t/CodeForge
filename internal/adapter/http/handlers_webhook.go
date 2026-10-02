package http

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/webhook"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
)

// --- Webhook registration (KI-85) ---

// webhookAuditUnavailable answers a change whose audit entry could not be
// written: nothing was changed.
func webhookAuditUnavailable(w http.ResponseWriter) {
	writeError(w, http.StatusServiceUnavailable, "the audit log is unavailable; nothing was changed")
}

// RegisterWebhook handles POST /api/v1/projects/{id}/webhooks (admins). The
// answer carries the webhook's secret, the only time it is shown.
func (h *Handlers) RegisterWebhook(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[webhook.CreateRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	reg, err := h.Webhooks.Register(r.Context(), chi.URLParam(r, "id"), &req)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusCreated, reg)
}

// ListWebhooks handles GET /api/v1/projects/{id}/webhooks (admins,
// editors): IDs, URLs, kinds and providers, never secrets.
func (h *Handlers) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	list, err := h.Webhooks.List(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSONList(w, http.StatusOK, list)
}

// RotateWebhookSecret handles POST /api/v1/projects/{id}/webhooks/{webhookId}/rotate
// (admins): a new secret, shown once; the old one stops working.
func (h *Handlers) RotateWebhookSecret(w http.ResponseWriter, r *http.Request) {
	projectID, id := chi.URLParam(r, "id"), chi.URLParam(r, "webhookId")
	middleware.AuditContext(r.Context(), map[string]string{"project_id": projectID})
	if err := middleware.RecordAudit(r.Context(), id, nil); err != nil {
		webhookAuditUnavailable(w)
		return
	}
	reg, err := h.Webhooks.RotateSecret(r.Context(), projectID, id)
	if err != nil {
		writeDomainError(w, err, "webhook not found")
		return
	}
	writeJSON(w, http.StatusOK, reg)
}

// setWebhookAPITokenRequest is the body of PUT .../api-token.
type setWebhookAPITokenRequest struct {
	APIToken string `json:"api_token"` //nolint:gosec // G117: request field, stored encrypted
}

// SetWebhookAPIToken handles PUT /api/v1/projects/{id}/webhooks/{webhookId}/api-token
// (admins): the PM integration's token for the provider's API ("" removes
// it).
func (h *Handlers) SetWebhookAPIToken(w http.ResponseWriter, r *http.Request) {
	projectID, id := chi.URLParam(r, "id"), chi.URLParam(r, "webhookId")
	middleware.AuditContext(r.Context(), map[string]string{"project_id": projectID})
	req, ok := readJSON[setWebhookAPITokenRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if err := middleware.RecordAudit(r.Context(), id, nil); err != nil {
		webhookAuditUnavailable(w)
		return
	}
	if err := h.Webhooks.SetAPIToken(r.Context(), projectID, id, req.APIToken); err != nil {
		writeDomainError(w, err, "webhook not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteWebhook handles DELETE /api/v1/projects/{id}/webhooks/{webhookId}
// (admins).
func (h *Handlers) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	projectID, id := chi.URLParam(r, "id"), chi.URLParam(r, "webhookId")
	middleware.AuditContext(r.Context(), map[string]string{"project_id": projectID})
	if err := middleware.RecordAudit(r.Context(), id, nil); err != nil {
		webhookAuditUnavailable(w)
		return
	}
	if err := h.Webhooks.Delete(r.Context(), projectID, id); err != nil {
		writeDomainError(w, err, "webhook not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Webhook deliveries (KI-85) ---

// deliveryHeaders names the headers a provider sends its event type,
// delivery ID and signature in.
var deliveryHeaders = map[string]struct{ event, delivery, signature string }{
	"github": {"X-GitHub-Event", "X-GitHub-Delivery", "X-Hub-Signature-256"},
	"gitlab": {"X-Gitlab-Event", "X-Gitlab-Event-UUID", "X-Gitlab-Token"},
	"plane":  {"X-Plane-Event", "X-Plane-Delivery", "X-Plane-Signature"},
}

// ReceiveVCSWebhook handles POST /api/v1/webhooks/vcs/{provider}/{webhookId}.
func (h *Handlers) ReceiveVCSWebhook(w http.ResponseWriter, r *http.Request) {
	h.receiveWebhook(w, r, webhook.KindVCS)
}

// ReceivePMWebhook handles POST /api/v1/webhooks/pm/{provider}/{webhookId}.
func (h *Handlers) ReceivePMWebhook(w http.ResponseWriter, r *http.Request) {
	h.receiveWebhook(w, r, webhook.KindPM)
}

// receiveWebhook hands a delivery to its webhook. Answers: 200 for a
// handled VCS event, an ignored event or a duplicate delivery; 202 when a
// PM sync started (its outcome is a pm.sync event); 400 for a payload or a
// sync the provider cannot run; one uniform 401 for an unknown webhook or a
// wrong signature. The webhook's registration names the tenant; the
// X-Tenant-ID header is never read here.
func (h *Handlers) receiveWebhook(w http.ResponseWriter, r *http.Request, kind webhook.Kind) {
	body := readBody(w, r)
	if body == nil {
		return
	}
	provider := chi.URLParam(r, "provider")
	names := deliveryHeaders[provider]
	d := &webhook.Delivery{Body: body}
	if names.event != "" {
		d.Event = r.Header.Get(names.event)
		d.DeliveryID = r.Header.Get(names.delivery)
		d.Signature = r.Header.Get(names.signature)
	}
	res, err := h.Webhooks.Receive(r.Context(), kind, provider, chi.URLParam(r, "webhookId"), d)
	switch {
	case errors.Is(err, service.ErrWebhookUnauthorized):
		writeError(w, http.StatusUnauthorized, service.ErrWebhookUnauthorized.Error())
	case err != nil:
		writeDomainError(w, err, "webhook processing failed")
	case res.Status == webhook.InboundAccepted:
		writeJSON(w, http.StatusAccepted, res)
	default:
		writeJSON(w, http.StatusOK, res)
	}
}

// RemovedWebhookRoute answers the global webhook routes removed with KI-85:
// their secrets were global and they acted in the default tenant (or the one
// an X-Tenant-ID header named). A provider still delivering there learns
// why from its delivery log.
func (h *Handlers) RemovedWebhookRoute(w http.ResponseWriter, r *http.Request) {
	slog.WarnContext(r.Context(), "delivery to a removed global webhook route - register a webhook per project",
		"path", r.URL.Path, "remote", r.RemoteAddr)
	writeError(w, http.StatusGone,
		"this global webhook route was removed: register a webhook per project (POST /api/v1/projects/{id}/webhooks) and deliver to its URL")
}

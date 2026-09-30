package http

import (
	"crypto/hmac"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/channel"
	"github.com/Strob0t/CodeForge/internal/middleware"
)

// ListChannels handles GET /api/v1/channels
func (h *Handlers) ListChannels(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	channels, err := h.Channels.List(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "list channels")
		return
	}
	writeJSONList(w, http.StatusOK, channels)
}

// CreateChannel handles POST /api/v1/channels
func (h *Handlers) CreateChannel(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[channel.Channel](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	ch, err := h.Channels.Create(r.Context(), &req)
	if err != nil {
		writeDomainError(w, err, "create channel")
		return
	}
	writeJSON(w, http.StatusCreated, ch)
}

// GetChannel handles GET /api/v1/channels/{id}
func (h *Handlers) GetChannel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ch, err := h.Channels.Get(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "channel not found")
		return
	}
	writeJSON(w, http.StatusOK, ch)
}

// DeleteChannel handles DELETE /api/v1/channels/{id}
func (h *Handlers) DeleteChannel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Channels.Delete(r.Context(), id); err != nil {
		writeDomainError(w, err, "delete channel")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListChannelMessages handles GET /api/v1/channels/{id}/messages
func (h *Handlers) ListChannelMessages(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "id")
	cursor := r.URL.Query().Get("cursor")
	limit := queryParamIntClamped(r, "limit", 50, 500)

	messages, err := h.Channels.ListMessages(r.Context(), channelID, cursor, limit)
	if err != nil {
		writeDomainError(w, err, "list channel messages")
		return
	}
	writeJSONList(w, http.StatusOK, messages)
}

// asAuthenticatedSender attributes a message to the calling user. Sender
// fields in the request body are ignored: messages are broadcast live to the
// whole tenant, so a client must not pose as an agent or another user.
func asAuthenticatedSender(w http.ResponseWriter, r *http.Request, msg *channel.Message) bool {
	u := middleware.UserFromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return false
	}
	msg.SenderType = channel.SenderUser
	msg.SenderID = u.ID
	msg.SenderName = u.Name
	return true
}

// SendChannelMessage handles POST /api/v1/channels/{id}/messages
func (h *Handlers) SendChannelMessage(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "id")
	req, ok := readJSON[channel.Message](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if !asAuthenticatedSender(w, r, &req) {
		return
	}
	req.ChannelID = channelID

	msg, err := h.Channels.SendMessage(r.Context(), &req)
	if err != nil {
		writeDomainError(w, err, "send channel message")
		return
	}
	writeJSON(w, http.StatusCreated, msg)
}

// SendThreadReply handles POST /api/v1/channels/{id}/messages/{mid}/thread
func (h *Handlers) SendThreadReply(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "id")
	parentID := chi.URLParam(r, "mid")

	req, ok := readJSON[channel.Message](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if !asAuthenticatedSender(w, r, &req) {
		return
	}
	req.ChannelID = channelID
	req.ParentID = parentID

	msg, err := h.Channels.SendMessage(r.Context(), &req)
	if err != nil {
		writeDomainError(w, err, "send thread reply")
		return
	}
	writeJSON(w, http.StatusCreated, msg)
}

// UpdateMemberNotify handles PUT /api/v1/channels/{id}/members/{uid}
func (h *Handlers) UpdateMemberNotify(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "id")
	userID := chi.URLParam(r, "uid")

	type notifyRequest struct {
		Notify channel.NotifySetting `json:"notify"`
	}

	req, ok := readJSON[notifyRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}

	if err := h.Channels.UpdateMemberNotify(r.Context(), channelID, userID, req.Notify); err != nil {
		writeDomainError(w, err, "update member notify")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// WebhookMessage handles POST /api/v1/channels/{id}/webhook
func (h *Handlers) WebhookMessage(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "id")

	webhookKey := r.Header.Get("X-Webhook-Key")
	if webhookKey == "" {
		writeError(w, http.StatusUnauthorized, "X-Webhook-Key header is required")
		return
	}

	// Validate the webhook key against the channel's stored key.
	ch, err := h.Channels.Get(r.Context(), channelID)
	if err != nil {
		writeDomainError(w, err, "channel not found")
		return
	}
	if !hmac.Equal([]byte(webhookKey), []byte(ch.WebhookKey)) {
		writeError(w, http.StatusForbidden, "invalid webhook key")
		return
	}

	req, ok := readJSON[channel.Message](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	req.ChannelID = channelID
	req.SenderType = channel.SenderWebhook
	req.SenderID = "" // a webhook is not a user; its sender_name is only a display label

	msg, err := h.Channels.SendMessage(r.Context(), &req)
	if err != nil {
		writeDomainError(w, err, "webhook message")
		return
	}
	writeJSON(w, http.StatusCreated, msg)
}

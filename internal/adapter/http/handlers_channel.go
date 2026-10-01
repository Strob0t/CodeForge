package http

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/channel"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
)

// ListChannels handles GET /api/v1/channels (with the caller's unread counts).
func (h *Handlers) ListChannels(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	userID := ""
	if u := middleware.UserFromContext(r.Context()); u != nil {
		userID = u.ID
	}
	channels, err := h.Channels.List(r.Context(), projectID, userID)
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

// RegenerateChannelWebhookKey handles POST /api/v1/channels/{id}/webhook-key
// (admins): it makes a new webhook key and returns it once; a previous key
// stops working.
func (h *Handlers) RegenerateChannelWebhookKey(w http.ResponseWriter, r *http.Request) {
	key, err := h.Channels.RegenerateWebhookKey(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeDomainError(w, err, "generate webhook key")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"webhook_key": key})
}

// MarkChannelRead handles POST /api/v1/channels/{id}/read: it moves the
// caller's read position to a message (204 when the caller has no account
// row, whose read position is not tracked).
func (h *Handlers) MarkChannelRead(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	type markReadRequest struct {
		MessageID string `json:"message_id"`
	}
	req, ok := readJSON[markReadRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	state, err := h.Channels.MarkRead(r.Context(), chi.URLParam(r, "id"), u.ID, req.MessageID)
	if errors.Is(err, channel.ErrReadStateNotTracked) {
		// No account row (auth disabled, internal service key): nothing is
		// stored and nothing is broadcast.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		writeDomainError(w, err, "mark channel read")
		return
	}
	writeJSON(w, http.StatusOK, state)
}

// ListChannelReadStates handles GET /api/v1/channels/{id}/read.
func (h *Handlers) ListChannelReadStates(w http.ResponseWriter, r *http.Request) {
	states, err := h.Channels.ListReadStates(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeDomainError(w, err, "list channel read states")
		return
	}
	writeJSONList(w, http.StatusOK, states)
}

// WebhookMessage handles POST /api/v1/webhooks/channels/{id}: a public
// endpoint for external systems, authenticated by the channel's webhook key
// (X-Webhook-Key) and handled in the channel's tenant.
func (h *Handlers) WebhookMessage(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "id")

	webhookKey := r.Header.Get("X-Webhook-Key")
	if webhookKey == "" {
		writeError(w, http.StatusUnauthorized, "X-Webhook-Key header is required")
		return
	}
	ctx, err := h.Channels.AuthorizeWebhook(r.Context(), channelID, webhookKey)
	if errors.Is(err, service.ErrWebhookForbidden) {
		writeError(w, http.StatusForbidden, "invalid webhook key")
		return
	}
	if err != nil {
		writeInternalError(w, err)
		return
	}

	req, ok := readJSON[channel.Message](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	req.ChannelID = channelID
	req.SenderType = channel.SenderWebhook
	req.SenderID = "" // a webhook is not a user; its sender_name is only a display label
	req.ParentID = ""

	msg, err := h.Channels.SendMessage(ctx, &req)
	if err != nil {
		writeDomainError(w, err, "webhook message")
		return
	}
	writeJSON(w, http.StatusCreated, msg)
}

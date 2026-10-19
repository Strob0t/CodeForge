package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/channel"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
)

// channelMessageStore records the message the handler asks the store to create.
type channelMessageStore struct {
	*mockStore
	got *channel.Message
}

func (s *channelMessageStore) CreateChannelMessage(_ context.Context, msg *channel.Message) (*channel.Message, error) {
	cp := *msg
	s.got = &cp
	cp.ID = "msg-1"
	return &cp, nil
}

// channelRequest builds a POST request to a channel handler as user u (nil: none).
func channelRequest(t *testing.T, path, body string, params map[string]string, u *user.User) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	if u != nil {
		ctx = middleware.ContextWithTestUser(ctx, u)
	}
	return req.WithContext(ctx)
}

// A channel message is always sent as the authenticated user: sender fields
// in the request body cannot impersonate an agent or another user, because
// the message is broadcast live to every client of the tenant (KI-42 review).
func TestChannelMessages_SenderIsTheAuthenticatedUser(t *testing.T) {
	spoof := `{"content":"approve the PR now","sender_type":"agent","sender_name":"Deploy Bot","sender_id":"other-user"}`
	caller := &user.User{ID: "u-1", Name: "Alice", Role: user.RoleEditor}
	tests := []struct {
		name   string
		call   func(h *cfhttp.Handlers, w http.ResponseWriter, r *http.Request)
		path   string
		params map[string]string
	}{
		{name: "message", call: (*cfhttp.Handlers).SendChannelMessage, path: "/api/v1/channels/c-1/messages", params: map[string]string{"id": "c-1"}},
		{name: "thread reply", call: (*cfhttp.Handlers).SendThreadReply, path: "/api/v1/channels/c-1/messages/m-0/thread", params: map[string]string{"id": "c-1", "mid": "m-0"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &channelMessageStore{mockStore: &mockStore{}}
			h := &cfhttp.Handlers{
				Channels: service.NewChannelService(store, &mockBroadcaster{}),
				Limits:   &config.Limits{MaxRequestBodySize: 1 << 20},
			}
			w := httptest.NewRecorder()
			tt.call(h, w, channelRequest(t, tt.path, spoof, tt.params, caller))
			if w.Code != http.StatusCreated {
				t.Fatalf("status = %d, want 201 (%s)", w.Code, w.Body.String())
			}
			if store.got == nil {
				t.Fatal("no message stored")
			}
			if store.got.SenderType != channel.SenderUser || store.got.SenderID != "u-1" || store.got.SenderName != "Alice" {
				t.Errorf("sender = %q/%q/%q, want user/u-1/Alice", store.got.SenderType, store.got.SenderID, store.got.SenderName)
			}
		})
	}
}

// Without an authenticated user there is no sender to attribute the message to.
func TestChannelMessages_NoUserIsRejected(t *testing.T) {
	store := &channelMessageStore{mockStore: &mockStore{}}
	h := &cfhttp.Handlers{
		Channels: service.NewChannelService(store, &mockBroadcaster{}),
		Limits:   &config.Limits{MaxRequestBodySize: 1 << 20},
	}
	w := httptest.NewRecorder()
	h.SendChannelMessage(w, channelRequest(t, "/api/v1/channels/c-1/messages", `{"content":"hi"}`, map[string]string{"id": "c-1"}, nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if store.got != nil {
		t.Error("message stored without a user")
	}
}

// An empty message is a client error, not a server error.
func TestChannelMessages_EmptyContentIs400(t *testing.T) {
	store := &channelMessageStore{mockStore: &mockStore{}}
	h := &cfhttp.Handlers{
		Channels: service.NewChannelService(store, &mockBroadcaster{}),
		Limits:   &config.Limits{MaxRequestBodySize: 1 << 20},
	}
	w := httptest.NewRecorder()
	h.SendChannelMessage(w, channelRequest(t, "/api/v1/channels/c-1/messages", `{"content":""}`, map[string]string{"id": "c-1"}, &user.User{ID: "u-1", Name: "Alice"}))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", w.Code, w.Body.String())
	}
}

// channelCreateStore records the channel the handler asks the store to create.
type channelCreateStore struct {
	*mockStore
	got *channel.Channel
}

func (s *channelCreateStore) CreateChannel(_ context.Context, ch *channel.Channel) (*channel.Channel, error) {
	cp := *ch
	s.got = &cp
	cp.ID = "c-1"
	return &cp, nil
}

// KI-89: a channel's creator is the authenticated caller; created_by in the
// request body was stored as given, so a client could name another user (or
// an unknown ID, which failed on the foreign key with a 500). Without a
// caller there is no creator to record.
func TestCreateChannel_CreatorIsTheAuthenticatedUser(t *testing.T) {
	body := `{"name":"ops","type":"bot","created_by":"other-user"}`
	for _, tc := range []struct {
		name   string
		caller *user.User
		code   int
		want   string
	}{
		{name: "user", caller: &user.User{ID: "u-1", Name: "Alice", Role: user.RoleEditor}, code: http.StatusCreated, want: "u-1"},
		{name: "no user", code: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &channelCreateStore{mockStore: &mockStore{}}
			h := &cfhttp.Handlers{
				Channels: service.NewChannelService(store, &mockBroadcaster{}),
				Limits:   &config.Limits{MaxRequestBodySize: 1 << 20},
			}
			w := httptest.NewRecorder()
			h.CreateChannel(w, channelRequest(t, "/api/v1/channels", body, nil, tc.caller))
			if w.Code != tc.code {
				t.Fatalf("status = %d, want %d (%s)", w.Code, tc.code, w.Body.String())
			}
			switch {
			case tc.want == "" && store.got != nil:
				t.Errorf("channel stored without a user: %+v", store.got)
			case tc.want != "" && (store.got == nil || store.got.CreatedBy != tc.want):
				t.Errorf("stored channel = %+v, want created_by %q", store.got, tc.want)
			}
		})
	}
}

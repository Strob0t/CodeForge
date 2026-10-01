package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/channel"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const channelTestTenant = "aaaaaaaa-0000-4000-8000-000000000001"

// accountlessUserID is the default user while auth is disabled: it has no
// account row.
const accountlessUserID = "00000000-0000-0000-0000-000000000000"

// channelHTTPStore keeps one channel of channelTestTenant.
type channelHTTPStore struct {
	*mockStore
	hash     []byte
	posted   []channel.Message
	postedIn []string // tenant of each post
	reads    []channel.ReadState
}

func (s *channelHTTPStore) SetChannelWebhookKeyHash(ctx context.Context, channelID string, hash []byte) error {
	if channelID != "ch-1" || tenantctx.FromContext(ctx) != channelTestTenant {
		return domain.ErrNotFound
	}
	s.hash = hash
	return nil
}

func (s *channelHTTPStore) GetChannelWebhookKeyHash(_ context.Context, channelID string) (tenantID string, hash []byte, err error) {
	if channelID != "ch-1" {
		return "", nil, domain.ErrNotFound
	}
	return channelTestTenant, s.hash, nil
}

func (s *channelHTTPStore) CreateChannelMessage(ctx context.Context, msg *channel.Message) (*channel.Message, error) {
	created := *msg
	created.ID = "msg-1"
	s.posted = append(s.posted, created)
	s.postedIn = append(s.postedIn, tenantctx.FromContext(ctx))
	return &created, nil
}

func (s *channelHTTPStore) MarkChannelRead(_ context.Context, channelID, userID, messageID string) (*channel.ReadState, error) {
	if userID == accountlessUserID {
		return nil, channel.ErrReadStateNotTracked
	}
	rs := channel.ReadState{ChannelID: channelID, UserID: userID, LastReadMessageID: messageID, LastReadAt: time.Unix(1, 0).UTC()}
	s.reads = append(s.reads, rs)
	return &rs, nil
}

func (s *channelHTTPStore) ListChannelReadStates(_ context.Context, _ string) ([]channel.ReadState, error) {
	return s.reads, nil
}

func channelRouter(store *channelHTTPStore) http.Handler {
	return newTestRouterWithLLM(store.mockStore, service.NewPolicyService("headless-safe-sandbox", nil), "http://localhost:4000",
		func(h *cfhttp.Handlers) { h.Channels = service.NewChannelService(store, &mockBroadcaster{}) })
}

func channelRouteRequest(method, path, body string, u *user.User) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	if u != nil {
		ctx := middleware.ContextWithTestUser(req.Context(), u)
		req = req.WithContext(tenantctx.WithTenant(ctx, u.TenantID))
	}
	return req
}

// KI-73: an admin generates a channel's webhook key (shown once); external
// systems post with it to the public webhook endpoint, without a user.
func TestChannelWebhook_KeyAndDelivery(t *testing.T) {
	store := &channelHTTPStore{mockStore: &mockStore{}}
	router := channelRouter(store)
	admin := &user.User{ID: "u-admin", Name: "Admin", Role: user.RoleAdmin, TenantID: channelTestTenant}
	editor := &user.User{ID: "u-editor", Name: "Ed", Role: user.RoleEditor, TenantID: channelTestTenant}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, channelRouteRequest(http.MethodPost, "/api/v1/channels/ch-1/webhook-key", "", editor))
	if w.Code != http.StatusForbidden {
		t.Fatalf("editor generated a key: %d", w.Code)
	}

	w = httptest.NewRecorder()
	router.ServeHTTP(w, channelRouteRequest(http.MethodPost, "/api/v1/channels/ch-1/webhook-key", "", admin))
	if w.Code != http.StatusCreated {
		t.Fatalf("generate key: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		WebhookKey string `json:"webhook_key"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil || len(resp.WebhookKey) != 64 {
		t.Fatalf("key response %+v, %v", resp, err)
	}

	post := func(key, body string) *httptest.ResponseRecorder {
		req := channelRouteRequest(http.MethodPost, "/api/v1/webhooks/channels/ch-1", body, nil)
		if key != "" {
			req.Header.Set("X-Webhook-Key", key)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	if w := post("", `{"content":"x"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("no key: %d", w.Code)
	}
	if w := post("0000", `{"content":"x"}`); w.Code != http.StatusForbidden {
		t.Fatalf("wrong key: %d", w.Code)
	}
	if len(store.posted) != 0 {
		t.Fatal("a rejected webhook call posted a message")
	}
	w = post(resp.WebhookKey, `{"content":"build ok","sender_name":"ci","sender_type":"user","sender_id":"u-admin"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("webhook post: %d %s", w.Code, w.Body.String())
	}
	got := store.posted[0]
	if got.SenderType != channel.SenderWebhook || got.SenderID != "" || got.SenderName != "ci" || got.Content != "build ok" {
		t.Fatalf("posted %+v", got)
	}
	if store.postedIn[0] != channelTestTenant {
		t.Fatalf("webhook message stored in tenant %q, want the channel's", store.postedIn[0])
	}

	w = httptest.NewRecorder()
	router.ServeHTTP(w, channelRouteRequest(http.MethodPost, "/api/v1/channels/ch-1/webhook", `{"content":"x"}`, admin))
	if w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("the old authenticated webhook path still answers %d", w.Code)
	}
}

// KI-73: a user's read position is stored per channel and listed.
func TestChannelRead_MarkAndList(t *testing.T) {
	store := &channelHTTPStore{mockStore: &mockStore{}}
	router := channelRouter(store)
	viewer := &user.User{ID: "u-viewer", Name: "V", Role: user.RoleViewer, TenantID: channelTestTenant}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, channelRouteRequest(http.MethodPost, "/api/v1/channels/ch-1/read", `{"message_id":"msg-9","user_id":"someone-else"}`, viewer))
	if w.Code != http.StatusOK {
		t.Fatalf("mark read: %d %s", w.Code, w.Body.String())
	}
	if len(store.reads) != 1 || store.reads[0].UserID != "u-viewer" || store.reads[0].LastReadMessageID != "msg-9" {
		t.Fatalf("reads = %+v (the caller's own position only)", store.reads)
	}

	w = httptest.NewRecorder()
	router.ServeHTTP(w, channelRouteRequest(http.MethodPost, "/api/v1/channels/ch-1/read", `{}`, viewer))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("mark read without message: %d", w.Code)
	}

	w = httptest.NewRecorder()
	router.ServeHTTP(w, channelRouteRequest(http.MethodGet, "/api/v1/channels/ch-1/read", "", viewer))
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"last_read_message_id":"msg-9"`)) {
		t.Fatalf("list read states: %d %s", w.Code, w.Body.String())
	}
}

// S6-H review 1: the read position of a user without an account row (auth
// disabled, internal service key) is not tracked; marking read is a no-op
// (204) instead of a 500 from the foreign key.
func TestChannelRead_AccountlessUserIsNoOp(t *testing.T) {
	store := &channelHTTPStore{mockStore: &mockStore{}}
	router := channelRouter(store)
	admin := &user.User{ID: accountlessUserID, Name: "Admin", Role: user.RoleAdmin, TenantID: channelTestTenant}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, channelRouteRequest(http.MethodPost, "/api/v1/channels/ch-1/read", `{"message_id":"msg-9"}`, admin))
	if w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatalf("mark read without an account = %d %q, want 204 and no body", w.Code, w.Body.String())
	}
	if len(store.reads) != 0 {
		t.Fatalf("reads = %+v, want none", store.reads)
	}
}

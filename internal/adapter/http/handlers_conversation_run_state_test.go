package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/service"
)

func getJSON(t *testing.T, router http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// KI-148: on page load the chat reads the conversation's running turn and
// pending approvals from GET /conversations/{id}/run.
func TestGetConversationRunState(t *testing.T) {
	store := &mockStore{}
	store.convs = append(store.convs,
		conversation.Conversation{ID: "conv-idle", ProjectID: "proj-1"},
		conversation.Conversation{ID: "conv-busy", ProjectID: "proj-1", ActiveTurnID: "turn-1"},
	)
	r := newTestRouterWithStore(store)

	if w := getJSON(t, r, "/api/v1/conversations/no-such-conv/run"); w.Code != http.StatusNotFound {
		t.Fatalf("unknown conversation: %d %s, want 404", w.Code, w.Body.String())
	}

	for _, tc := range []struct {
		id         string
		wantActive bool
		wantTurn   string
	}{
		{"conv-idle", false, ""},
		{"conv-busy", true, "turn-1"},
	} {
		w := getJSON(t, r, "/api/v1/conversations/"+tc.id+"/run")
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s, want 200", tc.id, w.Code, w.Body.String())
		}
		var got service.ConversationRunState
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("%s: decode: %v", tc.id, err)
		}
		if got.Active != tc.wantActive || got.TurnID != tc.wantTurn || got.PendingApprovals == nil {
			t.Fatalf("%s: run state %s", tc.id, w.Body.String())
		}
	}
}

// KI-148: a conversation without a session (never forked or resumed) is the
// normal case and answered 204, so the chat's load does not log a 404; an
// unknown conversation stays 404.
func TestGetConversationSession_NoSessionIsNoContent(t *testing.T) {
	store := &mockStore{}
	store.convs = append(store.convs, conversation.Conversation{ID: "conv-1", ProjectID: "proj-1"})
	r := newTestRouterWithStore(store)

	if w := getJSON(t, r, "/api/v1/conversations/conv-1/session"); w.Code != http.StatusNoContent {
		t.Fatalf("no session: %d %q, want 204", w.Code, w.Body.String())
	}
	if w := getJSON(t, r, "/api/v1/conversations/no-such-conv/session"); w.Code != http.StatusNotFound {
		t.Fatalf("unknown conversation: %d %s, want 404", w.Code, w.Body.String())
	}
}

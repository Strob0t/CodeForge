package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// TestSendMessage_WhileARunIsActiveReturns409: a conversation runs one run
// at a time; a message sent while its run is active is refused with 409 and
// not stored (review 2, finding 5). After a stop the next message runs.
func TestSendMessage_WhileARunIsActiveReturns409(t *testing.T) {
	for _, agentic := range []bool{true, false} {
		name := "simple"
		if agentic {
			name = "agentic"
		}
		t.Run(name, func(t *testing.T) {
			store := &mockStore{
				projects: []project.Project{{ID: "proj-1", Name: "p", WorkspacePath: t.TempDir()}},
				convs:    []conversation.Conversation{{ID: "conv-1", ProjectID: "proj-1"}},
			}
			r := newTestRouterWithModelAndStore(store, "openai/gpt-4o")
			send := func() *httptest.ResponseRecorder {
				body, _ := json.Marshal(conversation.SendMessageRequest{Content: "do it", Agentic: &agentic})
				req := httptest.NewRequest("POST", "/api/v1/conversations/conv-1/messages", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				return w
			}

			if w := send(); w.Code != http.StatusAccepted {
				t.Fatalf("first message: %d %s", w.Code, w.Body.String())
			}
			stored := len(store.messages)

			w := send()
			if w.Code != http.StatusConflict {
				t.Fatalf("second message: %d %s, want 409", w.Code, w.Body.String())
			}
			if msg := decodeErrorMessage(t, w); msg != "conversation run in progress" {
				t.Errorf("error = %q, want %q", msg, "conversation run in progress")
			}
			if len(store.messages) != stored {
				t.Errorf("messages = %d, want %d: the refused message is not stored", len(store.messages), stored)
			}

			stop := httptest.NewRequest("POST", "/api/v1/conversations/conv-1/stop", http.NoBody)
			sw := httptest.NewRecorder()
			r.ServeHTTP(sw, stop)
			if sw.Code != http.StatusOK {
				t.Fatalf("stop: %d %s", sw.Code, sw.Body.String())
			}
			if w := send(); w.Code != http.StatusAccepted {
				t.Fatalf("message after the stop: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

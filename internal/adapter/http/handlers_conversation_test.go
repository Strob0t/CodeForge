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

// --- Conversation Handler Tests ---

func TestHandleCreateConversation_Success(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	store.projects = append(store.projects, project.Project{
		ID:   "proj-1",
		Name: "test-project",
	})

	body, _ := json.Marshal(conversation.CreateRequest{Title: "Test Chat"})
	req := httptest.NewRequest("POST", "/api/v1/projects/proj-1/conversations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var conv conversation.Conversation
	if err := json.NewDecoder(w.Body).Decode(&conv); err != nil {
		t.Fatal(err)
	}
	if conv.Title != "Test Chat" {
		t.Fatalf("expected title 'Test Chat', got %q", conv.Title)
	}
	if conv.ProjectID != "proj-1" {
		t.Fatalf("expected project_id 'proj-1', got %q", conv.ProjectID)
	}
	if conv.ID == "" {
		t.Fatal("expected non-empty conversation ID")
	}
}

func TestHandleCreateConversation_DefaultTitle(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	store.projects = append(store.projects, project.Project{
		ID:   "proj-1",
		Name: "test-project",
	})

	// Empty title should default to "New Conversation".
	body, _ := json.Marshal(conversation.CreateRequest{})
	req := httptest.NewRequest("POST", "/api/v1/projects/proj-1/conversations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var conv conversation.Conversation
	if err := json.NewDecoder(w.Body).Decode(&conv); err != nil {
		t.Fatal(err)
	}
	if conv.Title != "New Conversation" {
		t.Fatalf("expected default title 'New Conversation', got %q", conv.Title)
	}
}

func TestHandleListConversations_Empty(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/projects/proj-1/conversations", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var convs []conversation.Conversation
	if err := json.NewDecoder(w.Body).Decode(&convs); err != nil {
		t.Fatal(err)
	}
	if len(convs) != 0 {
		t.Fatalf("expected empty list, got %d", len(convs))
	}
}

func TestHandleListConversations_WithData(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	store.convs = append(store.convs,
		conversation.Conversation{ID: "conv-1", ProjectID: "proj-1", Title: "Chat 1"},
		conversation.Conversation{ID: "conv-2", ProjectID: "proj-1", Title: "Chat 2"},
		conversation.Conversation{ID: "conv-3", ProjectID: "proj-other", Title: "Other"},
	)

	req := httptest.NewRequest("GET", "/api/v1/projects/proj-1/conversations", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var convs []conversation.Conversation
	if err := json.NewDecoder(w.Body).Decode(&convs); err != nil {
		t.Fatal(err)
	}
	if len(convs) != 2 {
		t.Fatalf("expected 2 conversations for proj-1, got %d", len(convs))
	}
}

func TestHandleGetConversation_Success(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	store.convs = append(store.convs, conversation.Conversation{
		ID:        "conv-1",
		ProjectID: "proj-1",
		Title:     "My Chat",
	})

	req := httptest.NewRequest("GET", "/api/v1/conversations/conv-1", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var conv conversation.Conversation
	if err := json.NewDecoder(w.Body).Decode(&conv); err != nil {
		t.Fatal(err)
	}
	if conv.Title != "My Chat" {
		t.Fatalf("expected title 'My Chat', got %q", conv.Title)
	}
}

func TestHandleGetConversation_NotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/conversations/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeleteConversation_Success(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	store.convs = append(store.convs, conversation.Conversation{
		ID:        "conv-1",
		ProjectID: "proj-1",
		Title:     "To Delete",
	})

	req := httptest.NewRequest("DELETE", "/api/v1/conversations/conv-1", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify it's gone.
	req = httptest.NewRequest("GET", "/api/v1/conversations/conv-1", http.NoBody)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", w.Code)
	}
}

func TestHandleDeleteConversation_NotFound(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("DELETE", "/api/v1/conversations/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListConversationMessages_Empty(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	store.convs = append(store.convs, conversation.Conversation{
		ID:        "conv-1",
		ProjectID: "proj-1",
	})

	req := httptest.NewRequest("GET", "/api/v1/conversations/conv-1/messages", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var msgs []conversation.Message
	if err := json.NewDecoder(w.Body).Decode(&msgs); err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected empty list, got %d", len(msgs))
	}
}

func TestHandleListConversationMessages_WithData(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	store.convs = append(store.convs, conversation.Conversation{
		ID:        "conv-1",
		ProjectID: "proj-1",
	})
	store.messages = append(store.messages,
		conversation.Message{ID: "msg-1", ConversationID: "conv-1", Role: "user", Content: "Hello"},
		conversation.Message{ID: "msg-2", ConversationID: "conv-1", Role: "assistant", Content: "Hi there"},
		conversation.Message{ID: "msg-3", ConversationID: "conv-other", Role: "user", Content: "Other"},
	)

	req := httptest.NewRequest("GET", "/api/v1/conversations/conv-1/messages", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var msgs []conversation.Message
	if err := json.NewDecoder(w.Body).Decode(&msgs); err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages for conv-1, got %d", len(msgs))
	}
}

func TestHandleSendMessage_EmptyContent(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	store.convs = append(store.convs, conversation.Conversation{
		ID:        "conv-1",
		ProjectID: "proj-1",
	})

	body, _ := json.Marshal(conversation.SendMessageRequest{Content: ""})
	req := httptest.NewRequest("POST", "/api/v1/conversations/conv-1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Empty content should fail — either 400 from validation or 500 from service.
	if w.Code == http.StatusCreated || w.Code == http.StatusAccepted {
		t.Fatalf("expected error status for empty content, got %d", w.Code)
	}
}

func TestHandleStopConversation(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	store.convs = append(store.convs, conversation.Conversation{
		ID:        "conv-1",
		ProjectID: "proj-1",
	})

	req := httptest.NewRequest("POST", "/api/v1/conversations/conv-1/stop", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// StopConversation requires a NATS queue; mock queue is a no-op publisher,
	// so this should either succeed (200) or fail gracefully.
	// The mock queue returns nil from Publish, so stop should succeed.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "cancelled" {
		t.Fatalf("expected status 'cancelled', got %q", result["status"])
	}
	if result["conversation_id"] != "conv-1" {
		t.Fatalf("expected conversation_id 'conv-1', got %q", result["conversation_id"])
	}
}

func TestHandleApproveToolCall_InvalidDecision(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(map[string]string{"decision": "maybe"})
	req := httptest.NewRequest("POST", "/api/v1/runs/run-1/approve/call-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleApproveToolCall_NoPendingApproval(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(map[string]string{"decision": "allow"})
	req := httptest.NewRequest("POST", "/api/v1/runs/run-1/approve/call-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

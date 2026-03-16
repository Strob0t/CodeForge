package conversation

import (
	"encoding/json"
	"testing"
	"time"
)

func TestConversationFields(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	c := Conversation{
		ID:        "conv-1",
		TenantID:  "tenant-1",
		ProjectID: "proj-1",
		Title:     "Test Conversation",
		CreatedAt: now,
		UpdatedAt: now,
	}

	if c.ID != "conv-1" {
		t.Errorf("ID = %q, want %q", c.ID, "conv-1")
	}
	if c.TenantID != "tenant-1" {
		t.Errorf("TenantID = %q, want %q", c.TenantID, "tenant-1")
	}
	if c.ProjectID != "proj-1" {
		t.Errorf("ProjectID = %q, want %q", c.ProjectID, "proj-1")
	}
	if c.Title != "Test Conversation" {
		t.Errorf("Title = %q, want %q", c.Title, "Test Conversation")
	}
	if !c.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", c.CreatedAt, now)
	}
	if !c.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt = %v, want %v", c.UpdatedAt, now)
	}
}

func TestConversationJSONRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
	orig := Conversation{
		ID:        "conv-abc",
		TenantID:  "t-1",
		ProjectID: "p-1",
		Title:     "My Chat",
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Conversation
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.ID != orig.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, orig.ID)
	}
	if decoded.TenantID != orig.TenantID {
		t.Errorf("TenantID = %q, want %q", decoded.TenantID, orig.TenantID)
	}
	if decoded.ProjectID != orig.ProjectID {
		t.Errorf("ProjectID = %q, want %q", decoded.ProjectID, orig.ProjectID)
	}
	if decoded.Title != orig.Title {
		t.Errorf("Title = %q, want %q", decoded.Title, orig.Title)
	}
}

func TestMessageFields(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	m := Message{
		ID:             "msg-1",
		ConversationID: "conv-1",
		Role:           "user",
		Content:        "Hello, agent!",
		TokensIn:       100,
		TokensOut:      200,
		Model:          "gpt-4",
		CreatedAt:      now,
	}

	if m.Role != "user" {
		t.Errorf("Role = %q, want %q", m.Role, "user")
	}
	if m.Content != "Hello, agent!" {
		t.Errorf("Content = %q, want %q", m.Content, "Hello, agent!")
	}
	if m.TokensIn != 100 {
		t.Errorf("TokensIn = %d, want 100", m.TokensIn)
	}
	if m.TokensOut != 200 {
		t.Errorf("TokensOut = %d, want 200", m.TokensOut)
	}
}

func TestMessageWithToolCalls(t *testing.T) {
	t.Parallel()

	toolCalls := json.RawMessage(`[{"id":"tc-1","name":"read_file"}]`)
	m := Message{
		ID:             "msg-2",
		ConversationID: "conv-1",
		Role:           "assistant",
		Content:        "",
		ToolCalls:      toolCalls,
		CreatedAt:      time.Now().UTC(),
	}

	if m.ToolCalls == nil {
		t.Fatal("ToolCalls should not be nil")
	}
	if string(m.ToolCalls) != `[{"id":"tc-1","name":"read_file"}]` {
		t.Errorf("ToolCalls = %s, want the original JSON", string(m.ToolCalls))
	}
}

func TestMessageWithToolResponse(t *testing.T) {
	t.Parallel()

	m := Message{
		ID:             "msg-3",
		ConversationID: "conv-1",
		Role:           "tool",
		Content:        "file contents here",
		ToolCallID:     "tc-1",
		ToolName:       "read_file",
		CreatedAt:      time.Now().UTC(),
	}

	if m.ToolCallID != "tc-1" {
		t.Errorf("ToolCallID = %q, want %q", m.ToolCallID, "tc-1")
	}
	if m.ToolName != "read_file" {
		t.Errorf("ToolName = %q, want %q", m.ToolName, "read_file")
	}
}

func TestMessageJSONRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
	orig := Message{
		ID:             "msg-rt",
		ConversationID: "conv-rt",
		Role:           "assistant",
		Content:        "Here is my response",
		TokensIn:       50,
		TokensOut:      150,
		Model:          "claude-3",
		CreatedAt:      now,
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.ID != orig.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, orig.ID)
	}
	if decoded.Role != orig.Role {
		t.Errorf("Role = %q, want %q", decoded.Role, orig.Role)
	}
	if decoded.Model != orig.Model {
		t.Errorf("Model = %q, want %q", decoded.Model, orig.Model)
	}
}

func TestCreateRequestFields(t *testing.T) {
	t.Parallel()

	cr := CreateRequest{
		ProjectID: "proj-1",
		Title:     "New Chat",
	}

	if cr.ProjectID != "proj-1" {
		t.Errorf("ProjectID = %q, want %q", cr.ProjectID, "proj-1")
	}
	if cr.Title != "New Chat" {
		t.Errorf("Title = %q, want %q", cr.Title, "New Chat")
	}
}

func TestSendMessageRequestFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		req     SendMessageRequest
		wantAg  bool
		hasMode bool
	}{
		{
			name:    "basic message",
			req:     SendMessageRequest{Content: "Hello"},
			wantAg:  false,
			hasMode: false,
		},
		{
			name: "agentic with mode",
			req: SendMessageRequest{
				Content: "Fix the bug",
				Agentic: boolPtr(true),
				Mode:    "coder",
			},
			wantAg:  true,
			hasMode: true,
		},
		{
			name: "non-agentic explicit",
			req: SendMessageRequest{
				Content: "Just chat",
				Agentic: boolPtr(false),
			},
			wantAg:  false,
			hasMode: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.req.Content == "" {
				t.Error("Content should not be empty")
			}
			if tt.req.Agentic != nil && *tt.req.Agentic != tt.wantAg {
				t.Errorf("Agentic = %v, want %v", *tt.req.Agentic, tt.wantAg)
			}
			if tt.hasMode && tt.req.Mode == "" {
				t.Error("Mode should not be empty when hasMode is true")
			}
		})
	}
}

func TestSendMessageRequestJSONOmitsUserID(t *testing.T) {
	t.Parallel()

	req := SendMessageRequest{
		Content: "test",
		UserID:  "user-123",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// UserID has `json:"-"`, so it must not appear in JSON.
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if _, found := m["user_id"]; found {
		t.Error("UserID should not be serialized to JSON (tag is \"-\")")
	}
}

func TestMessageImageRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		img  MessageImage
	}{
		{
			name: "full fields",
			img: MessageImage{
				Data:      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
				MediaType: "image/png",
				AltText:   "A red pixel",
			},
		},
		{
			name: "empty alt_text omitted",
			img: MessageImage{
				Data:      "AAAA",
				MediaType: "image/jpeg",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tt.img)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			var decoded MessageImage
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			if decoded.Data != tt.img.Data {
				t.Errorf("Data = %q, want %q", decoded.Data, tt.img.Data)
			}
			if decoded.MediaType != tt.img.MediaType {
				t.Errorf("MediaType = %q, want %q", decoded.MediaType, tt.img.MediaType)
			}
			if decoded.AltText != tt.img.AltText {
				t.Errorf("AltText = %q, want %q", decoded.AltText, tt.img.AltText)
			}

			// Empty alt_text should be omitted.
			if tt.img.AltText == "" {
				var m map[string]any
				if err := json.Unmarshal(data, &m); err != nil {
					t.Fatalf("Unmarshal to map: %v", err)
				}
				if _, found := m["alt_text"]; found {
					t.Error("empty alt_text should be omitted from JSON (omitempty)")
				}
			}
		})
	}
}

func TestMessageImagesOmittedWhenEmpty(t *testing.T) {
	t.Parallel()

	// Message with nil Images should omit the images key.
	m := Message{
		ID:             "msg-no-img",
		ConversationID: "conv-1",
		Role:           "user",
		Content:        "Just text",
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map: %v", err)
	}

	if _, found := raw["images"]; found {
		t.Error("nil Images should be omitted from JSON (omitempty)")
	}
}

func TestSendMessageRequestWithImages(t *testing.T) {
	t.Parallel()

	req := SendMessageRequest{
		Content: "Analyze this design",
		Images: []MessageImage{
			{Data: "base64data", MediaType: "image/png", AltText: "wireframe"},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded SendMessageRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(decoded.Images) != 1 {
		t.Fatalf("Images len = %d, want 1", len(decoded.Images))
	}
	if decoded.Images[0].MediaType != "image/png" {
		t.Errorf("Images[0].MediaType = %q, want %q", decoded.Images[0].MediaType, "image/png")
	}
}

func TestSendMessageRequestImagesOmittedWhenNil(t *testing.T) {
	t.Parallel()

	req := SendMessageRequest{
		Content: "no images",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map: %v", err)
	}

	if _, found := raw["images"]; found {
		t.Error("nil Images should be omitted from JSON (omitempty)")
	}
}

func boolPtr(b bool) *bool { return &b }

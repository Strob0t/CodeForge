package litellm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
)

func TestListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/model/info" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		resp := map[string][]litellm.Model{
			"data": {
				{ModelName: "gpt-4o", Provider: "openai"},
				{ModelName: "claude-sonnet-4-20250514", Provider: "anthropic"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")
	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ModelName != "gpt-4o" {
		t.Fatalf("expected gpt-4o, got %q", models[0].ModelName)
	}
}

func TestAddModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/model/new" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			t.Fatalf("unexpected auth: %q", auth)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"Model added successfully"}`))
	}))
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")
	err := client.AddModel(context.Background(), litellm.AddModelRequest{
		ModelName:     "test-model",
		LiteLLMParams: map[string]string{"model": "openai/gpt-4o"},
	})
	if err != nil {
		t.Fatalf("AddModel failed: %v", err)
	}
}

func TestDeleteModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/model/delete" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")
	err := client.DeleteModel(context.Background(), "model-123")
	if err != nil {
		t.Fatalf("DeleteModel failed: %v", err)
	}
}

func TestHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy"}`))
	}))
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")
	healthy, err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("Health failed: %v", err)
	}
	if !healthy {
		t.Fatal("expected healthy")
	}
}

func TestHealthUnhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"unhealthy"}`))
	}))
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")
	healthy, _ := client.Health(context.Background())
	if healthy {
		t.Fatal("expected unhealthy")
	}
}

// --- ChatCompletion tests ---

func TestChatCompletionSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"content": "Hello world"}}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5},
			"model": "gpt-4o-mini"
		}`))
	}))
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")
	resp, err := client.ChatCompletion(context.Background(), litellm.ChatCompletionRequest{
		Model:    "gpt-4o-mini",
		Messages: []litellm.ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", resp.Content)
	}
	if resp.TokensIn != 10 {
		t.Errorf("expected 10 tokens_in, got %d", resp.TokensIn)
	}
	if resp.TokensOut != 5 {
		t.Errorf("expected 5 tokens_out, got %d", resp.TokensOut)
	}
	if resp.Model != "gpt-4o-mini" {
		t.Errorf("expected model 'gpt-4o-mini', got %q", resp.Model)
	}
}

func TestChatCompletionEmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices": [], "usage": {}, "model": "test"}`))
	}))
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "")
	resp, err := client.ChatCompletion(context.Background(), litellm.ChatCompletionRequest{
		Model:    "test",
		Messages: []litellm.ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "" {
		t.Errorf("expected empty content, got %q", resp.Content)
	}
}

func TestChatCompletionHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "bad request"}`))
	}))
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "")
	_, err := client.ChatCompletion(context.Background(), litellm.ChatCompletionRequest{
		Model:    "test",
		Messages: []litellm.ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestChatCompletionAuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices": [{"message": {"content": "ok"}}], "usage": {}, "model": "m"}`))
	}))
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "sk-secret")
	_, err := client.ChatCompletion(context.Background(), litellm.ChatCompletionRequest{
		Model:    "m",
		Messages: []litellm.ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer sk-secret" {
		t.Errorf("expected 'Bearer sk-secret', got %q", gotAuth)
	}
}

// --- Tool-calling tests ---

func TestChatCompletionWithToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify tools are sent in the request body.
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		var reqBody map[string]any
		if err := json.Unmarshal(body, &reqBody); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}
		tools, ok := reqBody["tools"]
		if !ok {
			t.Fatal("expected tools in request body")
		}
		toolsSlice, ok := tools.([]any)
		if !ok || len(toolsSlice) != 1 {
			t.Fatalf("expected 1 tool, got %v", tools)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {
					"content": "",
					"tool_calls": [{
						"id": "call_abc123",
						"type": "function",
						"function": {
							"name": "get_weather",
							"arguments": "{\"location\":\"London\"}"
						}
					}]
				},
				"finish_reason": "tool_calls"
			}],
			"usage": {"prompt_tokens": 20, "completion_tokens": 10},
			"model": "gpt-4o"
		}`))
	}))
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")
	resp, err := client.ChatCompletion(context.Background(), litellm.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []litellm.ChatMessage{{Role: "user", Content: "What is the weather in London?"}},
		Tools: []litellm.ToolDefinition{{
			Type: "function",
			Function: litellm.ToolFunction{
				Name:        "get_weather",
				Description: "Get the current weather",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{"type": "string"},
					},
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.FinishReason != "tool_calls" {
		t.Errorf("expected finish_reason 'tool_calls', got %q", resp.FinishReason)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "call_abc123" {
		t.Errorf("expected tool call ID 'call_abc123', got %q", tc.ID)
	}
	if tc.Type != "function" {
		t.Errorf("expected type 'function', got %q", tc.Type)
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("expected function name 'get_weather', got %q", tc.Function.Name)
	}
	if tc.Function.Arguments != `{"location":"London"}` {
		t.Errorf("unexpected arguments: %q", tc.Function.Arguments)
	}
	if resp.Content != "" {
		t.Errorf("expected empty content for tool call response, got %q", resp.Content)
	}
	if resp.TokensIn != 20 {
		t.Errorf("expected 20 tokens_in, got %d", resp.TokensIn)
	}
}

func TestStreamingToolCallAssembly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.Flusher")
		}

		// Chunk 1: first tool call begins with ID, type, and function name.
		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_001","type":"function","function":{"name":"read_file","arguments":""}}]},"finish_reason":null}],"model":"gpt-4o"}`)
		flusher.Flush()

		// Chunk 2: first argument fragment.
		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":"}}]},"finish_reason":null}],"model":"gpt-4o"}`)
		flusher.Flush()

		// Chunk 3: second argument fragment.
		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"main.go\"}"}}]},"finish_reason":null}],"model":"gpt-4o"}`)
		flusher.Flush()

		// Chunk 4: second tool call begins.
		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_002","type":"function","function":{"name":"list_dir","arguments":"{\"path\":\".\"}"}}]},"finish_reason":null}],"model":"gpt-4o"}`)
		flusher.Flush()

		// Chunk 5: finish reason.
		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"delta":{},"finish_reason":"tool_calls"}],"model":"gpt-4o","usage":{"prompt_tokens":15,"completion_tokens":25}}`)
		flusher.Flush()

		// Chunk 6: done.
		_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")

	var chunks []litellm.StreamChunk
	resp, err := client.ChatCompletionStream(context.Background(), litellm.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []litellm.ChatMessage{{Role: "user", Content: "read main.go and list files"}},
		Tools: []litellm.ToolDefinition{{
			Type: "function",
			Function: litellm.ToolFunction{
				Name:        "read_file",
				Description: "Read a file",
				Parameters:  map[string]any{"type": "object"},
			},
		}},
	}, func(chunk litellm.StreamChunk) {
		chunks = append(chunks, chunk)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the final accumulated response.
	if len(resp.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(resp.ToolCalls))
	}

	tc0 := resp.ToolCalls[0]
	if tc0.ID != "call_001" {
		t.Errorf("expected tool call 0 ID 'call_001', got %q", tc0.ID)
	}
	if tc0.Function.Name != "read_file" {
		t.Errorf("expected function name 'read_file', got %q", tc0.Function.Name)
	}
	if tc0.Function.Arguments != `{"path":"main.go"}` {
		t.Errorf("expected assembled arguments, got %q", tc0.Function.Arguments)
	}

	tc1 := resp.ToolCalls[1]
	if tc1.ID != "call_002" {
		t.Errorf("expected tool call 1 ID 'call_002', got %q", tc1.ID)
	}
	if tc1.Function.Name != "list_dir" {
		t.Errorf("expected function name 'list_dir', got %q", tc1.Function.Name)
	}
	if tc1.Function.Arguments != `{"path":"."}` {
		t.Errorf("expected arguments '{\"path\":\".\"}', got %q", tc1.Function.Arguments)
	}

	if resp.FinishReason != "tool_calls" {
		t.Errorf("expected finish_reason 'tool_calls', got %q", resp.FinishReason)
	}
	if resp.TokensIn != 15 {
		t.Errorf("expected 15 tokens_in, got %d", resp.TokensIn)
	}
	if resp.TokensOut != 25 {
		t.Errorf("expected 25 tokens_out, got %d", resp.TokensOut)
	}

	// The last chunk should be the Done chunk with accumulated tool calls.
	lastChunk := chunks[len(chunks)-1]
	if !lastChunk.Done {
		t.Error("expected last chunk to be Done")
	}
	if len(lastChunk.ToolCalls) != 2 {
		t.Errorf("expected 2 tool calls in Done chunk, got %d", len(lastChunk.ToolCalls))
	}
}

func TestChatCompletionNoTools(t *testing.T) {
	// Verify backward compatibility: no tools in request means no tool_calls fields
	// in the serialized JSON, and the response parses normally.
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"content": "Hello"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 5, "completion_tokens": 3},
			"model": "gpt-4o-mini"
		}`))
	}))
	defer srv.Close()

	client := litellm.NewClient(srv.URL, "test-key")
	resp, err := client.ChatCompletion(context.Background(), litellm.ChatCompletionRequest{
		Model:    "gpt-4o-mini",
		Messages: []litellm.ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Tools/ToolChoice should be omitted from the request body (omitempty).
	if _, ok := capturedBody["tools"]; ok {
		t.Error("expected tools to be omitted from request body when not provided")
	}
	if _, ok := capturedBody["tool_choice"]; ok {
		t.Error("expected tool_choice to be omitted from request body when not provided")
	}

	if resp.Content != "Hello" {
		t.Errorf("expected 'Hello', got %q", resp.Content)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got %q", resp.FinishReason)
	}
	if len(resp.ToolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(resp.ToolCalls))
	}
	if resp.TokensIn != 5 {
		t.Errorf("expected 5 tokens_in, got %d", resp.TokensIn)
	}
	if resp.TokensOut != 3 {
		t.Errorf("expected 3 tokens_out, got %d", resp.TokensOut)
	}
}

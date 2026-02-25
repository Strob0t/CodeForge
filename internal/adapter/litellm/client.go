// Package litellm provides an HTTP client for the LiteLLM Proxy API,
// including admin operations and chat completions.
package litellm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Strob0t/CodeForge/internal/resilience"
	"github.com/Strob0t/CodeForge/internal/secrets"
)

// Model represents a configured model in LiteLLM.
type Model struct {
	ModelName string         `json:"model_name"`
	Provider  string         `json:"litellm_provider,omitempty"`
	ModelID   string         `json:"model_id,omitempty"`
	ModelInfo map[string]any `json:"model_info,omitempty"`
	Params    map[string]any `json:"litellm_params,omitempty"`
}

// HealthStatus represents the health of a LiteLLM model.
type HealthStatus struct {
	Healthy   []ModelHealth `json:"healthy_endpoints"`
	Unhealthy []ModelHealth `json:"unhealthy_endpoints"`
}

// ModelHealth represents the health of a single model endpoint.
type ModelHealth struct {
	Model    string `json:"model"`
	Provider string `json:"api_base,omitempty"`
}

// Client talks to the LiteLLM Proxy admin API.
type Client struct {
	baseURL    string
	masterKey  string
	vault      *secrets.Vault
	httpClient *http.Client
	breaker    *resilience.Breaker
}

// NewClient creates a new LiteLLM admin client.
func NewClient(baseURL, masterKey string) *Client {
	return &Client{
		baseURL:   baseURL,
		masterKey: masterKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SetBreaker attaches a circuit breaker to all outgoing HTTP calls.
func (c *Client) SetBreaker(b *resilience.Breaker) {
	c.breaker = b
}

// SetVault attaches a secrets vault. When set, the master key is read from
// the vault on each request, enabling hot reload via SIGHUP.
func (c *Client) SetVault(v *secrets.Vault) {
	c.vault = v
}

// activeMasterKey returns the master key from the vault (if set and non-empty),
// falling back to the static masterKey field.
func (c *Client) activeMasterKey() string {
	if c.vault != nil {
		if k := c.vault.Get("LITELLM_MASTER_KEY"); k != "" {
			return k
		}
	}
	return c.masterKey
}

// ListModels returns all configured models from LiteLLM.
func (c *Client) ListModels(ctx context.Context) ([]Model, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/model/info", nil)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}

	var result struct {
		Data []Model `json:"data"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("unmarshal models: %w", err)
	}
	return result.Data, nil
}

// AddModel adds a new model configuration to LiteLLM.
func (c *Client) AddModel(ctx context.Context, req AddModelRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal add model: %w", err)
	}

	if _, err := c.doRequest(ctx, http.MethodPost, "/model/new", body); err != nil {
		return fmt.Errorf("add model: %w", err)
	}
	return nil
}

// DeleteModel removes a model configuration from LiteLLM.
func (c *Client) DeleteModel(ctx context.Context, modelID string) error {
	body, err := json.Marshal(map[string]string{"id": modelID})
	if err != nil {
		return fmt.Errorf("marshal delete model: %w", err)
	}

	if _, err := c.doRequest(ctx, http.MethodPost, "/model/delete", body); err != nil {
		return fmt.Errorf("delete model: %w", err)
	}
	return nil
}

// Health checks if LiteLLM is healthy.
func (c *Client) Health(ctx context.Context) (bool, error) {
	_, err := c.doRequest(ctx, http.MethodGet, "/health", nil)
	return err == nil, err
}

// AddModelRequest is the request body for adding a model to LiteLLM.
type AddModelRequest struct {
	ModelName     string            `json:"model_name"`
	LiteLLMParams map[string]string `json:"litellm_params"`
	ModelInfo     map[string]any    `json:"model_info,omitempty"`
}

// DiscoveredModel represents a model found via auto-discovery with health status.
type DiscoveredModel struct {
	ModelName     string         `json:"model_name"`
	ModelID       string         `json:"model_id,omitempty"`
	Provider      string         `json:"provider,omitempty"`
	Tags          []string       `json:"tags,omitempty"`
	MaxTokens     int            `json:"max_tokens,omitempty"`
	InputCostPer  float64        `json:"input_cost_per_token,omitempty"`
	OutputCostPer float64        `json:"output_cost_per_token,omitempty"`
	Status        string         `json:"status"` // "reachable" or "unreachable"
	Source        string         `json:"source"` // "litellm" or "ollama"
	ModelInfo     map[string]any `json:"model_info,omitempty"`
}

// DiscoverModels queries LiteLLM /model/info and /v1/models to discover all
// available models with their health status and metadata.
func (c *Client) DiscoverModels(ctx context.Context) ([]DiscoveredModel, error) {
	// Fetch model info from LiteLLM admin API.
	infoResp, err := c.doRequest(ctx, http.MethodGet, "/model/info", nil)
	if err != nil {
		return nil, fmt.Errorf("discover models (model/info): %w", err)
	}

	var infoResult struct {
		Data []struct {
			ModelName string         `json:"model_name"`
			ModelID   string         `json:"model_id"`
			ModelInfo map[string]any `json:"model_info"`
			Params    map[string]any `json:"litellm_params"`
		} `json:"data"`
	}
	if err := json.Unmarshal(infoResp, &infoResult); err != nil {
		return nil, fmt.Errorf("unmarshal model info: %w", err)
	}

	// Also fetch the OpenAI-compatible /v1/models list for ID cross-reference.
	modelsResp, err := c.doRequest(ctx, http.MethodGet, "/v1/models", nil)
	if err != nil {
		// Non-fatal: we can still return info results.
		modelsResp = nil
	}

	reachableSet := make(map[string]bool)
	if modelsResp != nil {
		var modelsList struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(modelsResp, &modelsList); err == nil {
			for _, m := range modelsList.Data {
				reachableSet[m.ID] = true
			}
		}
	}

	discovered := make([]DiscoveredModel, 0, len(infoResult.Data))
	for _, m := range infoResult.Data {
		dm := DiscoveredModel{
			ModelName: m.ModelName,
			ModelID:   m.ModelID,
			Source:    "litellm",
			ModelInfo: m.ModelInfo,
		}

		// Extract provider from litellm_params.
		if m.Params != nil {
			if model, ok := m.Params["model"].(string); ok {
				if provider, _, found := strings.Cut(model, "/"); found {
					dm.Provider = provider
				}
			}
		}

		// Extract known fields from model_info.
		if m.ModelInfo != nil {
			if maxTok, ok := m.ModelInfo["max_tokens"].(float64); ok {
				dm.MaxTokens = int(maxTok)
			}
			if inCost, ok := m.ModelInfo["input_cost_per_token"].(float64); ok {
				dm.InputCostPer = inCost
			}
			if outCost, ok := m.ModelInfo["output_cost_per_token"].(float64); ok {
				dm.OutputCostPer = outCost
			}
			if tags, ok := m.ModelInfo["tags"].([]any); ok {
				for _, t := range tags {
					if s, ok := t.(string); ok {
						dm.Tags = append(dm.Tags, s)
					}
				}
			}
		}

		// Models in /model/info are configured in LiteLLM, so mark reachable.
		dm.Status = "reachable"

		discovered = append(discovered, dm)
	}

	return discovered, nil
}

// DiscoverOllamaModels queries a local Ollama instance for available models.
// If ollamaBaseURL is empty, it returns nil (no Ollama configured).
func (c *Client) DiscoverOllamaModels(ctx context.Context, ollamaBaseURL string) ([]DiscoveredModel, error) {
	if ollamaBaseURL == "" {
		return nil, nil
	}

	// Ollama API: GET /api/tags returns available local models.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ollamaBaseURL+"/api/tags", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create ollama request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ollama API error %d", resp.StatusCode)
	}

	var result struct {
		Models []struct {
			Name       string `json:"name"`
			ModifiedAt string `json:"modified_at"`
			Size       int64  `json:"size"`
			Details    struct {
				ParameterSize string `json:"parameter_size"`
				Family        string `json:"family"`
			} `json:"details"`
		} `json:"models"`
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read ollama response: %w", err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal ollama models: %w", err)
	}

	discovered := make([]DiscoveredModel, 0, len(result.Models))
	for _, m := range result.Models {
		dm := DiscoveredModel{
			ModelName: m.Name,
			ModelID:   "ollama/" + m.Name,
			Provider:  "ollama",
			Status:    "reachable",
			Source:    "ollama",
			ModelInfo: map[string]any{
				"parameter_size": m.Details.ParameterSize,
				"family":         m.Details.Family,
				"size_bytes":     m.Size,
			},
		}
		discovered = append(discovered, dm)
	}

	return discovered, nil
}

// SelectStrongestModel picks the best model from a list of discovered models.
// Ranking heuristic (in order of priority):
//  1. Output cost per token — paid models correlate strongly with quality.
//  2. Model name pattern matching — known model families have known strengths.
//  3. MaxTokens — larger context windows as tiebreaker.
//
// Returns an empty string if the list is empty.
func SelectStrongestModel(models []DiscoveredModel) string {
	if len(models) == 0 {
		return ""
	}

	bestIdx := 0
	bestScore := modelScore(&models[0])
	for i := 1; i < len(models); i++ {
		if s := modelScore(&models[i]); s > bestScore {
			bestIdx = i
			bestScore = s
		}
	}
	return models[bestIdx].ModelName
}

// modelScore assigns a quality score to a discovered model.
func modelScore(m *DiscoveredModel) float64 {
	// Paid models: output cost is a strong quality signal.
	// More expensive models are almost always more capable.
	if m.OutputCostPer > 0 {
		// Scale so typical prices produce scores in 50-100 range.
		// GPT-4o: ~$15/1M = 1.5e-5 → score ~75
		// Claude Opus: ~$75/1M = 7.5e-5 → score ~100+
		// GPT-4o-mini: ~$0.6/1M = 6e-7 → score ~30
		return m.OutputCostPer * 5e6
	}

	// Free/local models: use name-based heuristics.
	// Check specific patterns first (longer matches) to avoid false hits.
	name := strings.ToLower(m.ModelName)
	patterns := []struct {
		substr string
		score  float64
	}{
		// Frontier models (if free tier / Groq-hosted)
		{"llama-4-maverick", 80},
		{"llama-4-scout", 75},
		{"llama-3.3-70b", 70},
		{"llama-3.1-70b", 60},
		{"mixtral-8x22b", 58},
		{"mixtral-8x7b", 45},
		{"llama-3.1-8b", 30},
		{"llama-3-8b", 28},
		{"gemma-2-27b", 50},
		{"gemma-2-9b", 35},
		{"phi-3", 32},
		{"qwen-2.5-72b", 65},
		{"qwen-2.5-32b", 55},
		{"deepseek-v3", 72},
		{"deepseek-r1", 74},
		{"deepseek-coder", 50},
		// Known premium model families
		{"claude-3-opus", 95},
		{"claude-3.5-sonnet", 93},
		{"claude-3-sonnet", 85},
		{"claude-3-haiku", 65},
		{"gpt-4o-mini", 63},
		{"gpt-4o", 90},
		{"gpt-4-turbo", 88},
		{"gpt-4", 82},
		{"gpt-3.5", 25},
		{"o1-preview", 92},
		{"o1-mini", 70},
		{"o3-mini", 78},
		{"gemini-1.5-pro", 88},
		{"gemini-2.0-flash", 76},
		{"gemini-1.5-flash", 60},
	}

	for _, p := range patterns {
		if strings.Contains(name, p.substr) {
			return p.score
		}
	}

	// Unknown model: use MaxTokens as rough proxy (larger context ≈ newer model).
	if m.MaxTokens > 0 {
		return float64(m.MaxTokens) / 1e5 // 128k → 1.28, not great but non-zero
	}

	return 1.0
}

// --- Chat Completion (OpenAI-compatible) ---

// ToolFunction describes a function that can be called by the model.
type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToolDefinition defines a tool available to the model.
type ToolDefinition struct {
	Type     string       `json:"type"` // Always "function".
	Function ToolFunction `json:"function"`
}

// ToolCallFunction holds the function name and serialized arguments of a tool call.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolCall represents a tool invocation requested by the model.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ChatMessage represents a single message in a chat completion.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ChatCompletionRequest is the request body for /v1/chat/completions.
type ChatCompletionRequest struct {
	Model       string           `json:"model"`
	Messages    []ChatMessage    `json:"messages"`
	Temperature float64          `json:"temperature,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	ToolChoice  any              `json:"tool_choice,omitempty"`
}

// ChatCompletionResponse is the parsed response from a completion call.
type ChatCompletionResponse struct {
	Content      string
	TokensIn     int
	TokensOut    int
	Model        string
	ToolCalls    []ToolCall
	FinishReason string
}

// ChatCompletion sends a chat completion request to the LiteLLM Proxy's
// OpenAI-compatible /v1/chat/completions endpoint.
func (c *Client) ChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) { //nolint:gocritic // hugeParam acceptable for request struct
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal completion request: %w", err)
	}

	data, err := c.doRequest(ctx, http.MethodPost, "/v1/chat/completions", body)
	if err != nil {
		return nil, fmt.Errorf("chat completion: %w", err)
	}

	var raw struct {
		Choices []struct {
			Message struct {
				Content   string     `json:"content"`
				ToolCalls []ToolCall `json:"tool_calls,omitempty"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal completion response: %w", err)
	}

	resp := &ChatCompletionResponse{
		TokensIn:  raw.Usage.PromptTokens,
		TokensOut: raw.Usage.CompletionTokens,
		Model:     raw.Model,
	}
	if len(raw.Choices) > 0 {
		resp.Content = raw.Choices[0].Message.Content
		resp.ToolCalls = raw.Choices[0].Message.ToolCalls
		resp.FinishReason = raw.Choices[0].FinishReason
	}

	return resp, nil
}

// StreamChunk represents a single chunk from a streaming completion response.
type StreamChunk struct {
	Content      string     // The text content of this chunk (may be empty for non-content chunks).
	Done         bool       // True when the stream is complete (final chunk or [DONE]).
	Model        string     // Model name from the response.
	TokensIn     int        // Prompt tokens (only set on the final chunk with usage data).
	TokensOut    int        // Completion tokens (only set on the final chunk with usage data).
	ToolCalls    []ToolCall // Accumulated tool calls (set on the final chunk when finish_reason is "tool_calls").
	FinishReason string     // The finish reason from the response (e.g. "stop", "tool_calls").
}

// ChatCompletionStream sends a streaming chat completion request. It calls
// onChunk for each SSE chunk received from the LiteLLM Proxy. The caller
// should accumulate content from chunks where Done is false.
func (c *Client) ChatCompletionStream(ctx context.Context, req ChatCompletionRequest, onChunk func(StreamChunk)) (*ChatCompletionResponse, error) { //nolint:gocritic // hugeParam acceptable for request struct
	// Force stream mode.
	type streamReq struct {
		ChatCompletionRequest
		Stream        bool `json:"stream"`
		StreamOptions *struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options,omitempty"`
	}
	sr := streamReq{
		ChatCompletionRequest: req,
		Stream:                true,
		StreamOptions: &struct {
			IncludeUsage bool `json:"include_usage"`
		}{IncludeUsage: true},
	}

	body, err := json.Marshal(sr)
	if err != nil {
		return nil, fmt.Errorf("marshal stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if key := c.activeMasterKey(); key != "" {
		httpReq.Header.Set("Authorization", "Bearer "+key)
	}

	// Use a client without the default timeout for streaming.
	streamClient := &http.Client{}
	resp, err := streamClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("stream request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("litellm stream API error %d: %s", resp.StatusCode, string(data))
	}

	// Parse SSE stream.
	var fullContent strings.Builder
	var model string
	var tokensIn, tokensOut int
	var finishReason string
	// Accumulate tool calls by index. Streaming deltas reference tool calls
	// by their index field; we grow this slice as needed and concatenate
	// argument fragments.
	var toolCalls []ToolCall

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// SSE format: "data: {json}" or "data: [DONE]"
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		if data == "[DONE]" {
			if onChunk != nil {
				onChunk(StreamChunk{
					Done:         true,
					Model:        model,
					TokensIn:     tokensIn,
					TokensOut:    tokensOut,
					ToolCalls:    toolCalls,
					FinishReason: finishReason,
				})
			}
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id,omitempty"`
						Type     string `json:"type,omitempty"`
						Function struct {
							Name      string `json:"name,omitempty"`
							Arguments string `json:"arguments,omitempty"`
						} `json:"function"`
					} `json:"tool_calls,omitempty"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Model string `json:"model"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // Skip malformed chunks.
		}

		if chunk.Model != "" {
			model = chunk.Model
		}
		if chunk.Usage != nil {
			tokensIn = chunk.Usage.PromptTokens
			tokensOut = chunk.Usage.CompletionTokens
		}

		content := ""
		if len(chunk.Choices) > 0 {
			choice := chunk.Choices[0]
			content = choice.Delta.Content

			if choice.FinishReason != nil {
				finishReason = *choice.FinishReason
			}

			// Assemble tool calls by index.
			for _, tc := range choice.Delta.ToolCalls {
				// Grow slice to accommodate the index.
				for len(toolCalls) <= tc.Index {
					toolCalls = append(toolCalls, ToolCall{})
				}
				if tc.ID != "" {
					toolCalls[tc.Index].ID = tc.ID
				}
				if tc.Type != "" {
					toolCalls[tc.Index].Type = tc.Type
				}
				if tc.Function.Name != "" {
					toolCalls[tc.Index].Function.Name = tc.Function.Name
				}
				toolCalls[tc.Index].Function.Arguments += tc.Function.Arguments
			}
		}
		if content != "" {
			fullContent.WriteString(content)
		}

		if onChunk != nil {
			onChunk(StreamChunk{
				Content: content,
				Model:   model,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read stream: %w", err)
	}

	return &ChatCompletionResponse{
		Content:      fullContent.String(),
		TokensIn:     tokensIn,
		TokensOut:    tokensOut,
		Model:        model,
		ToolCalls:    toolCalls,
		FinishReason: finishReason,
	}, nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	var result []byte
	call := func() error {
		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}

		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		if key := c.activeMasterKey(); key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("http request: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}

		if resp.StatusCode >= 400 {
			return fmt.Errorf("litellm API error %d: %s", resp.StatusCode, string(data))
		}

		result = data
		return nil
	}

	if c.breaker != nil {
		if err := c.breaker.Execute(call); err != nil {
			return nil, err
		}
		return result, nil
	}

	if err := call(); err != nil {
		return nil, err
	}
	return result, nil
}

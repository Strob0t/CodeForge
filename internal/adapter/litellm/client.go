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

	"github.com/Strob0t/CodeForge/internal/port/llm"
	"github.com/Strob0t/CodeForge/internal/resilience"
	"github.com/Strob0t/CodeForge/internal/secrets"
)

// Compile-time assertions: *Client satisfies all port/llm interfaces.
var (
	_ llm.Provider        = (*Client)(nil)
	_ llm.ModelDiscoverer = (*Client)(nil)
	_ llm.ModelAdmin      = (*Client)(nil)
)

// Type aliases — canonical definitions live in port/llm.
// All existing code using litellm.ChatMessage etc. continues to compile.
type (
	Model                  = llm.Model
	HealthStatus           = llm.HealthStatus
	ModelHealth            = llm.ModelHealth
	ToolFunction           = llm.ToolFunction
	ToolDefinition         = llm.ToolDefinition
	ToolCallFunction       = llm.ToolCallFunction
	ToolCall               = llm.ToolCall
	ChatMessage            = llm.ChatMessage
	ChatCompletionRequest  = llm.ChatCompletionRequest
	ChatCompletionResponse = llm.ChatCompletionResponse
	StreamChunk            = llm.StreamChunk
	HealthStatusReport     = llm.HealthStatusReport
	ModelEndpoint          = llm.ModelEndpoint
	DiscoveredModel        = llm.DiscoveredModel
	AddModelRequest        = llm.AddModelRequest
)

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
	// Infer vision capability from metadata or model name.
	for i := range result.Data {
		result.Data[i].SupportsVision = inferVisionSupport(
			result.Data[i].ModelName, result.Data[i].ModelInfo,
		)
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

// HealthDetailed fetches per-model health from LiteLLM /health and parses
// the healthy/unhealthy endpoint breakdown.
func (c *Client) HealthDetailed(ctx context.Context) (*HealthStatusReport, error) {
	body, err := c.doRequest(ctx, http.MethodGet, "/health", nil)
	if err != nil {
		return nil, fmt.Errorf("health detailed: %w", err)
	}

	var report HealthStatusReport
	if err := json.Unmarshal(body, &report); err != nil {
		return nil, fmt.Errorf("unmarshal health report: %w", err)
	}
	return &report, nil
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

		// Infer vision capability from metadata or model name.
		dm.SupportsVision = inferVisionSupport(m.ModelName, m.ModelInfo)

		// Default to reachable; will be refined by health check below.
		dm.Status = "reachable"

		discovered = append(discovered, dm)
	}

	// Refine reachability via per-model health check.
	report, healthErr := c.HealthDetailed(ctx)
	if healthErr == nil && report != nil {
		unhealthySet := make(map[string]string, len(report.UnhealthyEndpoints))
		for _, ep := range report.UnhealthyEndpoints {
			unhealthySet[ep.Model] = ep.Error
		}
		for i := range discovered {
			if errMsg, bad := unhealthySet[discovered[i].ModelName]; bad {
				discovered[i].Status = "unreachable"
				discovered[i].ErrorDetail = errMsg
			}
		}
	}
	// If health check fails, all models stay "reachable" (graceful degradation).

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
		dm.SupportsVision = inferVisionSupport(m.Name, nil)
		discovered = append(discovered, dm)
	}

	return discovered, nil
}

// SelectStrongestModel delegates to port/llm.SelectStrongestModel.
// Kept here for backward compatibility with existing callers (e.g. tests).
func SelectStrongestModel(models []DiscoveredModel) string {
	return llm.SelectStrongestModel(models)
}

// inferVisionSupport determines if a model supports vision (image) inputs.
// It first checks the model_info metadata from LiteLLM, then falls back to
// name-based heuristics for known vision-capable model families.
func inferVisionSupport(modelName string, modelInfo map[string]any) bool {
	// Check explicit metadata from LiteLLM model_info.
	if modelInfo != nil {
		if v, ok := modelInfo["supports_vision"].(bool); ok {
			return v
		}
	}

	// Fall back to name-based pattern matching.
	name := strings.ToLower(modelName)
	visionPatterns := []string{
		"gpt-4o",
		"gpt-4-vision",
		"claude-3",
		"claude-4",
		"claude-opus-4",
		"claude-sonnet-4",
		"claude-haiku-4",
		"gemini",
	}
	for _, p := range visionPatterns {
		if strings.Contains(name, p) {
			return true
		}
	}
	return false
}

// --- Chat Completion (OpenAI-compatible) ---

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

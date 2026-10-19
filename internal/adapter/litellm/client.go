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

// DefaultCompletionTimeout bounds one chat completion (litellm.completion_timeout).
const DefaultCompletionTimeout = 10 * time.Minute

// Client talks to the LiteLLM Proxy admin API.
type Client struct {
	baseURL   string
	masterKey string
	vault     *secrets.Vault
	// httpClient serves the admin calls (models, health): 10 s.
	httpClient *http.Client
	// completionClient serves chat completions, which take minutes on real
	// and local models (KI-213).
	completionClient *http.Client
	breaker          *resilience.Breaker
	keys             llm.ProviderKeys
}

// NewClient creates a new LiteLLM admin client.
func NewClient(baseURL, masterKey string) *Client {
	return &Client{
		baseURL:   baseURL,
		masterKey: masterKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		completionClient: &http.Client{
			Timeout: DefaultCompletionTimeout,
		},
	}
}

// SetCompletionTimeout sets the timeout of one chat completion, streamed or
// not (litellm.completion_timeout); the admin calls keep theirs.
func (c *Client) SetCompletionTimeout(d time.Duration) {
	c.completionClient = &http.Client{Timeout: d}
}

// SetBreaker attaches a circuit breaker to all outgoing HTTP calls.
func (c *Client) SetBreaker(b *resilience.Breaker) {
	c.breaker = b
}

// SetProviderKeys tells the model listing which providers have an API key:
// their wildcard routes list the models LiteLLM expands them to, the others
// one route row (collapseCatalogueRoutes). Without it no cloud provider
// counts as keyed.
func (c *Client) SetProviderKeys(keys llm.ProviderKeys) {
	c.keys = keys
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
	result.Data = c.collapseCatalogueRoutes(result.Data)
	for i := range result.Data {
		// Infer vision capability from metadata or model name.
		result.Data[i].SupportsVision = inferVisionSupport(
			result.Data[i].ModelName, result.Data[i].ModelInfo,
		)
		if result.Data[i].ModelID == "" {
			result.Data[i].ModelID = deploymentID(result.Data[i].ModelInfo)
		}
	}
	return result.Data, nil
}

// deploymentID returns the deployment ID LiteLLM reports in model_info.id
// (/model/info has no top-level model_id); /model/delete takes this ID.
func deploymentID(info map[string]any) string {
	id, _ := info["id"].(string)
	return id
}

// collapseCatalogueRoutes lists the wildcard route of a provider without an
// API key as one row instead of every model LiteLLM expands it to (KI-125).
// LiteLLM expands a route such as groq/* from its built-in catalogue whether
// or not the key is set (about 600 models nobody can call without keys), and
// a route whose provider lists its models (anthropic/*, openai/*, ...) to
// that list once the key is set. Both kinds of rows share the route's
// deployment ID (model_info.id) and /model/info strips the keys, so whether
// a provider has a key comes from c.keys: a keyed provider keeps its rows (a
// user picks a concrete model, and the default model is one), a keyless one
// becomes one route row ("groq/*"), which stays usable for a model typed by
// name and for the worker's router. A route with an api_base (Ollama, LM
// Studio, OpenAI-compatible services) keeps its rows: LiteLLM lists them
// from the server itself, which answers only when it can be used.
func (c *Client) collapseCatalogueRoutes(rows []Model) []Model {
	counts := make(map[string]int, len(rows))
	for i := range rows {
		if id := deploymentID(rows[i].ModelInfo); id != "" {
			counts[id]++
		}
	}

	type route struct{ id, provider string }
	out := make([]Model, 0, len(rows))
	listed := make(map[route]bool)
	for i := range rows {
		id := deploymentID(rows[i].ModelInfo)
		provider, _, prefixed := strings.Cut(rows[i].ModelName, "/")
		if counts[id] < 2 || !prefixed || hasAPIBase(rows[i].Params) || c.keys.HasKey(provider) {
			out = append(out, rows[i])
			continue
		}
		r := route{id: id, provider: provider}
		if listed[r] {
			continue
		}
		listed[r] = true
		out = append(out, routeRow(&rows[i], id, provider+"/*"))
	}
	return out
}

// routeRow is the row of a collapsed wildcard route: the route's parameters
// (tags, timeout, ...) with the pattern as model, and only the deployment ID
// as model info (an expanded row's pricing and limits belong to one model).
func routeRow(row *Model, id, pattern string) Model {
	params := make(map[string]any, len(row.Params))
	for k, v := range row.Params {
		params[k] = v
	}
	params["model"] = pattern
	return Model{
		ModelName: pattern,
		Provider:  row.Provider,
		ModelID:   id,
		ModelInfo: map[string]any{"id": id},
		Params:    params,
	}
}

func hasAPIBase(params map[string]any) bool {
	base, _ := params["api_base"].(string)
	return base != ""
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
// It asks /health/readiness: /health makes a live model call per checked
// deployment (KI-213).
func (c *Client) Health(ctx context.Context) (bool, error) {
	_, err := c.doRequest(ctx, http.MethodGet, "/health/readiness", nil)
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
		Data []Model `json:"data"`
	}
	if err := json.Unmarshal(infoResp, &infoResult); err != nil {
		return nil, fmt.Errorf("unmarshal model info: %w", err)
	}
	infoResult.Data = c.collapseCatalogueRoutes(infoResult.Data)

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
	for i := range infoResult.Data {
		m := &infoResult.Data[i]
		modelID := m.ModelID
		if modelID == "" {
			modelID = deploymentID(m.ModelInfo)
		}
		dm := DiscoveredModel{
			ModelName: m.ModelName,
			ModelID:   modelID,
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

	data, err := c.doRequestWith(ctx, c.completionClient, http.MethodPost, "/v1/chat/completions", body)
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

	resp, err := c.completionClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("stream request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("reading stream error response: %w", err)
		}
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
	return c.doRequestWith(ctx, c.httpClient, method, path, body)
}

// doRequestWith sends one request with the given client through the breaker.
// A 4xx answer is the request's fault and does not count against LiteLLM
// (resilience.Neutral), nor does the end of ctx; transport errors, the
// client's own timeout and 5xx do (KI-213).
func (c *Client) doRequestWith(ctx context.Context, client *http.Client, method, path string, body []byte) ([]byte, error) {
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

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("http request: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}

		if resp.StatusCode >= 400 {
			apiErr := fmt.Errorf("litellm API error %d: %s", resp.StatusCode, string(data))
			if resp.StatusCode < 500 {
				return resilience.Neutral(apiErr)
			}
			return apiErr
		}

		result = data
		return nil
	}

	if c.breaker != nil {
		if err := c.breaker.ExecuteContext(ctx, call); err != nil {
			return nil, err
		}
		return result, nil
	}

	if err := call(); err != nil {
		return nil, err
	}
	return result, nil
}

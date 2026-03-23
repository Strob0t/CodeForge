// Package llm defines the port-layer types for LLM chat completion operations.
// These types are the canonical definitions; adapter packages (e.g. litellm)
// use type aliases pointing here so that a single set of structs is shared
// across the entire codebase.
package llm

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

// Model represents a configured model in LiteLLM.
type Model struct {
	ModelName      string         `json:"model_name"`
	Provider       string         `json:"litellm_provider,omitempty"`
	ModelID        string         `json:"model_id,omitempty"`
	ModelInfo      map[string]any `json:"model_info,omitempty"`
	Params         map[string]any `json:"litellm_params,omitempty"`
	SupportsVision bool           `json:"supports_vision"`
}

// HealthStatusReport contains detailed per-model health information.
type HealthStatusReport struct {
	HealthyEndpoints   []ModelEndpoint `json:"healthy_endpoints"`
	UnhealthyEndpoints []ModelEndpoint `json:"unhealthy_endpoints"`
	HealthyCount       int             `json:"healthy_count"`
	UnhealthyCount     int             `json:"unhealthy_count"`
}

// ModelEndpoint is a single model endpoint from the health response.
type ModelEndpoint struct {
	Model    string `json:"model"`
	APIBase  string `json:"api_base,omitempty"`
	Provider string `json:"provider,omitempty"`
	Error    string `json:"error,omitempty"`
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

// DiscoveredModel represents a model found via auto-discovery with health status.
type DiscoveredModel struct {
	ModelName      string         `json:"model_name"`
	ModelID        string         `json:"model_id,omitempty"`
	Provider       string         `json:"provider,omitempty"`
	Tags           []string       `json:"tags,omitempty"`
	MaxTokens      int            `json:"max_tokens,omitempty"`
	InputCostPer   float64        `json:"input_cost_per_token,omitempty"`
	OutputCostPer  float64        `json:"output_cost_per_token,omitempty"`
	SupportsVision bool           `json:"supports_vision"`
	Status         string         `json:"status"`                 // "reachable" or "unreachable"
	Source         string         `json:"source"`                 // "litellm" or "ollama"
	ErrorDetail    string         `json:"error_detail,omitempty"` // error reason for unhealthy models
	ModelInfo      map[string]any `json:"model_info,omitempty"`
}

// AddModelRequest is the request body for adding a model to LiteLLM.
type AddModelRequest struct {
	ModelName     string         `json:"model_name"`
	LiteLLMParams map[string]any `json:"litellm_params"`
	ModelInfo     map[string]any `json:"model_info,omitempty"`
}

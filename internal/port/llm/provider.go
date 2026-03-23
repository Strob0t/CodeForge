package llm

import "context"

// Provider abstracts LLM chat completion operations for the service layer.
// The primary adapter is adapter/litellm.Client.
type Provider interface {
	ChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error)
	ChatCompletionStream(ctx context.Context, req ChatCompletionRequest, onChunk func(StreamChunk)) (*ChatCompletionResponse, error)
	ListModels(ctx context.Context) ([]Model, error)
	Health(ctx context.Context) (bool, error)
	HealthDetailed(ctx context.Context) (*HealthStatusReport, error)
}

// ModelDiscoverer extends Provider with model discovery capabilities.
// Not all LLM backends support discovery (e.g. Ollama requires a separate URL).
type ModelDiscoverer interface {
	DiscoverModels(ctx context.Context) ([]DiscoveredModel, error)
	DiscoverOllamaModels(ctx context.Context, ollamaBaseURL string) ([]DiscoveredModel, error)
}

// ModelAdmin extends Provider with model management capabilities.
type ModelAdmin interface {
	AddModel(ctx context.Context, req AddModelRequest) error
	DeleteModel(ctx context.Context, modelID string) error
}

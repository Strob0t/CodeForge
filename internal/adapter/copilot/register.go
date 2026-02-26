package copilot

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
)

// RegisterWithLiteLLM registers the Copilot model with LiteLLM using the
// exchanged bearer token. The model is registered as "copilot/gpt-4" which
// routes through GitHub's Copilot API.
func RegisterWithLiteLLM(ctx context.Context, llm *litellm.Client, bearerToken string) error {
	req := litellm.AddModelRequest{
		ModelName: "copilot/gpt-4",
		LiteLLMParams: map[string]string{
			"model":               "openai/gpt-4",
			"api_key":             bearerToken,
			"api_base":            "https://api.githubcopilot.com",
			"custom_llm_provider": "openai",
		},
	}

	if err := llm.AddModel(ctx, req); err != nil {
		return fmt.Errorf("register copilot model with litellm: %w", err)
	}

	slog.Info("copilot model registered with litellm", "model_name", req.ModelName)
	return nil
}

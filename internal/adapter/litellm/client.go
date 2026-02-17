// Package litellm provides an HTTP client for the LiteLLM Proxy admin API.
package litellm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Strob0t/CodeForge/internal/resilience"
)

// Model represents a configured model in LiteLLM.
type Model struct {
	ModelName string            `json:"model_name"`
	Provider  string            `json:"litellm_provider,omitempty"`
	ModelID   string            `json:"model_id,omitempty"`
	ModelInfo map[string]any    `json:"model_info,omitempty"`
	Params    map[string]string `json:"litellm_params,omitempty"`
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
		if c.masterKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.masterKey)
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

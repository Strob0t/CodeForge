package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Strob0t/CodeForge/internal/adapter/auth"
)

// SubscriptionService orchestrates OAuth device flows for subscription providers.
type SubscriptionService struct {
	providers   map[string]auth.SubscriptionProvider
	envWriter   *EnvWriter
	minInterval time.Duration // minimum polling interval (default 5s)

	mu    sync.Mutex
	flows map[string]*activeFlow // keyed by provider name
}

// activeFlow tracks a running device authorization flow.
type activeFlow struct {
	deviceCode *auth.DeviceCode
	startedAt  time.Time
	cancel     context.CancelFunc
	done       chan flowResult
}

// flowResult is the outcome of a completed device flow.
type flowResult struct {
	apiKey string
	err    error
}

// ProviderInfo describes a subscription provider and its connection status.
type ProviderInfo struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Connected   bool     `json:"connected"`
	EnvVar      string   `json:"env_var"`
	Models      []string `json:"models"`
}

// DeviceFlowStatus is the current state of a device flow.
type DeviceFlowStatus struct {
	Status string `json:"status"` // "pending", "complete", "error"
	Error  string `json:"error,omitempty"`
}

// providerMeta holds static display metadata for known providers.
var providerMeta = map[string]struct {
	displayName string
	description string
	models      []string
}{
	"anthropic": {
		displayName: "Claude Max",
		description: "Connect your Claude Code Max subscription for API access",
		models:      []string{"claude-sonnet-4", "claude-opus-4.6", "claude-haiku-3.5"},
	},
	"github_copilot": {
		displayName: "GitHub Copilot",
		description: "Connect your GitHub Copilot subscription for model access",
		models:      []string{"gpt-4o", "gpt-4o-mini", "o3-mini"},
	},
}

// NewSubscriptionService creates a new SubscriptionService with the given
// providers and .env file path.
func NewSubscriptionService(envPath string, providers ...auth.SubscriptionProvider) *SubscriptionService {
	pm := make(map[string]auth.SubscriptionProvider, len(providers))
	for _, p := range providers {
		pm[p.Name()] = p
	}
	return &SubscriptionService{
		providers:   pm,
		envWriter:   NewEnvWriter(envPath),
		minInterval: 5 * time.Second,
		flows:       make(map[string]*activeFlow),
	}
}

// ListProviders returns info about all registered providers and their
// connection status (based on whether their env var is set in .env).
func (s *SubscriptionService) ListProviders() []ProviderInfo {
	infos := make([]ProviderInfo, 0, len(s.providers))
	for _, p := range s.providers {
		connected, _ := s.envWriter.Has(p.EnvVarName())
		meta := providerMeta[p.Name()]
		infos = append(infos, ProviderInfo{
			Name:        p.Name(),
			DisplayName: meta.displayName,
			Description: meta.description,
			Connected:   connected,
			EnvVar:      p.EnvVarName(),
			Models:      meta.models,
		})
	}
	return infos
}

// StartConnect initiates a device authorization flow for the named provider.
// Returns the device code info the user needs to authorize in their browser.
func (s *SubscriptionService) StartConnect(ctx context.Context, providerName string) (*auth.DeviceCode, error) {
	provider, ok := s.providers[providerName]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", providerName)
	}

	s.mu.Lock()
	if existing, ok := s.flows[providerName]; ok {
		existing.cancel()
		delete(s.flows, providerName)
	}
	s.mu.Unlock()

	dc, err := provider.DeviceFlowStart(ctx)
	if err != nil {
		return nil, fmt.Errorf("device flow start: %w", err)
	}

	flowCtx, cancel := context.WithTimeout(context.Background(), time.Duration(dc.ExpiresIn)*time.Second) //nolint:gosec // G118: cancel stored in flow.cancel, called on cleanup
	done := make(chan flowResult, 1)

	flow := &activeFlow{
		deviceCode: dc,
		startedAt:  time.Now(),
		cancel:     cancel,
		done:       done,
	}

	s.mu.Lock()
	s.flows[providerName] = flow
	s.mu.Unlock()

	go s.pollAndExchange(flowCtx, provider, dc, done)

	slog.Info("subscription connect started",
		"provider", providerName,
		"user_code", dc.UserCode,
		"verification_uri", dc.VerificationURI,
	)

	return dc, nil
}

// GetStatus returns the current status of a device flow for the named provider.
func (s *SubscriptionService) GetStatus(providerName string) DeviceFlowStatus {
	s.mu.Lock()
	flow, ok := s.flows[providerName]
	s.mu.Unlock()

	if !ok {
		connected, _ := s.envWriter.Has(s.envVarForProvider(providerName))
		if connected {
			return DeviceFlowStatus{Status: "complete"}
		}
		return DeviceFlowStatus{Status: "error", Error: "no active flow"}
	}

	select {
	case result := <-flow.done:
		// Flow completed; put result back so subsequent calls see it.
		flow.done <- result
		if result.err != nil {
			return DeviceFlowStatus{Status: "error", Error: result.err.Error()}
		}
		return DeviceFlowStatus{Status: "complete"}
	default:
		return DeviceFlowStatus{Status: "pending"}
	}
}

// Disconnect removes the API key for the named provider from .env.
func (s *SubscriptionService) Disconnect(providerName string) error {
	provider, ok := s.providers[providerName]
	if !ok {
		return fmt.Errorf("unknown provider: %s", providerName)
	}

	s.mu.Lock()
	if flow, ok := s.flows[providerName]; ok {
		flow.cancel()
		delete(s.flows, providerName)
	}
	s.mu.Unlock()

	if err := s.envWriter.Delete(provider.EnvVarName()); err != nil {
		return fmt.Errorf("delete env var: %w", err)
	}

	slog.Info("subscription disconnected", "provider", providerName)
	return nil
}

// pollAndExchange runs in a goroutine, polling the device flow and exchanging
// the token for an API key on success.
func (s *SubscriptionService) pollAndExchange(
	ctx context.Context,
	provider auth.SubscriptionProvider,
	dc *auth.DeviceCode,
	done chan<- flowResult,
) {
	defer func() {
		s.mu.Lock()
		delete(s.flows, provider.Name())
		s.mu.Unlock()
	}()

	interval := max(time.Duration(dc.Interval)*time.Second, s.minInterval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			done <- flowResult{err: ctx.Err()}
			return
		case <-ticker.C:
			token, err := provider.DeviceFlowPoll(ctx, dc.DeviceCode)
			if errors.Is(err, auth.ErrAuthPending) {
				continue
			}
			if errors.Is(err, auth.ErrSlowDown) {
				interval += 5 * time.Second
				ticker.Reset(interval)
				continue
			}
			if err != nil {
				done <- flowResult{err: fmt.Errorf("poll: %w", err)}
				return
			}

			apiKey, err := provider.ExchangeForAPIKey(ctx, token)
			if err != nil {
				done <- flowResult{err: fmt.Errorf("exchange: %w", err)}
				return
			}

			if err := s.envWriter.Set(provider.EnvVarName(), apiKey); err != nil {
				done <- flowResult{err: fmt.Errorf("write env: %w", err)}
				return
			}

			slog.Info("subscription connected",
				"provider", provider.Name(),
				"env_var", provider.EnvVarName(),
			)
			done <- flowResult{apiKey: apiKey}
			return
		}
	}
}

// envVarForProvider returns the env var name for a provider, or empty string.
func (s *SubscriptionService) envVarForProvider(name string) string {
	if p, ok := s.providers[name]; ok {
		return p.EnvVarName()
	}
	return ""
}

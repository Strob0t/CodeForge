package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/domain/prompt"
	mq "github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// PromptEvolutionStore extends PromptVariantStore with mutation and promotion operations.
type PromptEvolutionStore interface {
	PromptVariantStore
	InsertVariant(ctx context.Context, v *prompt.PromptVariant) error
	GetVariantByID(ctx context.Context, id string) (prompt.PromptVariant, error)
	UpdatePromotionStatus(ctx context.Context, id string, status prompt.PromotionStatus) error
	ListVariants(ctx context.Context, modeID, status string) ([]prompt.PromptVariant, error)
}

// PromptEvolutionService orchestrates the four-stage evolution loop:
// SCORE → REFLECT → MUTATE → SELECT.
type PromptEvolutionService struct {
	queue  mq.Queue
	store  PromptEvolutionStore
	config *prompt.EvolutionConfig
}

// NewPromptEvolutionService creates a new evolution orchestrator.
func NewPromptEvolutionService(queue mq.Queue, store PromptEvolutionStore, config *prompt.EvolutionConfig) *PromptEvolutionService {
	return &PromptEvolutionService{
		queue:  queue,
		store:  store,
		config: config,
	}
}

// TriggerReflection publishes a reflect request to NATS for the Python worker.
func (s *PromptEvolutionService) TriggerReflection(
	ctx context.Context,
	tenantID, modeID, modelFamily, currentPrompt string,
	failures []map[string]json.RawMessage,
) error {
	payload := mq.PromptEvolutionReflectPayload{
		TenantID:      tenantID,
		ModeID:        modeID,
		ModelFamily:   modelFamily,
		CurrentPrompt: currentPrompt,
		Failures:      failures,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal reflect payload: %w", err)
	}

	slog.Info("triggering prompt evolution reflection",
		"tenant_id", tenantID,
		"mode_id", modeID,
		"model_family", modelFamily,
		"failure_count", len(failures),
	)

	return s.queue.Publish(ctx, mq.SubjectPromptEvolutionReflect, data)
}

// HandleMutateComplete processes a mutation result from the Python worker
// and stores the variant as a candidate.
func (s *PromptEvolutionService) HandleMutateComplete(ctx context.Context, data []byte) error {
	var payload mq.PromptEvolutionMutateCompletePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("unmarshal mutate complete: %w", err)
	}

	if payload.Error != "" {
		slog.Warn("prompt mutation returned error",
			"mode_id", payload.ModeID,
			"error", payload.Error,
		)
		return nil
	}

	if !payload.ValidationPassed {
		slog.Warn("prompt mutation variant failed validation, skipping",
			"mode_id", payload.ModeID,
		)
		return nil
	}

	if s.store == nil {
		return nil
	}

	variant := prompt.PromptVariant{
		TenantID:        payload.TenantID,
		ModeID:          payload.ModeID,
		ModelFamily:     payload.ModelFamily,
		Content:         payload.VariantContent,
		Version:         payload.Version,
		ParentID:        payload.ParentID,
		MutationSource:  payload.MutationSource,
		PromotionStatus: prompt.PromotionCandidate,
		Enabled:         true,
	}

	if err := s.store.InsertVariant(ctx, &variant); err != nil {
		return fmt.Errorf("insert variant: %w", err)
	}

	slog.Info("stored new prompt variant",
		"mode_id", payload.ModeID,
		"model_family", payload.ModelFamily,
		"version", payload.Version,
		"source", payload.MutationSource,
	)
	return nil
}

// HandleReflectComplete processes a prompt.evolution.reflect.complete message from the Python worker.
// It logs the tactical fixes and strategic principles received from the reflection step.
func (s *PromptEvolutionService) HandleReflectComplete(_ context.Context, _ string, data []byte) error {
	var payload mq.PromptEvolutionReflectCompletePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("unmarshal reflect complete: %w", err)
	}

	if payload.Error != "" {
		slog.Warn("prompt reflection returned error",
			"mode_id", payload.ModeID,
			"model_family", payload.ModelFamily,
			"error", payload.Error,
		)
		return nil
	}

	slog.Info("prompt reflection complete",
		"mode_id", payload.ModeID,
		"model_family", payload.ModelFamily,
		"tactical_fixes", len(payload.TacticalFixes),
		"strategic_principles", len(payload.StrategicPrinciples),
	)
	return nil
}

// handleMutateCompleteMsg wraps HandleMutateComplete with the mq.Handler signature.
func (s *PromptEvolutionService) handleMutateCompleteMsg(ctx context.Context, _ string, data []byte) error {
	return s.HandleMutateComplete(ctx, data)
}

// StartSubscribers subscribes to prompt evolution result subjects and returns cancel functions.
func (s *PromptEvolutionService) StartSubscribers(ctx context.Context) ([]func(), error) {
	cancelReflect, err := s.queue.Subscribe(ctx, mq.SubjectPromptEvolutionReflectComplete, s.HandleReflectComplete)
	if err != nil {
		return nil, fmt.Errorf("subscribe reflect complete: %w", err)
	}

	cancelMutate, err := s.queue.Subscribe(ctx, mq.SubjectPromptEvolutionMutateComplete, s.handleMutateCompleteMsg)
	if err != nil {
		cancelReflect()
		return nil, fmt.Errorf("subscribe mutate complete: %w", err)
	}

	return []func(){cancelReflect, cancelMutate}, nil
}

// PromoteVariant promotes a candidate variant, retiring any previously promoted variant
// for the same mode and model family.
func (s *PromptEvolutionService) PromoteVariant(ctx context.Context, tenantID, variantID string) error {
	if s.store == nil {
		return fmt.Errorf("no store configured")
	}

	candidate, err := s.store.GetVariantByID(ctx, variantID)
	if err != nil {
		return fmt.Errorf("get variant %s: %w", variantID, err)
	}

	// Retire existing promoted variants for the same mode + model.
	existing, err := s.store.GetVariantsByModeAndModel(ctx, candidate.ModeID, candidate.ModelFamily)
	if err != nil {
		return fmt.Errorf("get existing variants: %w", err)
	}
	for i := range existing {
		if existing[i].PromotionStatus == prompt.PromotionPromoted {
			if err := s.store.UpdatePromotionStatus(ctx, existing[i].ID, prompt.PromotionRetired); err != nil {
				return fmt.Errorf("retire variant %s: %w", existing[i].ID, err)
			}
		}
	}

	// Promote the candidate.
	if err := s.store.UpdatePromotionStatus(ctx, variantID, prompt.PromotionPromoted); err != nil {
		return fmt.Errorf("promote variant %s: %w", variantID, err)
	}

	slog.Info("promoted prompt variant",
		"variant_id", variantID,
		"mode_id", candidate.ModeID,
		"model_family", candidate.ModelFamily,
	)

	// Publish promoted event.
	event := mq.PromptEvolutionEventPayload{
		TenantID:  tenantID,
		ModeID:    candidate.ModeID,
		VariantID: variantID,
		Action:    "promoted",
	}
	eventData, _ := json.Marshal(event)
	return s.queue.Publish(ctx, mq.SubjectPromptEvolutionPromoted, eventData)
}

// RevertMode retires all variants for a mode, reverting to base YAML prompts.
func (s *PromptEvolutionService) RevertMode(ctx context.Context, tenantID, modeID string) error {
	if s.store == nil {
		return fmt.Errorf("no store configured")
	}

	// Get all variants for this mode across model families.
	// We iterate all model families by getting variants with an empty model family
	// and then checking. In practice the store should support mode-only queries,
	// but for now we use a simple approach.
	for _, modelFamily := range []string{"openai", "anthropic", "google", "meta", "local"} {
		variants, err := s.store.GetVariantsByModeAndModel(ctx, modeID, modelFamily)
		if err != nil {
			continue
		}
		for i := range variants {
			if variants[i].PromotionStatus != prompt.PromotionRetired {
				if err := s.store.UpdatePromotionStatus(ctx, variants[i].ID, prompt.PromotionRetired); err != nil {
					slog.Error("failed to retire variant", "variant_id", variants[i].ID, "error", err)
				}
			}
		}
	}

	slog.Info("reverted mode to base prompts", "mode_id", modeID)

	event := mq.PromptEvolutionEventPayload{
		TenantID: tenantID,
		ModeID:   modeID,
		Action:   "reverted",
	}
	eventData, _ := json.Marshal(event)
	return s.queue.Publish(ctx, mq.SubjectPromptEvolutionReverted, eventData)
}

// ListVariants returns all prompt variants for a tenant, optionally filtered by mode and status.
func (s *PromptEvolutionService) ListVariants(ctx context.Context, modeID, status string) ([]prompt.PromptVariant, error) {
	if s.store == nil {
		return nil, fmt.Errorf("no store configured")
	}
	return s.store.ListVariants(ctx, modeID, status)
}

// GetStatus returns the current evolution configuration status.
func (s *PromptEvolutionService) GetStatus() prompt.EvolutionStatus {
	return prompt.EvolutionStatus{
		Enabled:  s.config.Enabled,
		Trigger:  s.config.Trigger,
		Strategy: s.config.PromotionStrategy,
	}
}

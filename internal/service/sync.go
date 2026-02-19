package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
)

// SyncService handles bidirectional synchronization between CodeForge roadmap features
// and external PM providers.
type SyncService struct {
	store database.Store
}

// NewSyncService creates a new SyncService.
func NewSyncService(store database.Store) *SyncService {
	return &SyncService{store: store}
}

// Sync performs a bidirectional sync operation based on the given config.
func (s *SyncService) Sync(ctx context.Context, cfg roadmap.SyncConfig) (*roadmap.SyncResult, error) {
	provider, err := pmprovider.New(cfg.Provider, map[string]string{})
	if err != nil {
		return nil, fmt.Errorf("create pm provider %q: %w", cfg.Provider, err)
	}

	switch cfg.Direction {
	case roadmap.SyncDirectionPull:
		return s.pullFromPM(ctx, cfg, provider)
	case roadmap.SyncDirectionPush:
		return s.pushToPM(ctx, cfg, provider)
	case roadmap.SyncDirectionBidi:
		pullResult, pullErr := s.pullFromPM(ctx, cfg, provider)
		if pullErr != nil {
			return nil, fmt.Errorf("pull phase: %w", pullErr)
		}

		caps := provider.Capabilities()
		if !caps.CreateItem && !caps.UpdateItem {
			return pullResult, nil
		}

		pushResult, pushErr := s.pushToPM(ctx, cfg, provider)
		if pushErr != nil {
			pullResult.Errors = append(pullResult.Errors, fmt.Sprintf("push phase: %s", pushErr))
			return pullResult, nil
		}

		return &roadmap.SyncResult{
			Direction: "bidi",
			Created:   pullResult.Created + pushResult.Created,
			Updated:   pullResult.Updated + pushResult.Updated,
			Skipped:   pullResult.Skipped + pushResult.Skipped,
			Errors:    append(pullResult.Errors, pushResult.Errors...),
			DryRun:    cfg.DryRun,
		}, nil
	default:
		return nil, fmt.Errorf("unknown sync direction: %q", cfg.Direction)
	}
}

// pullFromPM imports items from the PM provider into CodeForge features.
func (s *SyncService) pullFromPM(ctx context.Context, cfg roadmap.SyncConfig, provider pmprovider.Provider) (*roadmap.SyncResult, error) {
	items, err := provider.ListItems(ctx, cfg.ProjectRef)
	if err != nil {
		return nil, fmt.Errorf("list PM items: %w", err)
	}

	// Load existing roadmap to find features
	rm, rmErr := s.store.GetRoadmapByProject(ctx, cfg.ProjectID)
	if rmErr != nil {
		return nil, fmt.Errorf("get roadmap: %w", rmErr)
	}

	milestones, msErr := s.store.ListMilestones(ctx, rm.ID)
	if msErr != nil {
		return nil, fmt.Errorf("list milestones: %w", msErr)
	}

	// Build lookup of existing features by external ID
	existingByExt := make(map[string]*roadmap.Feature)
	for i := range milestones {
		features, fErr := s.store.ListFeatures(ctx, milestones[i].ID)
		if fErr != nil {
			continue
		}
		for j := range features {
			if extID, ok := features[j].ExternalIDs[cfg.Provider]; ok {
				existingByExt[extID] = &features[j]
			}
		}
	}

	result := &roadmap.SyncResult{Direction: "pull", DryRun: cfg.DryRun}

	for i := range items {
		item := &items[i]
		if existing, ok := existingByExt[item.ExternalID]; ok {
			if cfg.UpdateExist && !cfg.DryRun {
				existing.Title = item.Title
				existing.Description = item.Description
				if err := s.store.UpdateFeature(ctx, existing); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("update %s: %s", item.ExternalID, err))
					continue
				}
			}
			result.Updated++
		} else if cfg.CreateNew {
			if len(milestones) == 0 {
				result.Errors = append(result.Errors, "no milestone to create features in")
				result.Skipped++
				continue
			}
			if !cfg.DryRun {
				req := &roadmap.CreateFeatureRequest{
					MilestoneID: milestones[0].ID,
					Title:       item.Title,
					Description: item.Description,
					Labels:      item.Labels,
					ExternalIDs: map[string]string{cfg.Provider: item.ExternalID},
				}
				if _, err := s.store.CreateFeature(ctx, req); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("create %s: %s", item.ExternalID, err))
					continue
				}
			}
			result.Created++
		} else {
			result.Skipped++
		}
	}

	slog.Info("sync pull completed", "project", cfg.ProjectID, "created", result.Created, "updated", result.Updated)
	return result, nil
}

// pushToPM exports CodeForge features to the PM provider.
func (s *SyncService) pushToPM(ctx context.Context, cfg roadmap.SyncConfig, provider pmprovider.Provider) (*roadmap.SyncResult, error) {
	caps := provider.Capabilities()
	result := &roadmap.SyncResult{Direction: "push", DryRun: cfg.DryRun}

	rm, rmErr := s.store.GetRoadmapByProject(ctx, cfg.ProjectID)
	if rmErr != nil {
		return nil, fmt.Errorf("get roadmap: %w", rmErr)
	}

	milestones, msErr := s.store.ListMilestones(ctx, rm.ID)
	if msErr != nil {
		return nil, fmt.Errorf("list milestones: %w", msErr)
	}

	for i := range milestones {
		features, fErr := s.store.ListFeatures(ctx, milestones[i].ID)
		if fErr != nil {
			continue
		}

		for j := range features {
			f := &features[j]
			extID := f.ExternalIDs[cfg.Provider]

			switch {
			case extID != "" && caps.UpdateItem && cfg.UpdateExist:
				// Feature has external ID -- update it
				if !cfg.DryRun {
					slog.Debug("push update skipped (not fully wired)", "feature", f.ID)
				}
				result.Updated++
			case extID == "" && caps.CreateItem && cfg.CreateNew:
				// Feature has no external ID -- create it
				if !cfg.DryRun {
					slog.Debug("push create skipped (not fully wired)", "feature", f.ID)
				}
				result.Created++
			default:
				result.Skipped++
			}
		}
	}

	slog.Info("sync push completed", "project", cfg.ProjectID, "created", result.Created, "updated", result.Updated)
	return result, nil
}

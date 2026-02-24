package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain/settings"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// SettingsService provides CRUD operations for application settings.
type SettingsService struct {
	store database.Store
}

// NewSettingsService creates a new SettingsService.
func NewSettingsService(store database.Store) *SettingsService {
	return &SettingsService{store: store}
}

// List returns all settings for the current tenant.
func (s *SettingsService) List(ctx context.Context) ([]settings.Setting, error) {
	return s.store.ListSettings(ctx)
}

// Get returns a single setting by key.
func (s *SettingsService) Get(ctx context.Context, key string) (*settings.Setting, error) {
	if key == "" {
		return nil, fmt.Errorf("setting key is required")
	}
	return s.store.GetSetting(ctx, key)
}

// Update upserts one or more settings from the given map.
func (s *SettingsService) Update(ctx context.Context, req settings.UpdateRequest) error {
	for key, value := range req.Settings {
		if key == "" {
			return fmt.Errorf("setting key must not be empty")
		}
		if !json.Valid(value) {
			return fmt.Errorf("invalid JSON value for setting %q", key)
		}
		if err := s.store.UpsertSetting(ctx, key, value); err != nil {
			return err
		}
	}
	return nil
}

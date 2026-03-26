package database

import (
	"context"
	"encoding/json"

	"github.com/Strob0t/CodeForge/internal/domain/settings"
)

// SettingsStore defines database operations for application settings.
type SettingsStore interface {
	ListSettings(ctx context.Context) ([]settings.Setting, error)
	GetSetting(ctx context.Context, key string) (*settings.Setting, error)
	UpsertSetting(ctx context.Context, key string, value json.RawMessage) error
}

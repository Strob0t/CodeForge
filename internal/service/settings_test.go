package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/settings"
	"github.com/Strob0t/CodeForge/internal/service"
)

// settingsMockStore provides an in-memory store for settings operations.
type settingsMockStore struct {
	runtimeMockStore
	data map[string]json.RawMessage
}

func newSettingsMockStore() *settingsMockStore {
	return &settingsMockStore{data: make(map[string]json.RawMessage)}
}

func (m *settingsMockStore) ListSettings(_ context.Context) ([]settings.Setting, error) {
	result := make([]settings.Setting, 0, len(m.data))
	for k, v := range m.data {
		result = append(result, settings.Setting{Key: k, Value: v})
	}
	return result, nil
}

func (m *settingsMockStore) GetSetting(_ context.Context, key string) (*settings.Setting, error) {
	v, ok := m.data[key]
	if !ok {
		return nil, errMockNotFound
	}
	return &settings.Setting{Key: key, Value: v}, nil
}

func (m *settingsMockStore) UpsertSetting(_ context.Context, key string, value json.RawMessage) error {
	m.data[key] = value
	return nil
}

func TestSettingsService_GetSet(t *testing.T) {
	store := newSettingsMockStore()
	svc := service.NewSettingsService(store)
	ctx := context.Background()

	// 1. List returns empty initially
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 settings, got %d", len(list))
	}

	// 2. Update with valid JSON succeeds
	err = svc.Update(ctx, settings.UpdateRequest{
		Settings: map[string]json.RawMessage{
			"theme": json.RawMessage(`"dark"`),
		},
	})
	if err != nil {
		t.Fatalf("Update theme: %v", err)
	}

	// 3. Get returns the stored value
	got, err := svc.Get(ctx, "theme")
	if err != nil {
		t.Fatalf("Get theme: %v", err)
	}
	if string(got.Value) != `"dark"` {
		t.Errorf("expected value %q, got %q", `"dark"`, string(got.Value))
	}

	// 4. Empty key rejected
	err = svc.Update(ctx, settings.UpdateRequest{
		Settings: map[string]json.RawMessage{
			"": json.RawMessage(`"value"`),
		},
	})
	if err == nil {
		t.Fatal("expected error for empty key")
	}

	// 5. Invalid JSON rejected
	err = svc.Update(ctx, settings.UpdateRequest{
		Settings: map[string]json.RawMessage{
			"bad": json.RawMessage(`not-json`),
		},
	})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}

	// 6. Get with empty key rejected
	_, err = svc.Get(ctx, "")
	if err == nil {
		t.Fatal("expected error for empty key in Get")
	}
}

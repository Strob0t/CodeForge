package postgres_test

import (
	"strings"
	"testing"
)

// --------------------------------------------------------------------------
// TestSettingsStore_TenantIsolation (FIX-010, FIX-031)
// --------------------------------------------------------------------------

func TestSettingsStore_TenantIsolation(t *testing.T) {
	const filename = "store_settings.go"
	content := readStoreSource(t, filename)

	t.Run("ContainsTenantID", func(t *testing.T) {
		assertFileContainsTenantID(t, content, filename)
	})

	t.Run("UsesTenantFromCtx", func(t *testing.T) {
		assertFileUsesTenantFromCtx(t, content, filename)
	})

	t.Run("AllQueriesHaveTenantID", func(t *testing.T) {
		assertSQLQueriesHaveTenantID(t, content, filename, nil)
	})

	t.Run("TenantScopedMethods", func(t *testing.T) {
		methods := []string{
			"ListSettings",
			"GetSetting",
			"UpsertSetting",
		}
		for _, method := range methods {
			t.Run(method, func(t *testing.T) {
				idx := strings.Index(content, "func (s *Store) "+method)
				if idx == -1 {
					t.Fatalf("method %s not found in %s", method, filename)
				}
				body := content[idx:]
				if endIdx := strings.Index(body[1:], "\nfunc "); endIdx != -1 {
					body = body[:endIdx+1]
				}
				if !strings.Contains(body, "tenantFromCtx") {
					t.Errorf("%s.%s: must call tenantFromCtx(ctx)", filename, method)
				}
			})
		}
	})
}

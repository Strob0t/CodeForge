package postgres_test

import (
	"strings"
	"testing"
)

// --------------------------------------------------------------------------
// TestAPIKeyStore_TenantIsolation (FIX-010, FIX-011, FIX-031)
// --------------------------------------------------------------------------

func TestAPIKeyStore_TenantIsolation(t *testing.T) {
	const filename = "store_api_key.go"
	content := readStoreSource(t, filename)

	t.Run("ContainsTenantID", func(t *testing.T) {
		assertFileContainsTenantID(t, content, filename)
	})

	t.Run("UsesTenantFromCtx", func(t *testing.T) {
		assertFileUsesTenantFromCtx(t, content, filename)
	})

	t.Run("AllQueriesHaveTenantID", func(t *testing.T) {
		// GetAPIKeyByHash is intentionally cross-tenant (auth lookup).
		assertSQLQueriesHaveTenantID(t, content, filename, []string{"GetAPIKeyByHash"})
	})

	t.Run("GetAPIKeyByHash_DocumentedCrossTenant", func(t *testing.T) {
		// Verify GetAPIKeyByHash is explicitly documented as cross-tenant.
		idx := strings.Index(content, "GetAPIKeyByHash")
		if idx == -1 {
			t.Fatal("GetAPIKeyByHash method not found")
		}
		// Check the surrounding comment block for the word "cross-tenant".
		commentStart := idx - 200
		if commentStart < 0 {
			commentStart = 0
		}
		surrounding := content[commentStart:idx]
		if !strings.Contains(surrounding, "cross-tenant") {
			t.Error("GetAPIKeyByHash must be documented as intentionally cross-tenant")
		}
	})

	t.Run("TenantScopedMethods_UseTenantFromCtx", func(t *testing.T) {
		// These methods MUST use tenantFromCtx.
		methods := []string{
			"CreateAPIKey",
			"ListAPIKeysByUser",
			"DeleteAPIKey",
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

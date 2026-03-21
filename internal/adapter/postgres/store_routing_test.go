package postgres_test

import (
	"strings"
	"testing"
)

// --------------------------------------------------------------------------
// TestRoutingStore_TenantIsolation (FIX-010, FIX-031)
// --------------------------------------------------------------------------

func TestRoutingStore_TenantIsolation(t *testing.T) {
	const filename = "store_routing.go"
	content := readStoreSource(t, filename)

	t.Run("ContainsTenantID", func(t *testing.T) {
		assertFileContainsTenantID(t, content, filename)
	})

	t.Run("UsesTenantFromCtx", func(t *testing.T) {
		assertFileUsesTenantFromCtx(t, content, filename)
	})

	t.Run("AllQueriesHaveTenantID", func(t *testing.T) {
		// AggregateRoutingOutcomes groups by tenant_id (cross-tenant aggregate).
		// It uses GROUP BY tenant_id so results are still tenant-scoped.
		assertSQLQueriesHaveTenantID(t, content, filename, []string{"AggregateRoutingOutcomes"})
	})

	t.Run("TenantScopedMethods", func(t *testing.T) {
		methods := []string{
			"CreateRoutingOutcome",
			"ListRoutingStats",
			"UpsertRoutingStats",
			"ListRoutingOutcomes",
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

	t.Run("AggregateRoutingOutcomes_GroupsByTenant", func(t *testing.T) {
		// AggregateRoutingOutcomes does not use tenantFromCtx but
		// groups by tenant_id so results are implicitly scoped.
		if !strings.Contains(content, "GROUP BY tenant_id") {
			t.Error("AggregateRoutingOutcomes must GROUP BY tenant_id")
		}
	})
}

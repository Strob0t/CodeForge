package postgres_test

import (
	"strings"
	"testing"
)

// --------------------------------------------------------------------------
// TestProjectGoalStore_TenantIsolation (FIX-010, FIX-011, FIX-031)
// --------------------------------------------------------------------------

func TestProjectGoalStore_TenantIsolation(t *testing.T) {
	const filename = "store_project_goal.go"
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

	t.Run("AllExportedMethods_UseTenant", func(t *testing.T) {
		// Every exported method in this store must reference tenantFromCtx.
		methods := []string{
			"CreateProjectGoal",
			"GetProjectGoal",
			"ListProjectGoals",
			"ListEnabledGoals",
			"UpdateProjectGoal",
			"DeleteProjectGoal",
			"DeleteProjectGoalsBySource",
		}
		for _, method := range methods {
			t.Run(method, func(t *testing.T) {
				// Find the method and check its body references tenantFromCtx.
				idx := strings.Index(content, "func (s *Store) "+method)
				if idx == -1 {
					t.Fatalf("method %s not found in %s", method, filename)
				}
				// Check the next 20 lines for tenantFromCtx.
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

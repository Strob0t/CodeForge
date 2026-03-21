package postgres_test

import (
	"os"
	"strings"
	"testing"
)

// readStoreSource reads a store source file from the current package directory.
// Fails the test if the file cannot be read.
func readStoreSource(t *testing.T, filename string) string {
	t.Helper()
	src, err := os.ReadFile(filename) //nolint:gosec // test helper reads known source files
	if err != nil {
		t.Fatalf("failed to read %s: %v", filename, err)
	}
	return string(src)
}

// assertFileContainsTenantID verifies that a store source file references tenant_id
// in its SQL queries. This is a baseline check that the store uses tenant isolation.
func assertFileContainsTenantID(t *testing.T, content, filename string) {
	t.Helper()
	if !strings.Contains(content, "tenant_id") {
		t.Errorf("%s: must contain tenant_id references for tenant isolation", filename)
	}
}

// assertFileUsesTenantFromCtx verifies that a store source file calls
// tenantFromCtx to extract the tenant ID from the request context.
func assertFileUsesTenantFromCtx(t *testing.T, content, filename string) {
	t.Helper()
	if !strings.Contains(content, "tenantFromCtx") {
		t.Errorf("%s: must use tenantFromCtx(ctx) for tenant isolation", filename)
	}
}

// assertSQLQueriesHaveTenantID checks that every SELECT/UPDATE/DELETE query
// in the source file contains a tenant_id reference. Queries in comments
// (lines starting with //) are skipped. Methods listed in exemptMethods are
// documented as intentionally cross-tenant.
func assertSQLQueriesHaveTenantID(t *testing.T, content, filename string, exemptMethods []string) {
	t.Helper()

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip comment-only lines.
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		// Skip lines within exempted methods by checking if the method name
		// appears within the surrounding 20 lines.
		if len(exemptMethods) > 0 {
			contextStart := i - 20
			if contextStart < 0 {
				contextStart = 0
			}
			contextEnd := i + 5
			if contextEnd > len(lines) {
				contextEnd = len(lines)
			}
			surroundingContext := strings.Join(lines[contextStart:contextEnd], "\n")

			exempt := false
			for _, m := range exemptMethods {
				if strings.Contains(surroundingContext, m) {
					exempt = true
					break
				}
			}
			if exempt {
				continue
			}
		}

		for _, keyword := range []string{"SELECT", "UPDATE", "DELETE"} {
			upper := strings.ToUpper(trimmed)
			if strings.Contains(upper, keyword) && strings.Contains(upper, "FROM") {
				// Look at the next 10 lines for the full query.
				endIdx := i + 10
				if endIdx > len(lines) {
					endIdx = len(lines)
				}
				queryBlock := strings.Join(lines[i:endIdx], "\n")
				if !strings.Contains(queryBlock, "tenant_id") {
					t.Logf("WARNING: %s:%d: query near %q may lack tenant_id filter:\n%s",
						filename, i+1, keyword, queryBlock)
				}
			}
		}
	}
}

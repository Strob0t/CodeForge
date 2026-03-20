package postgres_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestReleaseStaleWork_IntentionallyCrossTenant verifies that ReleaseStaleWork
// is documented as an intentionally cross-tenant system-level operation.
// This test guards against accidental regression of the documentation.
func TestReleaseStaleWork_IntentionallyCrossTenant(t *testing.T) {
	const expectedComment = "INTENTIONALLY CROSS-TENANT"
	src := readSourceFile(t, "store_active_work.go")
	if !strings.Contains(src, expectedComment) {
		t.Fatalf("ReleaseStaleWork must contain %q comment documenting it as intentionally cross-tenant", expectedComment)
	}
}

// readSourceFile reads a Go source file from the adapter/postgres package.
func readSourceFile(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location")
	}
	dir := filepath.Dir(thisFile)
	data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // test reads from known package dir
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

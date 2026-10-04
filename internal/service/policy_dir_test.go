package service_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// loadPolicyDir starts a PolicyService on dir the way cmd/codeforge does.
func loadPolicyDir(t *testing.T, dir string) *service.PolicyService {
	t.Helper()
	svc := service.NewPolicyService("headless-safe-sandbox", nil)
	if err := svc.LoadPolicyDir(dir); err != nil {
		t.Fatalf("load policy dir: %v", err)
	}
	return svc
}

func writePolicyFile(t *testing.T, dir, file, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func dirFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// defaultTenantDir is where the default tenant's profiles are stored (KI-68);
// a context without a tenant is the default tenant.
func defaultTenantDir(dir string) string {
	return filepath.Join(dir, tenantctx.DefaultTenantID)
}

var allowMake = policy.PermissionRule{
	Specifier:    policy.ToolSpecifier{Tool: "Bash"},
	Decision:     policy.DecisionAllow,
	CommandAllow: []string{"make"},
}

// Review finding 13: the loader takes profile names from the file content
// and also reads .yml, but persistence always wrote <name>.yaml. A change
// to a profile loaded from team.yml created a second file for the same
// profile, and the stale team.yml competed with it at the next start.
func TestPolicyDir_ChangesAreWrittenToTheSourceFile(t *testing.T) {
	dir := t.TempDir()
	tenantDir := defaultTenantDir(dir)
	writePolicyFile(t, tenantDir, "team.yml", "name: team-policy\nmode: default\n")
	svc := loadPolicyDir(t, dir)

	if err := svc.PrependRule(context.Background(), "team-policy", &allowMake); err != nil {
		t.Fatalf("PrependRule: %v", err)
	}
	if got := dirFiles(t, tenantDir); len(got) != 1 || got[0] != "team.yml" {
		t.Fatalf("expected only team.yml, got %v", got)
	}
	restarted := loadPolicyDir(t, dir)
	p, ok := restarted.GetProfile(context.Background(), "team-policy")
	if !ok || len(p.Rules) != 1 || p.Rules[0].CommandAllow[0] != "make" {
		t.Fatalf("rule not persisted in team.yml: %+v", p)
	}
}

// Deleting a profile removes the file that defines it, so it does not come
// back at the next start.
func TestPolicyDir_DeleteRemovesTheSourceFile(t *testing.T) {
	dir := t.TempDir()
	tenantDir := defaultTenantDir(dir)
	writePolicyFile(t, tenantDir, "team.yml", "name: team-policy\nmode: default\n")
	svc := loadPolicyDir(t, dir)

	if err := svc.DeleteProfile(context.Background(), "team-policy"); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	if got := dirFiles(t, tenantDir); len(got) != 0 {
		t.Fatalf("expected an empty tenant policy dir, got %v", got)
	}
	if _, ok := loadPolicyDir(t, dir).GetProfile(context.Background(), "team-policy"); ok {
		t.Fatal("deleted profile is back after a restart")
	}
}

// A new profile never overwrites the file of another profile.
func TestPolicyDir_NewProfileDoesNotOverwriteAnotherProfilesFile(t *testing.T) {
	dir := t.TempDir()
	writePolicyFile(t, defaultTenantDir(dir), "a.yaml", "name: b\nmode: default\n")
	svc := loadPolicyDir(t, dir)

	err := svc.SaveProfile(context.Background(), &policy.PolicyProfile{Name: "a", Mode: policy.ModeDefault})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	restarted := loadPolicyDir(t, dir)
	if _, ok := restarted.GetProfile(context.Background(), "b"); !ok {
		t.Fatal("profile b lost: its file was overwritten")
	}
	if _, ok := svc.GetProfile(context.Background(), "a"); ok {
		t.Fatal("rejected profile is in memory")
	}
}

// New profiles are written to <tenant>/<name>.yaml and loaded again.
func TestPolicyDir_NewProfileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	svc := loadPolicyDir(t, dir)
	if err := svc.SaveProfile(context.Background(), &policy.PolicyProfile{Name: "fresh", Mode: policy.ModeDefault}); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(defaultTenantDir(dir), "fresh.yaml")); err != nil {
		t.Fatalf("expected fresh.yaml: %v", err)
	}
	if _, ok := loadPolicyDir(t, dir).GetProfile(context.Background(), "fresh"); !ok {
		t.Fatal("new profile not loaded after restart")
	}
}

// Two files that define the same profile are ambiguous: startup fails
// instead of picking one silently, in the flat layout and in a tenant.
func TestPolicyDir_DuplicateProfileNamesFail(t *testing.T) {
	for _, sub := range []string{"", tenantctx.DefaultTenantID} {
		dir := t.TempDir()
		writePolicyFile(t, filepath.Join(dir, sub), "one.yaml", "name: same\nmode: default\n")
		writePolicyFile(t, filepath.Join(dir, sub), "two.yml", "name: same\nmode: plan\n")
		err := service.NewPolicyService("headless-safe-sandbox", nil).LoadPolicyDir(dir)
		if err == nil || !strings.Contains(err.Error(), "one.yaml") || !strings.Contains(err.Error(), "two.yml") {
			t.Fatalf("%q: expected a duplicate profile error naming both files, got %v", sub, err)
		}
	}
}

// A missing directory is created on the first write.
func TestPolicyDir_MissingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "policies")
	svc := loadPolicyDir(t, dir)
	if err := svc.SaveProfile(context.Background(), &policy.PolicyProfile{Name: "fresh", Mode: policy.ModeDefault}); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(defaultTenantDir(dir), "fresh.yaml")); err != nil {
		t.Fatalf("expected fresh.yaml: %v", err)
	}
}

// Subdirectories that are not tenant IDs are ignored, not loaded as tenants.
func TestPolicyDir_IgnoresNonTenantSubdirectories(t *testing.T) {
	dir := t.TempDir()
	writePolicyFile(t, filepath.Join(dir, "backup"), "old.yaml", "name: old\nmode: default\n")
	svc := loadPolicyDir(t, dir)
	if _, ok := svc.GetProfile(context.Background(), "old"); ok {
		t.Fatal("profile of a non-tenant subdirectory loaded")
	}
}

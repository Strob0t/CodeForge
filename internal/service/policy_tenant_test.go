package service_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// Custom policy profiles belong to a tenant (KI-68): stored under
// <policy dir>/<tenant id>/, resolved tenant first, then the built-in presets
// (global, read-only). Files directly in the policy directory (the layout
// before tenants) stay readable for the default tenant only and are never
// changed.

const (
	tenantA = "aaaaaaaa-0000-4000-8000-000000000001"
	tenantB = "bbbbbbbb-0000-4000-8000-000000000002"
)

func inTenant(tenantID string) context.Context {
	return tenantctx.WithTenant(context.Background(), tenantID)
}

func teamProfile(mode policy.PermissionMode) *policy.PolicyProfile {
	return &policy.PolicyProfile{Name: "team", Mode: mode}
}

func TestPolicyTenants_SameNameInTwoTenantsIsIndependent(t *testing.T) {
	svc := loadPolicyDir(t, t.TempDir())
	ctxA, ctxB := inTenant(tenantA), inTenant(tenantB)

	if err := svc.SaveProfile(ctxA, teamProfile(policy.ModePlan)); err != nil {
		t.Fatalf("save A: %v", err)
	}
	if err := svc.SaveProfile(ctxB, teamProfile(policy.ModeAcceptEdits)); err != nil {
		t.Fatalf("save B: %v", err)
	}

	a, _ := svc.GetProfile(ctxA, "team")
	b, _ := svc.GetProfile(ctxB, "team")
	if a.Mode != policy.ModePlan || b.Mode != policy.ModeAcceptEdits {
		t.Fatalf("modes A=%s B=%s, want plan and acceptEdits", a.Mode, b.Mode)
	}
	dA, _ := svc.Evaluate(ctxA, "team", policy.ToolCall{Tool: "Bash", Command: "ls"})
	dB, _ := svc.Evaluate(ctxB, "team", policy.ToolCall{Tool: "Bash", Command: "ls"})
	if dA != policy.DecisionDeny || dB != policy.DecisionAllow {
		t.Fatalf("decisions A=%s B=%s, want deny and allow", dA, dB)
	}
}

func TestPolicyTenants_AnotherTenantCannotSeeReplaceDeleteOrEvaluate(t *testing.T) {
	svc := loadPolicyDir(t, t.TempDir())
	ctxA, ctxB := inTenant(tenantA), inTenant(tenantB)
	if err := svc.SaveProfile(ctxA, &policy.PolicyProfile{Name: "a-only", Mode: policy.ModePlan}); err != nil {
		t.Fatal(err)
	}

	if _, ok := svc.GetProfile(ctxB, "a-only"); ok {
		t.Error("tenant B sees tenant A's profile")
	}
	if slices.Contains(svc.ListProfiles(ctxB), "a-only") {
		t.Error("tenant B lists tenant A's profile")
	}
	if !slices.Contains(svc.ListProfiles(ctxA), "a-only") {
		t.Error("tenant A does not list its own profile")
	}
	if _, err := svc.EvaluateWithReason(ctxB, "a-only", policy.ToolCall{Tool: "Read"}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("tenant B evaluates tenant A's profile: err=%v", err)
	}
	if err := svc.DeleteProfile(ctxB, "a-only"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("tenant B deletes tenant A's profile: err=%v", err)
	}
	// Saving the name in B creates B's own profile; A's stays.
	if err := svc.SaveProfile(ctxB, &policy.PolicyProfile{Name: "a-only", Mode: policy.ModeAcceptEdits}); err != nil {
		t.Fatalf("save B: %v", err)
	}
	if p, ok := svc.GetProfile(ctxA, "a-only"); !ok || p.Mode != policy.ModePlan {
		t.Errorf("tenant A's profile changed: %+v ok=%v", p, ok)
	}
	if err := svc.PrependRule(ctxB, "a-only", &allowMake); err != nil {
		t.Fatalf("prepend B: %v", err)
	}
	if p, _ := svc.GetProfile(ctxA, "a-only"); len(p.Rules) != 0 {
		t.Errorf("tenant B's rule reached tenant A's profile: %+v", p.Rules)
	}
}

func TestPolicyTenants_PresetsAreGlobalAndReadOnly(t *testing.T) {
	svc := loadPolicyDir(t, t.TempDir())
	for _, tenant := range []string{tenantA, tenantB, tenantctx.DefaultTenantID} {
		ctx := inTenant(tenant)
		for _, name := range policy.PresetNames() {
			if _, ok := svc.GetProfile(ctx, name); !ok {
				t.Errorf("tenant %s: preset %s missing", tenant, name)
			}
			if !slices.Contains(svc.ListProfiles(ctx), name) {
				t.Errorf("tenant %s: preset %s not listed", tenant, name)
			}
		}
		err := svc.SaveProfile(ctx, &policy.PolicyProfile{Name: "plan-readonly", Mode: policy.ModeAcceptEdits})
		if !errors.Is(err, domain.ErrConflict) {
			t.Errorf("tenant %s replaced a preset: err=%v", tenant, err)
		}
	}
}

func TestPolicyTenants_ProfilesArePersistedPerTenant(t *testing.T) {
	dir := t.TempDir()
	svc := loadPolicyDir(t, dir)
	if err := svc.SaveProfile(inTenant(tenantA), teamProfile(policy.ModePlan)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, tenantA, "team.yaml")); err != nil {
		t.Fatalf("expected %s/team.yaml: %v", tenantA, err)
	}

	restarted := loadPolicyDir(t, dir)
	if p, ok := restarted.GetProfile(inTenant(tenantA), "team"); !ok || p.Mode != policy.ModePlan {
		t.Fatalf("tenant A's profile not loaded after restart: %+v ok=%v", p, ok)
	}
	if _, ok := restarted.GetProfile(inTenant(tenantB), "team"); ok {
		t.Fatal("tenant B sees tenant A's profile after restart")
	}
	if err := restarted.DeleteProfile(inTenant(tenantA), "team"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, tenantA, "team.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file not removed: %v", err)
	}
}

// A tenant ID that is not a UUID never becomes a path.
func TestPolicyTenants_InvalidTenantIsNotAPath(t *testing.T) {
	dir := t.TempDir()
	svc := loadPolicyDir(t, dir)
	for _, tenant := range []string{"../escape", "a/b", ".", "..", "not-a-uuid"} {
		err := svc.SaveProfile(inTenant(tenant), teamProfile(policy.ModePlan))
		if !errors.Is(err, domain.ErrValidation) {
			t.Errorf("tenant %q: err=%v, want ErrValidation", tenant, err)
		}
	}
	if entries, _ := os.ReadDir(filepath.Dir(dir)); len(entries) != 1 {
		t.Errorf("files written next to the policy dir: %v", entries)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("files written for an invalid tenant: %v", entries)
	}
}

func TestPolicyTenants_FlatFilesAreReadOnlyForTheDefaultTenant(t *testing.T) {
	dir := t.TempDir()
	writePolicyFile(t, dir, "legacy.yaml", "name: legacy\nmode: plan\n")
	original, _ := os.ReadFile(filepath.Join(dir, "legacy.yaml")) //nolint:gosec // G304: test file path from t.TempDir()
	svc := loadPolicyDir(t, dir)
	def := inTenant(tenantctx.DefaultTenantID)

	if _, ok := svc.GetProfile(def, "legacy"); !ok {
		t.Fatal("default tenant does not see the flat file profile")
	}
	if _, ok := svc.GetProfile(inTenant(tenantA), "legacy"); ok {
		t.Fatal("another tenant sees the flat file profile")
	}
	if err := svc.DeleteProfile(def, "legacy"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("delete of a flat file profile: err=%v, want ErrConflict", err)
	}
	// A change is a tenant copy that takes precedence; the flat file stays.
	if err := svc.PrependRule(def, "legacy", &allowMake); err != nil {
		t.Fatalf("PrependRule: %v", err)
	}
	if p, _ := svc.GetProfile(def, "legacy"); len(p.Rules) != 1 {
		t.Fatalf("rule missing: %+v", p.Rules)
	}
	if now, _ := os.ReadFile(filepath.Join(dir, "legacy.yaml")); !bytes.Equal(now, original) { //nolint:gosec // G304: test file path from t.TempDir()
		t.Fatalf("flat file changed:\n%s", now)
	}
	if _, err := os.Stat(filepath.Join(dir, tenantctx.DefaultTenantID, "legacy.yaml")); err != nil {
		t.Fatalf("tenant copy not written: %v", err)
	}
	restarted := loadPolicyDir(t, dir)
	if p, _ := restarted.GetProfile(def, "legacy"); len(p.Rules) != 1 {
		t.Fatalf("tenant copy does not take precedence after restart: %+v", p.Rules)
	}
	// Deleting the copy brings back the flat file's version.
	if err := restarted.DeleteProfile(def, "legacy"); err != nil {
		t.Fatalf("delete copy: %v", err)
	}
	if p, ok := restarted.GetProfile(def, "legacy"); !ok || len(p.Rules) != 0 {
		t.Fatalf("flat file profile not back: %+v ok=%v", p, ok)
	}
}

// Allow-Always clones land in the namespace of the tenant that owns the
// project, and only that tenant's calls use them.
func TestPolicyTenants_AllowAlwaysCloneBelongsToTheProjectsTenant(t *testing.T) {
	policySvc := newPersistentPolicyService(t, "headless-safe-sandbox")
	svc, store, queue, _ := newRuntimeTestEnvWithPolicy(policySvc)
	store.mu.Lock()
	store.projects = []project.Project{
		{ID: "proj-a", TenantID: tenantA, WorkspacePath: "/tmp/a"},
		{ID: "proj-b", TenantID: tenantB, WorkspacePath: "/tmp/b"},
	}
	store.runs = append(store.runs,
		run.Run{ID: "run-a", TenantID: tenantA, TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-a",
			PolicyProfile: "headless-safe-sandbox", Status: run.StatusRunning, StartedAt: time.Now()},
		run.Run{ID: "run-b", TenantID: tenantB, TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-b",
			PolicyProfile: "headless-safe-sandbox", Status: run.StatusRunning, StartedAt: time.Now()},
	)
	store.mu.Unlock()

	clone, err := policySvc.AllowAlways(inTenant(tenantA), storeProjects{store}, "proj-a", "headless-safe-sandbox", "bash", "make build")
	if err != nil {
		t.Fatalf("AllowAlways: %v", err)
	}
	if _, ok := policySvc.GetProfile(inTenant(tenantA), clone.Name); !ok {
		t.Fatal("clone not in tenant A")
	}
	if _, ok := policySvc.GetProfile(inTenant(tenantB), clone.Name); ok {
		t.Fatal("clone visible in tenant B")
	}
	for runID, want := range map[string]string{"run-a": "allow", "run-b": "deny"} {
		decision, reason := toolCallDecision(t, svc, queue, &messagequeue.ToolCallRequestPayload{
			RunID: runID, CallID: "c-" + runID, Tool: "bash", Command: "make build",
		})
		if decision != want {
			t.Errorf("%s: make build -> %s (%s), want %s", runID, decision, reason, want)
		}
	}
}

// The run path resolves the run's profile in the run's tenant.
func TestPolicyTenants_RunToolCallUsesTheRunsTenant(t *testing.T) {
	policySvc := loadPolicyDir(t, t.TempDir())
	strict := &policy.PolicyProfile{Name: "team", Mode: policy.ModePlan}
	open := &policy.PolicyProfile{Name: "team", Mode: policy.ModeAcceptEdits}
	if err := policySvc.SaveProfile(inTenant(tenantA), strict); err != nil {
		t.Fatal(err)
	}
	if err := policySvc.SaveProfile(inTenant(tenantB), open); err != nil {
		t.Fatal(err)
	}
	svc, store, queue, _ := newRuntimeTestEnvWithPolicy(policySvc)
	store.mu.Lock()
	store.runs = append(store.runs,
		run.Run{ID: "run-a", TenantID: tenantA, TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
			PolicyProfile: "team", Status: run.StatusRunning, StartedAt: time.Now()},
		run.Run{ID: "run-b", TenantID: tenantB, TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
			PolicyProfile: "team", Status: run.StatusRunning, StartedAt: time.Now()},
	)
	store.mu.Unlock()

	for runID, want := range map[string]string{"run-a": "deny", "run-b": "allow"} {
		decision, reason := toolCallDecision(t, svc, queue, &messagequeue.ToolCallRequestPayload{
			RunID: runID, CallID: "c-" + runID, Tool: "bash", Command: "ls",
		})
		if decision != want {
			t.Errorf("%s: ls -> %s (%s), want %s", runID, decision, reason, want)
		}
	}
}

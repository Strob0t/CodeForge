package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// newPersistentPolicyRouter returns a router whose PolicyService writes
// custom profiles to a temporary policy directory, as cmd/codeforge does
// when policy.custom_dir is set.
func newPersistentPolicyRouter(t *testing.T, store *mockStore) (chi.Router, *service.PolicyService, string) {
	t.Helper()
	dir := t.TempDir()
	policySvc := service.NewPolicyService("headless-safe-sandbox", nil)
	if err := policySvc.LoadPolicyDir(dir); err != nil {
		t.Fatal(err)
	}
	return newTestRouterWithPolicies(store, policySvc), policySvc, dir
}

func postJSON(t *testing.T, r chi.Router, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func createProfile(t *testing.T, r chi.Router, p *policy.PolicyProfile) {
	t.Helper()
	if w := postJSON(t, r, "/api/v1/policies", p); w.Code != http.StatusCreated {
		t.Fatalf("create profile %q: expected 201, got %d: %s", p.Name, w.Code, w.Body.String())
	}
}

func decodeProfile(t *testing.T, w *httptest.ResponseRecorder) policy.PolicyProfile {
	t.Helper()
	var result policy.PolicyProfile
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

// TestAllowAlwaysPolicy_ClonePreset verifies that when a project has no
// custom policy profile (i.e. it falls back to the default built-in preset),
// the handler clones the preset into the project's custom profile, persists
// it and prepends the requested allow rule. The project is not pinned to the
// clone: pinning made every mode of the project use the clone of the
// service default instead of its mode-derived profile (review finding 5);
// the clone replaces the preset for this project's calls instead.
func TestAllowAlwaysPolicy_ClonePreset(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{
			{ID: "proj-1", Name: "Test Project"},
		},
	}
	r, _, dir := newPersistentPolicyRouter(t, store)

	w := postJSON(t, r, "/api/v1/policies/allow-always", map[string]string{
		"project_id": "proj-1",
		"tool":       "write_file",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeProfile(t, w)

	// The cloned profile name should contain the original preset name and project ID.
	expectedName := "headless-safe-sandbox-custom-proj-1"
	if result.Name != expectedName {
		t.Fatalf("expected profile name %q, got %q", expectedName, result.Name)
	}

	// The first rule should be the new "allow Write" rule (canonical tool name).
	if len(result.Rules) == 0 {
		t.Fatal("expected at least one rule in the cloned profile")
	}
	firstRule := result.Rules[0]
	if firstRule.Specifier.Tool != "Write" {
		t.Fatalf("expected first rule tool 'Write', got %q", firstRule.Specifier.Tool)
	}
	if firstRule.Decision != policy.DecisionAllow {
		t.Fatalf("expected first rule decision 'allow', got %q", firstRule.Decision)
	}

	// The project's profile selection is unchanged.
	if store.projects[0].PolicyProfile != "" {
		t.Fatalf("expected project policy profile unchanged, got %q", store.projects[0].PolicyProfile)
	}

	// The clone is on disk (in the caller's tenant), so the project reference survives a restart.
	if _, err := os.Stat(filepath.Join(dir, tenantctx.DefaultTenantID, expectedName+".yaml")); err != nil {
		t.Fatalf("expected persisted clone: %v", err)
	}
}

// TestAllowAlwaysPolicy_ClonesConfigPreset verifies that the clone is made
// from the project's config["policy_preset"], not from the service default.
func TestAllowAlwaysPolicy_ClonesConfigPreset(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{
			{ID: "proj-cfg", Name: "Cfg", Config: map[string]string{"policy_preset": "trusted-mount-autonomous"}},
		},
	}
	r, _, _ := newPersistentPolicyRouter(t, store)

	w := postJSON(t, r, "/api/v1/policies/allow-always", map[string]string{
		"project_id": "proj-cfg",
		"tool":       "mcp__github__create_issue",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := decodeProfile(t, w).Name; got != "trusted-mount-autonomous-custom-proj-cfg" {
		t.Fatalf("expected clone of the config preset, got %q", got)
	}
}

// TestAllowAlwaysPolicy_ClonesRequestedProfile verifies that the profile
// named by the permission request (the one that decided the call) is the
// one extended, not the project's or the service's default.
func TestAllowAlwaysPolicy_ClonesRequestedProfile(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{{ID: "proj-req", Name: "Req"}},
	}
	r, _, _ := newPersistentPolicyRouter(t, store)

	w := postJSON(t, r, "/api/v1/policies/allow-always", map[string]string{
		"project_id": "proj-req",
		"profile":    "trusted-mount-autonomous",
		"tool":       "write_file",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := decodeProfile(t, w).Name; got != "trusted-mount-autonomous-custom-proj-req" {
		t.Fatalf("expected clone of the requested profile, got %q", got)
	}

	w = postJSON(t, r, "/api/v1/policies/allow-always", map[string]string{
		"project_id": "proj-req",
		"profile":    "no-such-profile",
		"tool":       "write_file",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown profile: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAllowAlwaysPolicy_ExistingCustomProfile verifies that when a project
// already has a custom (non-preset) policy profile, the handler just prepends
// the rule without cloning.
func TestAllowAlwaysPolicy_ExistingCustomProfile(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{
			{ID: "proj-2", Name: "Custom Project", PolicyProfile: "my-custom"},
		},
	}
	r, _, _ := newPersistentPolicyRouter(t, store)

	// First, create the custom profile so it exists in the policy service.
	createProfile(t, r, &policy.PolicyProfile{
		Name: "my-custom",
		Mode: policy.ModeDefault,
		Rules: []policy.PermissionRule{
			{Specifier: policy.ToolSpecifier{Tool: "Read"}, Decision: policy.DecisionAllow},
		},
	})

	// Now call allow-always for the "edit_file" tool.
	w := postJSON(t, r, "/api/v1/policies/allow-always", map[string]string{
		"project_id": "proj-2",
		"tool":       "edit_file",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeProfile(t, w)

	// Profile name should remain "my-custom" (no clone).
	if result.Name != "my-custom" {
		t.Fatalf("expected profile name 'my-custom', got %q", result.Name)
	}

	// The first rule should be the new "allow Edit" rule.
	if len(result.Rules) < 2 {
		t.Fatalf("expected at least 2 rules, got %d", len(result.Rules))
	}
	if result.Rules[0].Specifier.Tool != "Edit" {
		t.Fatalf("expected first rule tool 'Edit', got %q", result.Rules[0].Specifier.Tool)
	}
	if result.Rules[0].Decision != policy.DecisionAllow {
		t.Fatalf("expected first rule decision 'allow', got %q", result.Rules[0].Decision)
	}

	// The project profile should not have changed.
	if store.projects[0].PolicyProfile != "my-custom" {
		t.Fatalf("expected project profile to stay 'my-custom', got %q", store.projects[0].PolicyProfile)
	}
}

// TestAllowAlwaysPolicy_Idempotent verifies that calling allow-always twice
// with the same tool and executable produces only one rule.
func TestAllowAlwaysPolicy_Idempotent(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{
			{ID: "proj-3", Name: "Idempotent Project", PolicyProfile: "idem-profile"},
		},
	}
	r, _, _ := newPersistentPolicyRouter(t, store)

	createProfile(t, r, &policy.PolicyProfile{Name: "idem-profile", Mode: policy.ModeDefault})

	body := map[string]string{
		"project_id": "proj-3",
		"tool":       "bash",
		"command":    "go test ./...",
	}

	w := postJSON(t, r, "/api/v1/policies/allow-always", body)
	if w.Code != http.StatusOK {
		t.Fatalf("first call: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	firstRuleCount := len(decodeProfile(t, w).Rules)

	// Second call: same tool, same executable, different arguments.
	body["command"] = "go test -run TestX ./internal/..."
	w = postJSON(t, r, "/api/v1/policies/allow-always", body)
	if w.Code != http.StatusOK {
		t.Fatalf("second call: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := len(decodeProfile(t, w).Rules); got != firstRuleCount {
		t.Fatalf("expected %d rules after idempotent call, got %d", firstRuleCount, got)
	}
}

// TestAllowAlwaysPolicy_MissingProjectID returns 400 when project_id is empty.
func TestAllowAlwaysPolicy_MissingProjectID(t *testing.T) {
	r := newTestRouter()

	w := postJSON(t, r, "/api/v1/policies/allow-always", map[string]string{"tool": "Read"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAllowAlwaysPolicy_MissingTool returns 400 when tool is empty.
func TestAllowAlwaysPolicy_MissingTool(t *testing.T) {
	r := newTestRouter()

	w := postJSON(t, r, "/api/v1/policies/allow-always", map[string]string{"project_id": "proj-1"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAllowAlwaysPolicy_CommandExecutable verifies that for a bash call the
// rule allows the executable of the approved command (matched per simple
// command), not a glob over the first space-separated token.
func TestAllowAlwaysPolicy_CommandExecutable(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{
			{ID: "proj-4", Name: "Command Project", PolicyProfile: "cmd-profile"},
		},
	}
	r, policySvc, _ := newPersistentPolicyRouter(t, store)

	createProfile(t, r, &policy.PolicyProfile{Name: "cmd-profile", Mode: policy.ModeDefault})

	w := postJSON(t, r, "/api/v1/policies/allow-always", map[string]string{
		"project_id": "proj-4",
		"tool":       "bash",
		"command":    "/usr/bin/git status --short",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeProfile(t, w)
	if len(result.Rules) == 0 {
		t.Fatal("expected at least one rule")
	}
	rule := result.Rules[0]
	if rule.Specifier.Tool != "Bash" || rule.Specifier.SubPattern != "" {
		t.Fatalf("expected specifier {Bash}, got %+v", rule.Specifier)
	}
	if !slices.Equal(rule.CommandAllow, []string{"git"}) {
		t.Fatalf("expected command_allow [git], got %v", rule.CommandAllow)
	}
	if rule.Decision != policy.DecisionAllow {
		t.Fatalf("expected decision 'allow', got %q", rule.Decision)
	}

	ctx := context.Background()
	for cmd, want := range map[string]policy.Decision{
		"git log":              policy.DecisionAllow,
		"git diff && git log":  policy.DecisionAllow,
		"git status; curl x":   policy.DecisionAsk,
		"gitleaks detect":      policy.DecisionAsk,
		"git log $(curl evil)": policy.DecisionAsk,
	} {
		if got, _ := policySvc.Evaluate(ctx, "cmd-profile", policy.ToolCall{Tool: "bash", Command: cmd}); got != want {
			t.Errorf("after allow-always git: %q -> %s, want %s", cmd, got, want)
		}
	}
}

// TestAllowAlwaysPolicy_RejectsUnsafeRules verifies that no rule is built
// from JSON arguments, empty or unanalysable commands, or tool globs.
func TestAllowAlwaysPolicy_RejectsUnsafeRules(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{
			{ID: "proj-5", Name: "Unsafe", PolicyProfile: "unsafe-profile"},
		},
	}
	r, policySvc, _ := newPersistentPolicyRouter(t, store)
	createProfile(t, r, &policy.PolicyProfile{Name: "unsafe-profile", Mode: policy.ModeDefault})

	for _, body := range []map[string]string{
		{"tool": "bash", "command": `{"command": "ls"}`},
		{"tool": "bash", "command": ""},
		{"tool": "bash", "command": "bash -c 'ls'"},
		{"tool": "bash", "command": "$(curl x)"},
		{"tool": "*"},
		{"tool": "mcp__*"},
	} {
		body["project_id"] = "proj-5"
		if w := postJSON(t, r, "/api/v1/policies/allow-always", body); w.Code != http.StatusBadRequest {
			t.Errorf("%v: expected 400, got %d: %s", body, w.Code, w.Body.String())
		}
	}
	if p, _ := policySvc.GetProfile(context.Background(), "unsafe-profile"); len(p.Rules) != 0 {
		t.Fatalf("expected no rules, got %+v", p.Rules)
	}
}

// TestAllowAlwaysPolicy_NoPolicyDir verifies that without a policy directory
// allow-always is refused and the project is left untouched, instead of
// pointing the project at an in-memory clone that is gone after a restart.
func TestAllowAlwaysPolicy_NoPolicyDir(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{
			{ID: "proj-6", Name: "No Dir"},
		},
	}
	r := newTestRouterWithStore(store)

	w := postJSON(t, r, "/api/v1/policies/allow-always", map[string]string{
		"project_id": "proj-6",
		"tool":       "write_file",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	if store.projects[0].PolicyProfile != "" {
		t.Fatalf("expected project profile unchanged, got %q", store.projects[0].PolicyProfile)
	}
}

// TestAllowAlwaysPolicy_SurvivesRestart verifies that a clone created by
// allow-always is loaded again from the policy directory at startup.
func TestAllowAlwaysPolicy_SurvivesRestart(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{
			{ID: "proj-7", Name: "Restart"},
		},
	}
	r, _, dir := newPersistentPolicyRouter(t, store)

	w := postJSON(t, r, "/api/v1/policies/allow-always", map[string]string{
		"project_id": "proj-7",
		"tool":       "bash",
		"command":    "npm test",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Restart: load the directory the way cmd/codeforge does.
	restarted := service.NewPolicyService("headless-safe-sandbox", nil)
	if err := restarted.LoadPolicyDir(dir); err != nil {
		t.Fatalf("load policy dir: %v", err)
	}
	name := "headless-safe-sandbox-custom-proj-7"
	p, ok := restarted.GetProfile(context.Background(), name)
	if !ok {
		t.Fatalf("profile %q not loaded after restart", name)
	}
	if len(p.Rules) == 0 || !slices.Equal(p.Rules[0].CommandAllow, []string{"npm"}) {
		t.Fatalf("allow-always rule missing after restart: %+v", p.Rules)
	}
	d, err := restarted.Evaluate(context.Background(), name, policy.ToolCall{Tool: "bash", Command: "npm test"})
	if err != nil || d != policy.DecisionAllow {
		t.Fatalf("npm test after restart -> %s, %v; want allow", d, err)
	}
}

// TestCreatePolicyProfile_PresetConflict verifies that built-in presets
// cannot be overwritten via the API (KI-9).
func TestCreatePolicyProfile_PresetConflict(t *testing.T) {
	r, policySvc, dir := newPersistentPolicyRouter(t, &mockStore{})

	for _, name := range policy.PresetNames() {
		w := postJSON(t, r, "/api/v1/policies", policy.PolicyProfile{
			Name:  name,
			Mode:  policy.ModeAcceptEdits,
			Rules: []policy.PermissionRule{{Specifier: policy.ToolSpecifier{Tool: "*"}, Decision: policy.DecisionAllow}},
		})
		if w.Code != http.StatusConflict {
			t.Errorf("%s: expected 409, got %d: %s", name, w.Code, w.Body.String())
		}
		p, _ := policySvc.GetProfile(context.Background(), name)
		want, _ := policy.PresetByName(name)
		if p.Mode != want.Mode || len(p.Rules) != len(want.Rules) {
			t.Errorf("%s: preset was modified: %+v", name, p)
		}
		if _, err := os.Stat(filepath.Join(dir, tenantctx.DefaultTenantID, name+".yaml")); !os.IsNotExist(err) {
			t.Errorf("%s: preset override written to the policy dir", name)
		}
	}
}

// TestCreatePolicyProfile_InvalidProfile returns 400 for a profile that fails validation.
func TestCreatePolicyProfile_InvalidProfile(t *testing.T) {
	r := newTestRouter()
	w := postJSON(t, r, "/api/v1/policies", policy.PolicyProfile{
		Name: "bad-mode",
		Mode: "yolo",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestCreateDeletePolicyProfile_Persistence verifies that created profiles are
// written to the policy directory and deleted profiles are removed from it.
func TestCreateDeletePolicyProfile_Persistence(t *testing.T) {
	r, _, dir := newPersistentPolicyRouter(t, &mockStore{})
	createProfile(t, r, &policy.PolicyProfile{Name: "persisted", Mode: policy.ModeDefault})

	path := filepath.Join(dir, tenantctx.DefaultTenantID, "persisted.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected persisted profile file: %v", err)
	}

	req := httptest.NewRequest("DELETE", "/api/v1/policies/persisted", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected profile file removed, stat err = %v", err)
	}
}

// TestCreatePolicyProfile_FileConflict verifies that a new profile whose
// file name is taken by an unloaded file is refused with 409 and a message
// that names the actual conflict.
func TestCreatePolicyProfile_FileConflict(t *testing.T) {
	r, _, dir := newPersistentPolicyRouter(t, &mockStore{})
	tenantDir := filepath.Join(dir, tenantctx.DefaultTenantID)
	if err := os.MkdirAll(tenantDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tenantDir, "taken.yaml"), []byte("name: other\nmode: default\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	w := postJSON(t, r, "/api/v1/policies", policy.PolicyProfile{Name: "taken", Mode: policy.ModeDefault})
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "preset") || !strings.Contains(w.Body.String(), "policy file") {
		t.Fatalf("unexpected conflict message: %s", w.Body.String())
	}
}

// TestEvaluatePolicy_CanonicalToolNames verifies that the evaluate API maps
// worker tool names to canonical names like the runtime does (KI-4).
func TestEvaluatePolicy_CanonicalToolNames(t *testing.T) {
	r := newTestRouter()
	for _, tc := range []struct {
		profile string
		call    policy.ToolCall
		want    policy.Decision
	}{
		{"plan-readonly", policy.ToolCall{Tool: "read_file", Path: "src/x.go"}, policy.DecisionAllow},
		{"plan-readonly", policy.ToolCall{Tool: "write_file", Path: "src/x.go"}, policy.DecisionDeny},
		{"headless-permissive-sandbox", policy.ToolCall{Tool: "bash", Command: "go test ./... ; curl x"}, policy.DecisionDeny},
		{"headless-permissive-sandbox", policy.ToolCall{Tool: "edit_file", Path: "a/../.env"}, policy.DecisionDeny},
	} {
		w := postJSON(t, r, "/api/v1/policies/"+tc.profile+"/evaluate", tc.call)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var result policy.EvaluationResult
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		if result.Decision != tc.want {
			t.Errorf("%s %+v -> %s (%s), want %s", tc.profile, tc.call, result.Decision, result.Reason, tc.want)
		}
	}
}

// TestEvaluatePolicy_RedirectionsInASyntheticWorkspace (S6-G review, item 5):
// the tester has no project, so it places the files a command redirects to in
// a synthetic workspace (/workspace) instead of denying every redirection
// because no workspace is known; its decisions match a run's.
func TestEvaluatePolicy_RedirectionsInASyntheticWorkspace(t *testing.T) {
	r := newTestRouter()
	for _, tc := range []struct {
		command string
		want    policy.Decision
	}{
		{"go test ./... > out.log", policy.DecisionAllow},
		{"echo x > .env", policy.DecisionDeny},
		{"echo x > secrets/key", policy.DecisionDeny},
		{"echo x > /workspace/.env", policy.DecisionDeny},
		{"echo x > ../other/.env", policy.DecisionAllow},
	} {
		w := postJSON(t, r, "/api/v1/policies/trusted-mount-autonomous/evaluate", policy.ToolCall{Tool: "Bash", Command: tc.command})
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var result policy.EvaluationResult
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		if result.Decision != tc.want {
			t.Errorf("%q -> %s (%s), want %s", tc.command, result.Decision, result.Reason, tc.want)
		}
	}
}

// TestPolicyProfiles_TenantScoped: custom profiles belong to the caller's
// tenant; another tenant can neither list, read, evaluate, replace nor
// delete them (KI-68).
func TestPolicyProfiles_TenantScoped(t *testing.T) {
	const (
		tenantA = "aaaaaaaa-0000-4000-8000-000000000001"
		tenantB = "bbbbbbbb-0000-4000-8000-000000000002"
	)
	r, _, dir := newPersistentPolicyRouter(t, &mockStore{})
	do := func(tenant, method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var data []byte
		if body != nil {
			var err error
			if data, err = json.Marshal(body); err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(tenantctx.WithTenant(req.Context(), tenant))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	if w := do(tenantA, "POST", "/api/v1/policies", policy.PolicyProfile{Name: "team", Mode: policy.ModePlan}); w.Code != http.StatusCreated {
		t.Fatalf("create in A: %d %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, tenantA, "team.yaml")); err != nil {
		t.Fatalf("profile not stored in tenant A's directory: %v", err)
	}

	var listB struct{ Profiles []string }
	if err := json.NewDecoder(do(tenantB, "GET", "/api/v1/policies", nil).Body).Decode(&listB); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(listB.Profiles, "team") || !slices.Contains(listB.Profiles, "plan-readonly") {
		t.Fatalf("tenant B's list: %v", listB.Profiles)
	}
	if w := do(tenantB, "GET", "/api/v1/policies/team", nil); w.Code != http.StatusNotFound {
		t.Errorf("B reads A's profile: %d", w.Code)
	}
	if w := do(tenantB, "POST", "/api/v1/policies/team/evaluate", policy.ToolCall{Tool: "Read"}); w.Code != http.StatusNotFound {
		t.Errorf("B evaluates A's profile: %d", w.Code)
	}
	if w := do(tenantB, "DELETE", "/api/v1/policies/team", nil); w.Code != http.StatusNotFound {
		t.Errorf("B deletes A's profile: %d", w.Code)
	}
	if w := do(tenantB, "POST", "/api/v1/policies", policy.PolicyProfile{Name: "team", Mode: policy.ModeAcceptEdits}); w.Code != http.StatusCreated {
		t.Fatalf("create in B: %d %s", w.Code, w.Body.String())
	}
	w := do(tenantA, "GET", "/api/v1/policies/team", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("A reads its profile: %d", w.Code)
	}
	if p := decodeProfile(t, w); p.Mode != policy.ModePlan {
		t.Fatalf("tenant A's profile replaced by B: %+v", p)
	}
}

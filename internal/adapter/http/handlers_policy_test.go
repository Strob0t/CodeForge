package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// TestAllowAlwaysPolicy_ClonePreset verifies that when a project has no
// custom policy profile (i.e. it falls back to the default built-in preset),
// the handler clones the preset into a custom profile, assigns it to the
// project, and prepends the requested allow rule.
func TestAllowAlwaysPolicy_ClonePreset(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{
			{ID: "proj-1", Name: "Test Project"},
		},
	}
	r := newTestRouterWithStore(store)

	body, _ := json.Marshal(map[string]string{
		"project_id": "proj-1",
		"tool":       "Write",
	})
	req := httptest.NewRequest("POST", "/api/v1/policies/allow-always", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result policy.PolicyProfile
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	// The cloned profile name should contain the original preset name and project ID.
	expectedName := "headless-safe-sandbox-custom-proj-1"
	if result.Name != expectedName {
		t.Fatalf("expected profile name %q, got %q", expectedName, result.Name)
	}

	// The first rule should be the new "allow Write" rule.
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

	// The project should now reference the custom clone.
	if store.projects[0].PolicyProfile != expectedName {
		t.Fatalf("expected project policy profile %q, got %q", expectedName, store.projects[0].PolicyProfile)
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
	r := newTestRouterWithStore(store)

	// First, create the custom profile so it exists in the policy service.
	createBody, _ := json.Marshal(policy.PolicyProfile{
		Name: "my-custom",
		Mode: policy.ModeDefault,
		Rules: []policy.PermissionRule{
			{Specifier: policy.ToolSpecifier{Tool: "Read"}, Decision: policy.DecisionAllow},
		},
	})
	createReq := httptest.NewRequest("POST", "/api/v1/policies", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	cw := httptest.NewRecorder()
	r.ServeHTTP(cw, createReq)
	if cw.Code != http.StatusCreated {
		t.Fatalf("create custom profile: expected 201, got %d: %s", cw.Code, cw.Body.String())
	}

	// Now call allow-always for "Edit" tool.
	body, _ := json.Marshal(map[string]string{
		"project_id": "proj-2",
		"tool":       "Edit",
	})
	req := httptest.NewRequest("POST", "/api/v1/policies/allow-always", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result policy.PolicyProfile
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

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
// with the same tool produces only one rule (idempotent behavior).
func TestAllowAlwaysPolicy_Idempotent(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{
			{ID: "proj-3", Name: "Idempotent Project", PolicyProfile: "idem-profile"},
		},
	}
	r := newTestRouterWithStore(store)

	// Create the custom profile.
	createBody, _ := json.Marshal(policy.PolicyProfile{
		Name: "idem-profile",
		Mode: policy.ModeDefault,
	})
	createReq := httptest.NewRequest("POST", "/api/v1/policies", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	cw := httptest.NewRecorder()
	r.ServeHTTP(cw, createReq)
	if cw.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", cw.Code)
	}

	body, _ := json.Marshal(map[string]string{
		"project_id": "proj-3",
		"tool":       "Bash",
	})

	// First call.
	req := httptest.NewRequest("POST", "/api/v1/policies/allow-always", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first call: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var first policy.PolicyProfile
	_ = json.NewDecoder(w.Body).Decode(&first)
	firstRuleCount := len(first.Rules)

	// Second call (same tool).
	req = httptest.NewRequest("POST", "/api/v1/policies/allow-always", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("second call: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var second policy.PolicyProfile
	_ = json.NewDecoder(w.Body).Decode(&second)

	if len(second.Rules) != firstRuleCount {
		t.Fatalf("expected %d rules after idempotent call, got %d", firstRuleCount, len(second.Rules))
	}
}

// TestAllowAlwaysPolicy_MissingProjectID returns 400 when project_id is empty.
func TestAllowAlwaysPolicy_MissingProjectID(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(map[string]string{
		"tool": "Read",
	})
	req := httptest.NewRequest("POST", "/api/v1/policies/allow-always", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAllowAlwaysPolicy_MissingTool returns 400 when tool is empty.
func TestAllowAlwaysPolicy_MissingTool(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(map[string]string{
		"project_id": "proj-1",
	})
	req := httptest.NewRequest("POST", "/api/v1/policies/allow-always", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAllowAlwaysPolicy_CommandSubPattern verifies that when a command is
// provided, the handler extracts the first word as a glob sub-pattern.
// E.g. command="git status" produces SubPattern="git*".
func TestAllowAlwaysPolicy_CommandSubPattern(t *testing.T) {
	store := &mockStore{
		projects: []project.Project{
			{ID: "proj-4", Name: "Command Project", PolicyProfile: "cmd-profile"},
		},
	}
	r := newTestRouterWithStore(store)

	// Create the custom profile.
	createBody, _ := json.Marshal(policy.PolicyProfile{
		Name: "cmd-profile",
		Mode: policy.ModeDefault,
	})
	createReq := httptest.NewRequest("POST", "/api/v1/policies", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	cw := httptest.NewRecorder()
	r.ServeHTTP(cw, createReq)
	if cw.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", cw.Code)
	}

	body, _ := json.Marshal(map[string]string{
		"project_id": "proj-4",
		"tool":       "Bash",
		"command":    "git status",
	})
	req := httptest.NewRequest("POST", "/api/v1/policies/allow-always", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result policy.PolicyProfile
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if len(result.Rules) == 0 {
		t.Fatal("expected at least one rule")
	}
	rule := result.Rules[0]
	if rule.Specifier.Tool != "Bash" {
		t.Fatalf("expected tool 'Bash', got %q", rule.Specifier.Tool)
	}
	expectedSub := "git*"
	if rule.Specifier.SubPattern != expectedSub {
		t.Fatalf("expected sub_pattern %q, got %q", expectedSub, rule.Specifier.SubPattern)
	}
	if rule.Decision != policy.DecisionAllow {
		t.Fatalf("expected decision 'allow', got %q", rule.Decision)
	}
}

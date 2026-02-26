//go:build integration

package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/domain/mode"
	"github.com/Strob0t/CodeForge/internal/domain/prompt"
	"github.com/Strob0t/CodeForge/internal/domain/tenant"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
)

// --------------------------------------------------------------------------
// Test helpers (mirrored from adapter/postgres/store_test.go)
// --------------------------------------------------------------------------

// setupIntegrationStore creates a pgxpool, runs migrations, and returns a
// ready-to-use Store. Skips the test when DATABASE_URL is not set.
func setupIntegrationStore(t *testing.T) *postgres.Store {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("requires DATABASE_URL")
	}

	ctx := context.Background()

	if err := postgres.RunMigrations(ctx, dsn); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	return postgres.NewStore(pool)
}

// ctxWithTenant builds a context carrying the given tenant ID by routing a
// fake HTTP request through the TenantID middleware.
func ctxWithTenant(t *testing.T, tenantID string) context.Context {
	t.Helper()
	ch := make(chan context.Context, 1)
	handler := middleware.TenantID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ch <- r.Context()
	}))
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("X-Tenant-ID", tenantID)
	handler.ServeHTTP(httptest.NewRecorder(), req)
	select {
	case ctx := <-ch:
		return ctx
	default:
		t.Fatal("TenantID middleware did not invoke next handler")
		return nil
	}
}

// createTenant creates a tenant with a unique slug and returns its ID.
func createTenant(t *testing.T, store *postgres.Store) string {
	t.Helper()
	slug := "integ-" + uuid.New().String()[:8]
	tn, err := store.CreateTenant(context.Background(), tenant.CreateRequest{
		Name: "Integration Test Tenant",
		Slug: slug,
	})
	if err != nil {
		t.Fatalf("create test tenant: %v", err)
	}
	return tn.ID
}

// findBuiltinMode returns a pointer to the built-in mode with the given ID.
func findBuiltinMode(t *testing.T, id string) *mode.Mode {
	t.Helper()
	builtins := mode.BuiltinModes()
	for i := range builtins {
		if builtins[i].ID == id {
			return &builtins[i]
		}
	}
	t.Fatalf("built-in mode %q not found", id)
	return nil
}

// --------------------------------------------------------------------------
// Integration tests
// --------------------------------------------------------------------------

// TestBuildModePromptWithDBOverrides_Replace verifies that a DB row with
// merge="replace" completely replaces the embedded section text.
func TestBuildModePromptWithDBOverrides_Replace(t *testing.T) {
	store := setupIntegrationStore(t)
	tenantID := createTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	// Step 1: Build embedded sections for the "coder" built-in mode.
	m := findBuiltinMode(t, "coder")
	_, sections := service.BuildModePrompt(m)

	// Sanity check: the embedded sections contain a "role" section.
	hasRole := false
	for _, s := range sections {
		if s.Name == "role" {
			hasRole = true
			break
		}
	}
	if !hasRole {
		t.Fatal("expected embedded sections to contain a 'role' section")
	}

	// Step 2: Insert a DB override that replaces the "role" section.
	overrideContent := "TEST OVERRIDE: You are a specialized test agent."
	err := store.UpsertPromptSection(ctx, &prompt.SectionRow{
		Name:      "role",
		Scope:     "mode:coder",
		Content:   overrideContent,
		Priority:  service.PriorityRole,
		SortOrder: 0,
		Enabled:   true,
		Merge:     "replace",
	})
	if err != nil {
		t.Fatalf("UpsertPromptSection: %v", err)
	}

	// Step 3: Load DB overrides and apply them.
	dbRows, err := store.ListPromptSections(ctx, "mode:coder")
	if err != nil {
		t.Fatalf("ListPromptSections: %v", err)
	}
	if len(dbRows) == 0 {
		t.Fatal("expected at least one DB prompt section row")
	}

	merged := service.ApplyDBOverrides(sections, dbRows)

	// Step 4: Verify the "role" section was replaced.
	var roleSection *service.PromptSection
	for i, s := range merged {
		if s.Name == "role" {
			roleSection = &merged[i]
			break
		}
	}
	if roleSection == nil {
		t.Fatal("expected merged sections to contain a 'role' section")
	}
	if roleSection.Text != overrideContent {
		t.Errorf("expected role text to be %q, got %q", overrideContent, roleSection.Text)
	}
	if roleSection.Source != "db_override" {
		t.Errorf("expected source 'db_override', got %q", roleSection.Source)
	}

	// Assembled prompt should contain the override, not the original.
	assembled := service.AssembleSections(merged)
	if !strings.Contains(assembled, overrideContent) {
		t.Errorf("assembled prompt should contain override text %q", overrideContent)
	}
}

// TestBuildModePromptWithDBOverrides_Append verifies that a DB row with
// merge="append" adds content after the embedded section text.
func TestBuildModePromptWithDBOverrides_Append(t *testing.T) {
	store := setupIntegrationStore(t)
	tenantID := createTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	// Step 1: Build embedded sections for the "coder" built-in mode.
	m := findBuiltinMode(t, "coder")
	_, sections := service.BuildModePrompt(m)

	// Capture the original role text before override.
	var originalRoleText string
	for _, s := range sections {
		if s.Name == "role" {
			originalRoleText = s.Text
			break
		}
	}
	if originalRoleText == "" {
		t.Fatal("expected non-empty original role text")
	}

	// Step 2: Insert a DB override with merge="append".
	appendContent := "ADDITIONAL INSTRUCTION: Always write tests first."
	err := store.UpsertPromptSection(ctx, &prompt.SectionRow{
		Name:      "role",
		Scope:     "mode:coder",
		Content:   appendContent,
		Priority:  0, // keep original priority
		SortOrder: 0,
		Enabled:   true,
		Merge:     "append",
	})
	if err != nil {
		t.Fatalf("UpsertPromptSection: %v", err)
	}

	// Step 3: Load DB overrides and apply them.
	dbRows, err := store.ListPromptSections(ctx, "mode:coder")
	if err != nil {
		t.Fatalf("ListPromptSections: %v", err)
	}

	merged := service.ApplyDBOverrides(sections, dbRows)

	// Step 4: Verify both texts are present in the "role" section.
	var roleSection *service.PromptSection
	for i, s := range merged {
		if s.Name == "role" {
			roleSection = &merged[i]
			break
		}
	}
	if roleSection == nil {
		t.Fatal("expected merged sections to contain a 'role' section")
	}
	if !strings.Contains(roleSection.Text, originalRoleText) {
		t.Error("appended section should still contain the original embedded text")
	}
	if !strings.Contains(roleSection.Text, appendContent) {
		t.Error("appended section should contain the DB override text")
	}
	// Original text should come before the appended text.
	origIdx := strings.Index(roleSection.Text, originalRoleText)
	appendIdx := strings.Index(roleSection.Text, appendContent)
	if origIdx >= appendIdx {
		t.Error("original text should appear before appended text")
	}
	if roleSection.Source != "db_override" {
		t.Errorf("expected source 'db_override', got %q", roleSection.Source)
	}
}

// TestBuildModePromptWithDBOverrides_Disabled verifies that a DB row with
// enabled=false disables the matching embedded section so it is excluded
// from the assembled prompt.
func TestBuildModePromptWithDBOverrides_Disabled(t *testing.T) {
	store := setupIntegrationStore(t)
	tenantID := createTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	// Step 1: Build embedded sections for the "coder" built-in mode.
	m := findBuiltinMode(t, "coder")
	_, sections := service.BuildModePrompt(m)

	// Capture the original role text before override.
	var originalRoleText string
	for _, s := range sections {
		if s.Name == "role" {
			originalRoleText = s.Text
			break
		}
	}
	if originalRoleText == "" {
		t.Fatal("expected non-empty original role text")
	}

	// Step 2: Insert a DB override with enabled=false.
	err := store.UpsertPromptSection(ctx, &prompt.SectionRow{
		Name:      "role",
		Scope:     "mode:coder",
		Content:   "This content should be irrelevant because the section is disabled.",
		Priority:  service.PriorityRole,
		SortOrder: 0,
		Enabled:   false,
		Merge:     "replace",
	})
	if err != nil {
		t.Fatalf("UpsertPromptSection: %v", err)
	}

	// Step 3: Load DB overrides and apply them.
	dbRows, err := store.ListPromptSections(ctx, "mode:coder")
	if err != nil {
		t.Fatalf("ListPromptSections: %v", err)
	}

	merged := service.ApplyDBOverrides(sections, dbRows)

	// Step 4: Verify the "role" section is disabled.
	var roleSection *service.PromptSection
	for i, s := range merged {
		if s.Name == "role" {
			roleSection = &merged[i]
			break
		}
	}
	if roleSection == nil {
		t.Fatal("expected merged sections to still contain a 'role' section entry")
	}
	if roleSection.Enabled {
		t.Error("expected role section to be disabled after DB override")
	}
	if roleSection.Source != "db_override" {
		t.Errorf("expected source 'db_override', got %q", roleSection.Source)
	}

	// Assembled prompt should NOT contain the original role text.
	assembled := service.AssembleSections(merged)
	if strings.Contains(assembled, originalRoleText) {
		t.Error("assembled prompt should NOT contain the disabled section text")
	}
	// The assembled prompt should still have other sections (tools, guardrails, etc.).
	if assembled == "" {
		t.Error("assembled prompt should not be empty -- other sections should survive")
	}
}
